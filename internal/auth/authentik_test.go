package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddleware_RejectsWhenUntrustedProxy(t *testing.T) {
	called := false
	h := Middleware(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("X-Authentik-Username", "evil")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if called {
		t.Error("handler was called despite untrusted proxy")
	}
}

func TestMiddleware_RejectsMissingHeader(t *testing.T) {
	h := Middleware(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMiddleware_PopulatesUser(t *testing.T) {
	var got User
	h := Middleware(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := FromContext(r.Context())
		if !ok {
			t.Fatal("no user in context")
		}
		got = u
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("X-Authentik-Username", "alice")
	req.Header.Set("X-Authentik-Email", "alice@example.com")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got.Username != "alice" || got.Email != "alice@example.com" {
		t.Errorf("user = %+v, want {alice alice@example.com}", got)
	}
}
