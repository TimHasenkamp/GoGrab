package auth

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestMiddleware_RejectsWhenUntrustedProxy(t *testing.T) {
	called := false
	h := Middleware(false, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	h := Middleware(true, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	h := Middleware(true, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestMiddleware_RejectsRemoteAddrOutsideTrustedCIDRs(t *testing.T) {
	cidrs := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	h := Middleware(true, cidrs)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for an untrusted source")
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("X-Authentik-Username", "alice")
	req.RemoteAddr = "203.0.113.1:1234" // public IP, NOT in 10.0.0.0/8
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMiddleware_AcceptsRemoteAddrInsideTrustedCIDRs(t *testing.T) {
	cidrs := []netip.Prefix{
		netip.MustParsePrefix("172.18.0.0/16"), // a typical Docker bridge net
	}
	got := ""
	h := Middleware(true, cidrs)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := FromContext(r.Context()); ok {
			got = u.Username
		}
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("X-Authentik-Username", "bob")
	req.RemoteAddr = "172.18.0.7:51234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got != "bob" {
		t.Errorf("user = %q, want bob", got)
	}
}

func TestMiddleware_HandlesIPv4MappedV6(t *testing.T) {
	cidrs := []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
	h := Middleware(true, cidrs)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("X-Authentik-Username", "carol")
	// ::ffff:127.0.0.1 is 127.0.0.1 in IPv4-mapped form. Unmap() in the
	// middleware should normalise this.
	req.RemoteAddr = "[::ffff:127.0.0.1]:51234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (IPv4-mapped should match)", rec.Code)
	}
}
