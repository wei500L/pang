package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegisterStaticRoutesUsesCleanURLs(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "voice.html"), []byte("voice page"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "accounts.html"), []byte("accounts page"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "keys.html"), []byte("keys page"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "sessions.html"), []byte("sessions page"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "records.html"), []byte("records page"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "app.css"), []byte("/* design system */"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(staticDir, "agent-visual"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "agent-visual", "index.js"), []byte("export const ready = true;"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(staticDir, "models"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "models", "agent-particles.bin"), []byte{1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	registerPublicStaticAssets(mux, staticDir)
	registerStaticRoutes(mux, staticDir)

	for path, wantBody := range map[string]string{
		"/voice":                             "voice page",
		"/accounts":                          "accounts page",
		"/keys":                              "keys page",
		"/sessions":                          "sessions page",
		"/records":                           "records page",
		"/static/app.css":                    "/* design system */",
		"/static/agent-visual/index.js":      "export const ready = true;",
		"/static/models/agent-particles.bin": string([]byte{1, 2, 3}),
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK || strings.TrimSpace(resp.Body.String()) != wantBody {
			t.Fatalf("GET %s: status=%d body=%q", path, resp.Code, resp.Body.String())
		}
	}

	for path, location := range map[string]string{
		"/":              "/voice",
		"/voice.html":    "/voice",
		"/accounts.html": "/accounts",
		"/keys.html":     "/keys",
		"/sessions.html": "/sessions",
		"/records.html":  "/records",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, req)
		if resp.Code < 300 || resp.Code >= 400 || resp.Header().Get("Location") != location {
			t.Fatalf("GET %s: status=%d location=%q", path, resp.Code, resp.Header().Get("Location"))
		}
	}
}

func TestSecurityHeadersUsesCacheablePolicyForStaticAssets(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	tests := []struct {
		path       string
		cache      string
		wantPragma bool
	}{
		{path: "/voice", cache: "no-store", wantPragma: true},
		{path: "/api/conversations", cache: "no-store", wantPragma: true},
		{path: "/static/voice-room.css", cache: staticDocumentCacheControl},
		{path: "/static/agent-visual/index.js", cache: staticDocumentCacheControl},
		{path: "/static/assets/voice-room/natural-room-wide.png", cache: staticAssetCacheControl},
		{path: "/static/models/agent-particles.bin", cache: staticAssetCacheControl},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, test.path, nil)
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)

			if got := resp.Header().Get("Cache-Control"); got != test.cache {
				t.Fatalf("Cache-Control=%q want %q", got, test.cache)
			}
			if got := resp.Header().Get("Pragma"); (got != "") != test.wantPragma {
				t.Fatalf("Pragma=%q want present=%v", got, test.wantPragma)
			}
		})
	}
}
