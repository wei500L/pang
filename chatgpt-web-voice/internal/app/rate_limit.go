package app

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dyhhhhhh/chatgpt-web-voice/internal/auth"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/config"
)

type publicRateEntry struct {
	count       int
	windowStart time.Time
}

type publicRateLimiter struct {
	mu              sync.Mutex
	sessions        map[string]publicRateEntry
	writes          map[string]publicRateEntry
	recordingChunks map[string]publicRateEntry
	sessionLimit    int
	writeLimit      int
	trustCloudflare bool
	now             func() time.Time
}

func newPublicRateLimiter(cfg config.Config) *publicRateLimiter {
	sessionLimit := cfg.PublicSessionRate
	if sessionLimit < 1 {
		sessionLimit = 10
	}
	writeLimit := cfg.PublicWriteRate
	if writeLimit < 1 {
		writeLimit = 60
	}
	return &publicRateLimiter{
		sessions:        make(map[string]publicRateEntry),
		writes:          make(map[string]publicRateEntry),
		recordingChunks: make(map[string]publicRateEntry),
		sessionLimit:    sessionLimit,
		writeLimit:      writeLimit,
		trustCloudflare: cfg.TrustCloudflareIP,
		now:             time.Now,
	}
}

func (l *publicRateLimiter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if l == nil || !auth.IsGuest(r.Context()) || !isPublicWrite(r) {
			next.ServeHTTP(w, r)
			return
		}
		key := l.clientIP(r)
		entries, limit := l.writes, l.writeLimit
		if isRecordingChunkWrite(r) {
			// Periodic audio chunks must not consume the quota used by chat and
			// conversation persistence. Keep a separate bounded bucket instead.
			entries, limit = l.recordingChunks, 60
		} else if isHighCostPublicWrite(r) {
			entries, limit = l.sessions, l.sessionLimit
		}
		allowed, retryAfter := l.allow(entries, key, limit)
		if !allowed {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"detail": map[string]any{"error": "public request rate limit exceeded"},
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isRecordingChunkWrite(r *http.Request) bool {
	return r.Method == http.MethodPut &&
		strings.HasPrefix(r.URL.Path, "/api/recordings/") &&
		strings.Contains(r.URL.Path, "/chunks/")
}

func (l *publicRateLimiter) allow(entries map[string]publicRateEntry, key string, limit int) (bool, int) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(entries) > 4096 {
		for entryKey, candidate := range entries {
			if now.Sub(candidate.windowStart) >= 2*time.Minute {
				delete(entries, entryKey)
			}
		}
	}
	entry := entries[key]
	if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= time.Minute {
		entry = publicRateEntry{windowStart: now}
	}
	entry.count++
	entries[key] = entry
	if entry.count <= limit {
		return true, 0
	}
	retry := int(entry.windowStart.Add(time.Minute).Sub(now).Round(time.Second) / time.Second)
	if retry < 1 {
		retry = 1
	}
	return false, retry
}

func (l *publicRateLimiter) clientIP(r *http.Request) string {
	if l.trustCloudflare {
		if value := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); net.ParseIP(value) != nil {
			return value
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if value := strings.TrimSpace(r.RemoteAddr); value != "" {
		return value
	}
	return "unknown"
}

func isPublicWrite(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return strings.HasPrefix(r.URL.Path, "/api/")
	default:
		return false
	}
}

func isHighCostPublicWrite(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	return r.URL.Path == "/api/voice/session" || strings.Contains(r.URL.Path, "/uploads")
}
