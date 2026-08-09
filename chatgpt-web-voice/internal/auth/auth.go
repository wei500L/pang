package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dyhhhhhh/chatgpt-web-voice/internal/logging"
)

const sessionCookieName = "voice_gateway_session"
const csrfCookieName = "voice_gateway_csrf"
const guestCookieName = "voice_gateway_guest"
const csrfHeaderName = "X-CSRF-Token"

const guestCookieTTL = 365 * 24 * time.Hour

// Default login brute-force controls. Override via NewWithLimits for tests/config.
const (
	DefaultLoginMaxFailures  = 8
	DefaultLoginWindow       = 15 * time.Minute
	DefaultLoginLockout      = 15 * time.Minute
	DefaultLoginFailureDelay = 200 * time.Millisecond
)

type userContextKey struct{}
type principalContextKey struct{}

const (
	RoleGuest = "guest"
	RoleAdmin = "admin"
)

type Principal struct {
	Role              string
	Username          string
	ConversationOwner string
	VoiceOwner        string
}

type session struct {
	Username  string
	ExpiresAt time.Time
}

type loginAttempt struct {
	failures    int
	firstAt     time.Time
	lockedUntil time.Time
	lastAt      time.Time
}

// Limits controls login / Basic-Auth brute-force protection.
type Limits struct {
	MaxFailures  int
	Window       time.Duration
	Lockout      time.Duration
	FailureDelay time.Duration
}

func (l Limits) normalized() Limits {
	if l.MaxFailures < 1 {
		l.MaxFailures = DefaultLoginMaxFailures
	}
	if l.Window <= 0 {
		l.Window = DefaultLoginWindow
	}
	if l.Lockout <= 0 {
		l.Lockout = DefaultLoginLockout
	}
	if l.FailureDelay < 0 {
		l.FailureDelay = 0
	}
	return l
}

// Manager validates configured credentials and owns browser login sessions.
type Manager struct {
	username          string
	password          string
	ttl               time.Duration
	limits            Limits
	logger            *slog.Logger
	store             *SessionStore
	turnstile         TurnstileVerifier
	trustCloudflareIP bool

	mu       sync.Mutex
	sessions map[string]session
	attempts map[string]*loginAttempt
}

// New creates an authentication manager with default login rate limits.
func New(username, password string, ttl time.Duration, logger *slog.Logger) *Manager {
	return NewWithLimits(username, password, ttl, Limits{}, logger)
}

// NewWithLimits creates an authentication manager with custom brute-force limits.
func NewWithLimits(username, password string, ttl time.Duration, limits Limits, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		username: strings.TrimSpace(username),
		password: password,
		ttl:      ttl,
		limits:   limits.normalized(),
		logger:   logger,
		sessions: make(map[string]session),
		attempts: make(map[string]*loginAttempt),
	}
}

// WithSessionStore enables durable browser sessions in SQLite.
func (m *Manager) WithSessionStore(store *SessionStore) *Manager {
	if m != nil {
		m.store = store
	}
	return m
}

// WithTurnstile requires a successful Turnstile verification before password
// validation. Tests and development callers may omit it explicitly.
func (m *Manager) WithTurnstile(verifier TurnstileVerifier) *Manager {
	if m != nil {
		m.turnstile = verifier
	}
	return m
}

// WithTrustedCloudflareIP enables CF-Connecting-IP for login lockout and
// Turnstile remote-IP verification. The origin must reject direct traffic.
func (m *Manager) WithTrustedCloudflareIP(trust bool) *Manager {
	if m != nil {
		m.trustCloudflareIP = trust
	}
	return m
}

// Username returns the authenticated username stored in the request context.
func Username(ctx context.Context) string {
	username, _ := ctx.Value(userContextKey{}).(string)
	return username
}

// RequestPrincipal returns the public/admin identity resolved by middleware.
func RequestPrincipal(ctx context.Context) Principal {
	principal, _ := ctx.Value(principalContextKey{}).(Principal)
	return principal
}

func IsAdmin(ctx context.Context) bool {
	return RequestPrincipal(ctx).Role == RoleAdmin
}

func IsGuest(ctx context.Context) bool {
	return RequestPrincipal(ctx).Role == RoleGuest
}

func ConversationOwner(ctx context.Context) string {
	return RequestPrincipal(ctx).ConversationOwner
}

func VoiceOwner(ctx context.Context) string {
	return RequestPrincipal(ctx).VoiceOwner
}

// Require protects pages, static content, and APIs. Browser navigation is
// redirected to the login page; API clients receive a JSON 401 response.
func (m *Manager) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, method, ok := m.authenticate(r)
		if ok {
			// Cookie sessions need CSRF protection on state-changing requests.
			if method == "session" && isUnsafeMethod(r.Method) && !m.validCSRF(r) {
				logging.FromContext(r.Context()).Warn("csrf_rejected", "remote_addr", r.RemoteAddr, "path", r.URL.Path)
				writeAuthError(w, http.StatusForbidden, "csrf token missing or invalid")
				return
			}
			if method == "session" {
				m.ensureCSRFCookie(w, r)
			}
			ctx := adminContext(r.Context(), username)
			logging.FromContext(ctx).Debug("authentication_succeeded", "auth_method", method, "username", username)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		logging.FromContext(r.Context()).Warn("authentication_denied", "remote_addr", r.RemoteAddr)
		if isBrowserNavigation(r) {
			nextPath := safeAdminNext(r.URL.RequestURI())
			location := "/login"
			if nextPath != "" {
				location += "?next=" + url.QueryEscape(nextPath)
			}
			http.Redirect(w, r, location, http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"detail": map[string]any{"error": "authentication required"},
		})
	})
}

// Public allows the built-in voice workspace without login while assigning an
// opaque per-browser owner. Authenticated administrators keep their existing
// owner namespaces for backward compatibility.
func (m *Manager) Public(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		guestToken := m.ensureGuestCookie(w, r)
		m.ensureCSRFCookie(w, r)
		if isUnsafeMethod(r.Method) && !m.validCSRF(r) {
			logging.FromContext(r.Context()).Warn("csrf_rejected", "remote_addr", r.RemoteAddr, "path", r.URL.Path)
			writeAuthError(w, http.StatusForbidden, "csrf token missing or invalid")
			return
		}

		ctx := r.Context()
		if username, method, ok := m.authenticate(r); ok {
			ctx = adminContext(ctx, username)
			logging.FromContext(ctx).Debug("authentication_succeeded", "auth_method", method, "username", username)
		} else {
			ctx = guestContext(ctx, guestToken)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LoginPage redirects an already authenticated browser to the requested safe
// administrator page, defaulting to the API-key dashboard.
func (m *Manager) LoginPage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := m.authenticate(r); ok {
			destination := safeAdminNext(r.URL.Query().Get("next"))
			if destination == "" {
				destination = "/keys"
			}
			http.Redirect(w, r, destination, http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// HandleLogin validates a JSON or form username/password and sets an HttpOnly
// session cookie. The optional remember flag controls whether the browser
// retains the cookie after it closes. Credentials are never written to logs.
func (m *Manager) HandleLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	ip := m.clientIP(r)
	if retry, locked := m.lockoutRemaining(ip); locked {
		logging.FromContext(r.Context()).Warn("login_locked", "remote_addr", r.RemoteAddr, "retry_after_sec", int(retry.Seconds()))
		w.Header().Set("Retry-After", formatRetryAfter(retry))
		writeAuthError(w, http.StatusTooManyRequests, "too many failed login attempts, try again later")
		return
	}

	var input struct {
		Username       string `json:"username"`
		Password       string `json:"password"`
		Remember       bool   `json:"remember"`
		TurnstileToken string `json:"turnstile_token"`
		Next           string `json:"next"`
	}

	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(contentType, "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeAuthError(w, http.StatusBadRequest, "invalid login request")
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			writeAuthError(w, http.StatusBadRequest, "invalid login request")
			return
		}
		input.Username = r.FormValue("username")
		input.Password = r.FormValue("password")
		input.Remember = formBool(r.FormValue("remember"))
		input.TurnstileToken = r.FormValue("turnstile_token")
		input.Next = r.FormValue("next")
	}

	if m.turnstile != nil {
		verified, err := m.turnstile.Verify(r.Context(), strings.TrimSpace(input.TurnstileToken), ip)
		if err != nil {
			logging.FromContext(r.Context()).Error("turnstile_verification_failed", "remote_addr", r.RemoteAddr, "error", err)
			writeAuthError(w, http.StatusServiceUnavailable, "captcha verification unavailable")
			return
		}
		if !verified {
			logging.FromContext(r.Context()).Warn("turnstile_rejected", "remote_addr", r.RemoteAddr)
			writeAuthError(w, http.StatusForbidden, "captcha verification failed")
			return
		}
	}

	if !m.validCredentials(input.Username, input.Password) {
		retry, locked := m.recordFailure(ip)
		logging.FromContext(r.Context()).Warn(
			"login_failed",
			"username", logUsername(input.Username),
			"remote_addr", r.RemoteAddr,
			"locked", locked,
		)
		if m.limits.FailureDelay > 0 {
			time.Sleep(m.limits.FailureDelay)
		}
		if locked {
			w.Header().Set("Retry-After", formatRetryAfter(retry))
			writeAuthError(w, http.StatusTooManyRequests, "too many failed login attempts, try again later")
			return
		}
		writeAuthError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	m.clearFailures(ip)

	token, err := newSessionToken()
	if err != nil {
		m.logger.Error("session_token_generation_failed", "error", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to create login session")
		return
	}

	expiresAt := time.Now().Add(m.ttl)
	m.mu.Lock()
	m.cleanupLocked(time.Now())
	m.sessions[token] = session{Username: m.username, ExpiresAt: expiresAt}
	m.mu.Unlock()
	if m.store != nil {
		if err := m.store.Put(token, m.username, expiresAt); err != nil {
			m.logger.Error("session_persist_failed", "error", err)
			writeAuthError(w, http.StatusInternalServerError, "failed to create login session")
			return
		}
	}

	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	}
	if input.Remember {
		cookie.Expires = expiresAt
		cookie.MaxAge = int(m.ttl.Seconds())
	}
	http.SetCookie(w, cookie)
	m.issueCSRFCookie(w, r, input.Remember, expiresAt)
	logging.FromContext(r.Context()).Info("login_succeeded", "username", m.username, "remote_addr", r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":       true,
		"username": m.username,
		"redirect": loginRedirect(input.Next),
	})
}

// HandleLogout revokes the current browser session and expires its cookie.
func (m *Manager) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		m.mu.Lock()
		delete(m.sessions, cookie.Value)
		m.mu.Unlock()
		if m.store != nil {
			_ = m.store.Delete(cookie.Value)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: false,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
	logging.FromContext(r.Context()).Info("logout_succeeded", "username", Username(r.Context()))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// HandleSession returns either the authenticated administrator or the public
// guest role without exposing configured credentials.
func (m *Manager) HandleSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if IsAdmin(r.Context()) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"authenticated": true,
			"role":          RoleAdmin,
			"username":      Username(r.Context()),
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"authenticated": false,
		"role":          RoleGuest,
	})
}

func (m *Manager) HandleAuthConfig(w http.ResponseWriter, _ *http.Request) {
	siteKey := ""
	if m.turnstile != nil {
		siteKey = m.turnstile.SiteKey()
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"turnstile_site_key": siteKey})
}

func (m *Manager) authenticate(r *http.Request) (username, method string, ok bool) {
	// Browser session cookie only. Management Basic Auth is intentionally unsupported.
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", "", false
	}
	now := time.Now()
	m.mu.Lock()
	m.cleanupLocked(now)
	item, found := m.sessions[cookie.Value]
	if found && now.Before(item.ExpiresAt) {
		username := item.Username
		m.mu.Unlock()
		return username, "session", true
	}
	delete(m.sessions, cookie.Value)
	m.mu.Unlock()

	if m.store != nil {
		username, expiresAt, ok, err := m.store.Get(cookie.Value)
		if err != nil {
			m.logger.Error("session_lookup_failed", "error", err)
			return "", "", false
		}
		if ok {
			m.mu.Lock()
			m.sessions[cookie.Value] = session{Username: username, ExpiresAt: expiresAt}
			m.mu.Unlock()
			return username, "session", true
		}
	}
	return "", "", false
}

func (m *Manager) validCredentials(username, password string) bool {
	wantUsername := sha256.Sum256([]byte(m.username))
	gotUsername := sha256.Sum256([]byte(strings.TrimSpace(username)))
	wantPassword := sha256.Sum256([]byte(m.password))
	gotPassword := sha256.Sum256([]byte(password))
	return subtle.ConstantTimeCompare(wantUsername[:], gotUsername[:]) == 1 &&
		subtle.ConstantTimeCompare(wantPassword[:], gotPassword[:]) == 1
}

func (m *Manager) lockoutRemaining(ip string) (time.Duration, bool) {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupAttemptsLocked(now)
	item := m.attempts[ip]
	if item == nil || !now.Before(item.lockedUntil) {
		return 0, false
	}
	return item.lockedUntil.Sub(now), true
}

func (m *Manager) recordFailure(ip string) (retryAfter time.Duration, locked bool) {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupAttemptsLocked(now)
	item := m.attempts[ip]
	if item == nil {
		item = &loginAttempt{firstAt: now}
		m.attempts[ip] = item
	}
	if now.Before(item.lockedUntil) {
		return item.lockedUntil.Sub(now), true
	}
	// Reset the rolling window when the previous window expired.
	if item.failures > 0 && now.Sub(item.firstAt) > m.limits.Window {
		item.failures = 0
		item.firstAt = now
	}
	if item.failures == 0 {
		item.firstAt = now
	}
	item.failures++
	item.lastAt = now
	if item.failures >= m.limits.MaxFailures {
		item.lockedUntil = now.Add(m.limits.Lockout)
		return m.limits.Lockout, true
	}
	return 0, false
}

func (m *Manager) clearFailures(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.attempts, ip)
}

func (m *Manager) cleanupAttemptsLocked(now time.Time) {
	for ip, item := range m.attempts {
		if item == nil {
			delete(m.attempts, ip)
			continue
		}
		if now.Before(item.lockedUntil) {
			continue
		}
		if item.failures > 0 && now.Sub(item.firstAt) <= m.limits.Window {
			continue
		}
		if item.failures == 0 && now.Sub(item.lastAt) <= m.limits.Window {
			continue
		}
		// Drop stale counters after both window and lockout have passed.
		if !now.Before(item.lockedUntil) && now.Sub(item.lastAt) > m.limits.Window {
			delete(m.attempts, ip)
		}
	}
}

func (m *Manager) cleanupLocked(now time.Time) {
	for token, item := range m.sessions {
		if !now.Before(item.ExpiresAt) {
			delete(m.sessions, token)
		}
	}
	m.cleanupAttemptsLocked(now)
}

func newSessionToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func adminContext(ctx context.Context, username string) context.Context {
	username = strings.TrimSpace(username)
	ctx = context.WithValue(ctx, userContextKey{}, username)
	return context.WithValue(ctx, principalContextKey{}, Principal{
		Role:              RoleAdmin,
		Username:          username,
		ConversationOwner: username,
		VoiceOwner:        "admin:" + username,
	})
}

func guestContext(ctx context.Context, token string) context.Context {
	sum := sha256.Sum256([]byte(token))
	owner := "guest:" + hex.EncodeToString(sum[:])
	return context.WithValue(ctx, principalContextKey{}, Principal{
		Role:              RoleGuest,
		ConversationOwner: owner,
		VoiceOwner:        owner,
	})
}

func (m *Manager) ensureGuestCookie(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(guestCookieName); err == nil && validGuestToken(cookie.Value) {
		return cookie.Value
	}
	token, err := newSessionToken()
	if err != nil {
		m.logger.Error("guest_token_generation_failed", "error", err)
		return "unavailable"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     guestCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(guestCookieTTL),
		MaxAge:   int(guestCookieTTL.Seconds()),
	})
	return token
}

func validGuestToken(value string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	return err == nil && len(raw) == 32
}

func safeAdminNext(value string) string {
	value = strings.TrimSpace(value)
	if parsed, err := url.Parse(value); err == nil && parsed.RawQuery == "" && parsed.Fragment == "" {
		value = parsed.Path
	}
	switch value {
	case "/accounts", "/keys", "/sessions", "/voice":
		return value
	default:
		return ""
	}
}

func loginRedirect(next string) string {
	if destination := safeAdminNext(next); destination != "" {
		return destination
	}
	return "/keys"
}

func isBrowserNavigation(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	return !strings.HasPrefix(r.URL.Path, "/api/") && strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/html")
}

func requestIsHTTPS(r *http.Request) bool {
	// Only the direct TLS state of this process is considered. Reverse proxies
	// such as Caddy should terminate TLS; this gateway does not interpret
	// X-Forwarded-Proto.
	return r != nil && r.TLS != nil
}

func (m *Manager) clientIP(r *http.Request) string {
	if m != nil && m.trustCloudflareIP {
		if value := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); net.ParseIP(value) != nil {
			return value
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if ip := strings.TrimSpace(r.RemoteAddr); ip != "" {
		return ip
	}
	return "unknown"
}

func formatRetryAfter(d time.Duration) string {
	sec := int(d.Round(time.Second) / time.Second)
	if sec < 1 {
		sec = 1
	}
	return strconv.Itoa(sec)
}

func formBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func logUsername(username string) string {
	username = strings.TrimSpace(username)
	if len(username) > 128 {
		return username[:128]
	}
	return username
}

func isUnsafeMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func (m *Manager) validCSRF(r *http.Request) bool {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	header := strings.TrimSpace(r.Header.Get(csrfHeaderName))
	if header == "" {
		return false
	}
	// Constant-time compare after hashing to equalize length handling.
	want := sha256.Sum256([]byte(cookie.Value))
	got := sha256.Sum256([]byte(header))
	return subtle.ConstantTimeCompare(want[:], got[:]) == 1
}

func (m *Manager) ensureCSRFCookie(w http.ResponseWriter, r *http.Request) {
	if _, err := r.Cookie(csrfCookieName); err == nil {
		return
	}
	m.issueCSRFCookie(w, r, false, time.Now().Add(m.ttl))
}

func (m *Manager) issueCSRFCookie(w http.ResponseWriter, r *http.Request, remember bool, expiresAt time.Time) {
	token, err := newSessionToken()
	if err != nil {
		m.logger.Error("csrf_token_generation_failed", "error", err)
		return
	}
	cookie := &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // must be readable by same-origin JS
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	}
	if remember {
		cookie.Expires = expiresAt
		cookie.MaxAge = int(time.Until(expiresAt).Seconds())
		if cookie.MaxAge < 1 {
			cookie.MaxAge = 1
		}
	}
	http.SetCookie(w, cookie)
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"detail": map[string]any{"error": message},
	})
}
