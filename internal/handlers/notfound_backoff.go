package handlers

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// notFoundTracker is a per-IP escalating backoff for repeated 404s on
// /api/requests/*. Tokens are 128-bit and unguessable, so a flood of misses
// from one IP is a brute-force scan — block them for a while.
//
// In-memory only: state is lost on restart, which is fine for short-lived
// backoffs. Entries older than the cleanup window are pruned to bound memory.
type notFoundTracker struct {
	mu      sync.Mutex
	entries map[string]*ntfEntry

	threshold  int           // 404s allowed in window before lock
	window     time.Duration // sliding window for counting
	lockoutDur time.Duration
}

type ntfEntry struct {
	count       int
	windowStart time.Time
	lockedUntil time.Time
}

// NewNotFoundTracker returns a tracker with sensible defaults:
// 10 404s in 60s → 5-minute lock per IP.
func NewNotFoundTracker() *notFoundTracker {
	t := &notFoundTracker{
		entries:    make(map[string]*ntfEntry),
		threshold:  10,
		window:     60 * time.Second,
		lockoutDur: 5 * time.Minute,
	}
	go t.cleanupLoop()
	return t
}

func (t *notFoundTracker) cleanupLoop() {
	tick := time.NewTicker(2 * time.Minute)
	defer tick.Stop()
	for range tick.C {
		t.mu.Lock()
		cutoff := time.Now().Add(-30 * time.Minute)
		for k, e := range t.entries {
			if e.lockedUntil.Before(cutoff) && e.windowStart.Before(cutoff) {
				delete(t.entries, k)
			}
		}
		t.mu.Unlock()
	}
}

// blocked returns true if the IP is currently in lockout.
func (t *notFoundTracker) blocked(ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.entries[ip]
	return ok && time.Now().Before(e.lockedUntil)
}

// register404 increments the counter for ip. Returns the new lockout deadline
// (zero time if not yet locked).
func (t *notFoundTracker) register404(ip string) time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	e, ok := t.entries[ip]
	if !ok {
		e = &ntfEntry{windowStart: now}
		t.entries[ip] = e
	}
	// reset window if older than threshold
	if now.Sub(e.windowStart) > t.window {
		e.count = 0
		e.windowStart = now
	}
	e.count++
	if e.count >= t.threshold && now.After(e.lockedUntil) {
		e.lockedUntil = now.Add(t.lockoutDur)
	}
	return e.lockedUntil
}

// statusCapture wraps http.ResponseWriter to remember the status code.
// status defaults to 200 (the implicit code for a body-only response that
// never calls WriteHeader) and gets overwritten by the first WriteHeader.
type statusCapture struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusCapture) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

// Middleware wraps a handler so that:
//   1. Requests from currently-locked IPs are rejected with 429.
//   2. Responses with 404 increment the IP's counter, which may produce a lock.
func (t *notFoundTracker) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if t.blocked(ip) {
			w.Header().Set("Retry-After", "300")
			http.Error(w, `{"error":"locked","message":"too many invalid tokens; try again later"}`, http.StatusTooManyRequests)
			return
		}
		sc := &statusCapture{ResponseWriter: w, status: 200}
		next.ServeHTTP(sc, r)
		if sc.status == http.StatusNotFound {
			t.register404(ip)
		}
	})
}

// clientIP extracts a stable per-client key from the request. Prefers the
// first hop of X-Forwarded-For (set by Traefik) and falls back to RemoteAddr
// with its ephemeral port stripped — otherwise every TCP connection would
// land in a fresh bucket and per-IP limits would be useless.
func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if comma := strings.Index(v, ","); comma >= 0 {
			return strings.TrimSpace(v[:comma])
		}
		return strings.TrimSpace(v)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
