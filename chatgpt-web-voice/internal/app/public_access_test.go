package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dyhhhhhh/chatgpt-web-voice/internal/api"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/apikeys"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/auth"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/config"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/conversations"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/store"
)

type browserCookies struct {
	guest *http.Cookie
	csrf  *http.Cookie
}

func newPublicAccessHandler(t *testing.T, cfg config.Config) http.Handler {
	t.Helper()
	staticDir := t.TempDir()
	for _, name := range []string{"login.html", "voice.html", "accounts.html", "keys.html", "sessions.html", "records.html"} {
		if err := os.WriteFile(filepath.Join(staticDir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "voice.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	adminAuth := auth.New("admin", "password", time.Hour, logger)
	keyStore := apikeys.NewStore(db)
	keyAuth := auth.NewAPIKeyManager(keyStore, logger)
	apiServer := api.New(api.Dependencies{
		Conversations: conversations.NewStore(db),
		APIKeys:       keyStore,
	})
	cfg.StaticDir = staticDir
	return newHandler(cfg, adminAuth, keyAuth, apiServer, logger)
}

func openPublicVoice(t *testing.T, handler http.Handler, remoteAddr string) browserCookies {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/voice", nil)
	req.Header.Set("Accept", "text/html")
	req.RemoteAddr = remoteAddr
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("public voice status=%d body=%s", resp.Code, resp.Body.String())
	}
	var result browserCookies
	for _, cookie := range resp.Result().Cookies() {
		switch cookie.Name {
		case "voice_gateway_guest":
			result.guest = cookie
		case "voice_gateway_csrf":
			result.csrf = cookie
		}
	}
	if result.guest == nil || !result.guest.HttpOnly || result.guest.MaxAge <= 0 {
		t.Fatalf("missing persistent guest cookie: %+v", resp.Result().Cookies())
	}
	if result.csrf == nil || result.csrf.HttpOnly {
		t.Fatalf("missing readable csrf cookie: %+v", resp.Result().Cookies())
	}
	return result
}

func addBrowserCookies(req *http.Request, cookies browserCookies, withCSRF bool) {
	if cookies.guest != nil {
		req.AddCookie(cookies.guest)
	}
	if cookies.csrf != nil {
		req.AddCookie(cookies.csrf)
		if withCSRF {
			req.Header.Set("X-CSRF-Token", cookies.csrf.Value)
		}
	}
}

func TestPublicVoiceAndAdminRoutesAreSeparated(t *testing.T) {
	handler := newPublicAccessHandler(t, config.Config{})
	cookies := openPublicVoice(t, handler, "203.0.113.10:1234")

	sessionReq := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	addBrowserCookies(sessionReq, cookies, false)
	sessionResp := httptest.NewRecorder()
	handler.ServeHTTP(sessionResp, sessionReq)
	if sessionResp.Code != http.StatusOK || !strings.Contains(sessionResp.Body.String(), `"role":"guest"`) {
		t.Fatalf("guest session status=%d body=%s", sessionResp.Code, sessionResp.Body.String())
	}

	keysReq := httptest.NewRequest(http.MethodGet, "/keys", nil)
	keysReq.Header.Set("Accept", "text/html")
	keysResp := httptest.NewRecorder()
	handler.ServeHTTP(keysResp, keysReq)
	if keysResp.Code != http.StatusSeeOther || keysResp.Header().Get("Location") != "/login?next=%2Fkeys" {
		t.Fatalf("keys redirect status=%d location=%q", keysResp.Code, keysResp.Header().Get("Location"))
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	adminResp := httptest.NewRecorder()
	handler.ServeHTTP(adminResp, adminReq)
	if adminResp.Code != http.StatusUnauthorized {
		t.Fatalf("admin api status=%d body=%s", adminResp.Code, adminResp.Body.String())
	}

	voiceConfigReq := httptest.NewRequest(http.MethodGet, "/api/voice/config", nil)
	addBrowserCookies(voiceConfigReq, cookies, false)
	voiceConfigResp := httptest.NewRecorder()
	handler.ServeHTTP(voiceConfigResp, voiceConfigReq)
	if voiceConfigResp.Code != http.StatusOK {
		t.Fatalf("public voice config status=%d body=%s", voiceConfigResp.Code, voiceConfigResp.Body.String())
	}
}

func TestGuestConversationOwnersAreIsolatedAndCSRFProtected(t *testing.T) {
	handler := newPublicAccessHandler(t, config.Config{})
	first := openPublicVoice(t, handler, "203.0.113.11:1234")
	second := openPublicVoice(t, handler, "203.0.113.12:1234")

	badReq := httptest.NewRequest(http.MethodPost, "/api/conversations", bytes.NewBufferString(`{"title":"private"}`))
	badReq.Header.Set("Content-Type", "application/json")
	addBrowserCookies(badReq, first, false)
	badResp := httptest.NewRecorder()
	handler.ServeHTTP(badResp, badReq)
	if badResp.Code != http.StatusForbidden {
		t.Fatalf("missing csrf status=%d body=%s", badResp.Code, badResp.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/conversations", bytes.NewBufferString(`{"title":"private"}`))
	createReq.Header.Set("Content-Type", "application/json")
	addBrowserCookies(createReq, first, true)
	createResp := httptest.NewRecorder()
	handler.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		Conversation struct {
			ID string `json:"id"`
		} `json:"conversation"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil || created.Conversation.ID == "" {
		t.Fatalf("decode created conversation: %v body=%s", err, createResp.Body.String())
	}

	ownedReq := httptest.NewRequest(http.MethodGet, "/api/conversations/"+created.Conversation.ID, nil)
	addBrowserCookies(ownedReq, first, false)
	ownedResp := httptest.NewRecorder()
	handler.ServeHTTP(ownedResp, ownedReq)
	if ownedResp.Code != http.StatusOK {
		t.Fatalf("owner read status=%d body=%s", ownedResp.Code, ownedResp.Body.String())
	}

	otherReq := httptest.NewRequest(http.MethodGet, "/api/conversations/"+created.Conversation.ID, nil)
	addBrowserCookies(otherReq, second, false)
	otherResp := httptest.NewRecorder()
	handler.ServeHTTP(otherResp, otherReq)
	if otherResp.Code != http.StatusNotFound {
		t.Fatalf("cross-guest read status=%d body=%s", otherResp.Code, otherResp.Body.String())
	}
}

func TestGuestHighCostRateLimit(t *testing.T) {
	handler := newPublicAccessHandler(t, config.Config{PublicSessionRate: 1, PublicWriteRate: 10})
	cookies := openPublicVoice(t, handler, "203.0.113.20:1234")
	for attempt := 1; attempt <= 2; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/api/voice/session", bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.20:4567"
		addBrowserCookies(req, cookies, true)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if attempt == 1 && resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("first attempt status=%d body=%s", resp.Code, resp.Body.String())
		}
		if attempt == 2 && (resp.Code != http.StatusTooManyRequests || resp.Header().Get("Retry-After") == "") {
			t.Fatalf("limited attempt status=%d retry=%q body=%s", resp.Code, resp.Header().Get("Retry-After"), resp.Body.String())
		}
	}
}

func TestRateLimiterUsesCloudflareIPOnlyWhenTrusted(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/voice", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	req.Header.Set("CF-Connecting-IP", "203.0.113.99")
	trusted := newPublicRateLimiter(config.Config{TrustCloudflareIP: true})
	if got := trusted.clientIP(req); got != "203.0.113.99" {
		t.Fatalf("trusted client ip=%q", got)
	}
	untrusted := newPublicRateLimiter(config.Config{TrustCloudflareIP: false})
	if got := untrusted.clientIP(req); got != "192.0.2.10" {
		t.Fatalf("untrusted client ip=%q", got)
	}
}

func TestBackgroundWritesUseIndependentRateLimitBuckets(t *testing.T) {
	limiter := newPublicRateLimiter(config.Config{PublicWriteRate: 1})
	if allowed, _ := limiter.allow(limiter.writes, "guest-ip", 1); !allowed {
		t.Fatal("expected first general write to be allowed")
	}
	if allowed, _ := limiter.allow(limiter.writes, "guest-ip", 1); allowed {
		t.Fatal("expected general write bucket to be exhausted")
	}
	if allowed, _ := limiter.allow(limiter.recordingChunks, "guest-ip", publicRecordingChunkLimit); !allowed {
		t.Fatal("recording chunks must not consume the general write bucket")
	}
	if allowed, _ := limiter.allow(limiter.conversationWrites, "guest-ip", publicConversationWriteLimit); !allowed {
		t.Fatal("conversation persistence must not consume the general write bucket")
	}
	req := httptest.NewRequest(http.MethodPut, "/api/recordings/rec_test/chunks/0", nil)
	if !isRecordingChunkWrite(req) {
		t.Fatal("recording chunk route was not classified separately")
	}
	conversationReq := httptest.NewRequest(http.MethodPost, "/api/conversations/cv_test/messages", nil)
	if !isConversationWrite(conversationReq) {
		t.Fatal("conversation message route was not classified separately")
	}
	controlReq := httptest.NewRequest(http.MethodPost, "/api/voice/session/release", nil)
	if isConversationWrite(controlReq) {
		t.Fatal("voice control writes must remain outside the conversation bucket")
	}
}

func TestConversationWriteLimitDoesNotConsumeGeneralWriteCapacity(t *testing.T) {
	limiter := newPublicRateLimiter(config.Config{PublicWriteRate: 1})
	for attempt := 0; attempt < publicConversationWriteLimit; attempt++ {
		if allowed, _ := limiter.allow(limiter.conversationWrites, "guest-ip", publicConversationWriteLimit); !allowed {
			t.Fatalf("conversation write %d was limited early", attempt+1)
		}
	}
	if allowed, _ := limiter.allow(limiter.conversationWrites, "guest-ip", publicConversationWriteLimit); allowed {
		t.Fatal("conversation write bucket must remain bounded")
	}
	if allowed, _ := limiter.allow(limiter.writes, "guest-ip", 1); !allowed {
		t.Fatal("conversation writes must not consume general control capacity")
	}
}

func TestPublicRateBucketCapsNewClientEntries(t *testing.T) {
	limiter := newPublicRateLimiter(config.Config{})
	for index := 0; index < maxPublicRateEntries; index++ {
		key := fmt.Sprintf("client-%d", index)
		if allowed, _ := limiter.allow(limiter.writes, key, 1); !allowed {
			t.Fatalf("client entry %d was rejected before the cap", index)
		}
	}
	if allowed, _ := limiter.allow(limiter.writes, "overflow-client", 1); allowed {
		t.Fatal("new client must be rejected when the bucket is at its memory cap")
	}
	if allowed, _ := limiter.allow(limiter.writes, "client-0", 2); !allowed {
		t.Fatal("an existing client must keep using its bounded entry")
	}
}
