package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestTracker(threshold int, window, lockout time.Duration) *notFoundTracker {
	return &notFoundTracker{
		entries:    make(map[string]*ntfEntry),
		threshold:  threshold,
		window:     window,
		lockoutDur: lockout,
	}
}

func TestNotFoundTracker_LocksAfterThreshold(t *testing.T) {
	tr := newTestTracker(3, time.Minute, time.Minute)
	const ip = "10.0.0.1"

	if tr.blocked(ip) {
		t.Fatal("unexpectedly blocked before any 404")
	}
	// Below threshold: no lock.
	tr.register404(ip)
	tr.register404(ip)
	if tr.blocked(ip) {
		t.Error("blocked after 2/3 404s")
	}
	// Reach threshold: lock.
	tr.register404(ip)
	if !tr.blocked(ip) {
		t.Error("not blocked after hitting threshold")
	}
}

func TestNotFoundTracker_WindowResets(t *testing.T) {
	tr := newTestTracker(3, time.Millisecond, time.Minute)
	const ip = "10.0.0.2"
	tr.register404(ip)
	tr.register404(ip)
	// Wait for window to expire.
	time.Sleep(5 * time.Millisecond)
	tr.register404(ip)
	if tr.blocked(ip) {
		t.Error("blocked even though count reset across the window boundary")
	}
}

func TestNotFoundTracker_DistinctIPs(t *testing.T) {
	tr := newTestTracker(2, time.Minute, time.Minute)
	tr.register404("1.1.1.1")
	tr.register404("1.1.1.1")
	tr.register404("2.2.2.2")
	if !tr.blocked("1.1.1.1") {
		t.Error("ip 1 should be locked")
	}
	if tr.blocked("2.2.2.2") {
		t.Error("ip 2 should NOT be locked (count = 1)")
	}
}

func TestNotFoundTracker_Middleware_BlockedReturns429(t *testing.T) {
	tr := newTestTracker(1, time.Minute, time.Minute)
	called := 0
	h := tr.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusNotFound)
	}))

	// First request — passes through and increments counter (which hits threshold).
	// Different ephemeral source ports MUST NOT split the bucket — they
	// always do on real traffic.
	req1 := httptest.NewRequest("GET", "/api/requests/abc", nil)
	req1.RemoteAddr = "9.9.9.9:1234"
	h.ServeHTTP(httptest.NewRecorder(), req1)

	// Second request from same IP, different ephemeral port — should be blocked.
	req2 := httptest.NewRequest("GET", "/api/requests/def", nil)
	req2.RemoteAddr = "9.9.9.9:1235"
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	if called != 1 {
		t.Errorf("inner handler should be called once (before lock), got %d", called)
	}
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("second request status = %d, want 429", rec2.Code)
	}
	if rec2.Header().Get("Retry-After") == "" {
		t.Error("missing Retry-After on lockout response")
	}
}
