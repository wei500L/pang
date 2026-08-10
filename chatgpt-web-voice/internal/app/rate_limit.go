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

const (
	publicConversationWriteLimit = 12000
	publicRecordingChunkLimit    = 3000
	maxPublicRateEntries         = 4096
	rateSweepInterval            = 10 * time.Second
	rateEntryMaxAge              = 2 * time.Minute
)

type publicRateBucket struct {
	entries   map[string]publicRateEntry
	lastSweep time.Time
}

type publicRateLimiter struct {
	mu                 sync.Mutex
	sessions           *publicRateBucket
	writes             *publicRateBucket
	conversationWrites *publicRateBucket
	recordingChunks    *publicRateBucket
	sessionLimit       int
	writeLimit         int
	trustCloudflare    bool
	now                func() time.Time
}

func newPublicRateLimiter(cfg config.Config) *publicRateLimiter {
	sessionLimit := cfg.PublicSessionRate
	if sessionLimit < 1 {
		sessionLimit = 300
	}
	writeLimit := cfg.PublicWriteRate
	if writeLimit < 1 {
		writeLimit = 3000
	}
	return &publicRateLimiter{
		sessions:           newPublicRateBucket(),
		writes:             newPublicRateBucket(),
		conversationWrites: newPublicRateBucket(),
		recordingChunks:    newPublicRateBucket(),
		sessionLimit:       sessionLimit,
		writeLimit:         writeLimit,
		trustCloudflare:    cfg.TrustCloudflareIP,
		now:                time.Now,
	}
}

func newPublicRateBucket() *publicRateBucket {
	return &publicRateBucket{entries: make(map[string]publicRateEntry)}
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
			// Periodic audio chunks must not consume the quota used by call
			// control and recording lifecycle requests.
			entries, limit = l.recordingChunks, publicRecordingChunkLimit
		} else if isConversationWrite(r) {
			// Streaming captions are idempotent updates to a small set of message
			// rows. Isolate them from call release/context and recording completion
			// so persistence pressure cannot degrade the live voice control path.
			entries, limit = l.conversationWrites, publicConversationWriteLimit
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

func isConversationWrite(r *http.Request) bool {
	return r.URL.Path == "/api/conversations" || strings.HasPrefix(r.URL.Path, "/api/conversations/")
}

func (l *publicRateLimiter) allow(bucket *publicRateBucket, key string, limit int) (bool, int) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, exists := bucket.entries[key]
	if !exists && len(bucket.entries) >= maxPublicRateEntries {
		if bucket.lastSweep.IsZero() || now.Sub(bucket.lastSweep) >= rateSweepInterval {
			for entryKey, candidate := range bucket.entries {
				if now.Sub(candidate.windowStart) >= rateEntryMaxAge {
					delete(bucket.entries, entryKey)
				}
			}
			bucket.lastSweep = now
		}
		if len(bucket.entries) >= maxPublicRateEntries {
			// Bound both memory and cleanup CPU during a flood of fresh source
			// addresses. Existing clients keep their current windows; unknown
			// clients fail closed until an entry expires.
			return false, int(time.Minute / time.Second)
		}
	}
	if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= time.Minute {
		entry = publicRateEntry{windowStart: now}
	}
	entry.count++
	bucket.entries[key] = entry
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
