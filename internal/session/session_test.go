package session

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i)
	}
	m, err := NewManager(secret, false)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return m
}

func TestSignVerifyRoundtrip(t *testing.T) {
	m := newTestManager(t)
	op := uuid.New()
	s := m.Issue(op)
	tok := m.sign(s)
	got, err := m.verify(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.OperatorID != op {
		t.Errorf("operator id mismatch: got %v want %v", got.OperatorID, op)
	}
	if !got.ExpiresAt.Equal(s.ExpiresAt) {
		t.Errorf("expiry drift: got %v want %v", got.ExpiresAt, s.ExpiresAt)
	}
}

func TestVerifyRejectsTamperedSignature(t *testing.T) {
	m := newTestManager(t)
	s := m.Issue(uuid.New())
	tok := m.sign(s)
	// flip a byte in the signature region (last segment)
	idx := strings.LastIndex(tok, ".") + 1
	bad := tok[:idx] + flipChar(tok[idx:idx+1]) + tok[idx+1:]
	if _, err := m.verify(bad); err == nil {
		t.Fatal("expected bad-signature error, got nil")
	}
}

func TestVerifyRejectsWrongKey(t *testing.T) {
	m1 := newTestManager(t)
	other := make([]byte, 32)
	for i := range other {
		other[i] = byte(0xff - i)
	}
	m2, err := NewManager(other, false)
	if err != nil {
		t.Fatalf("new m2: %v", err)
	}
	tok := m1.sign(m1.Issue(uuid.New()))
	if _, err := m2.verify(tok); err == nil {
		t.Fatal("manager with different secret accepted forged token")
	}
}

func TestVerifyDetectsExpiry(t *testing.T) {
	m := newTestManager(t)
	s := Session{OperatorID: uuid.New(), ExpiresAt: time.Now().Add(-time.Minute)}
	tok := m.sign(s)
	if _, err := m.verify(tok); err != ErrExpired {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}

func TestFromRequestNoCookie(t *testing.T) {
	m := newTestManager(t)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := m.FromRequest(r)
	if err != http.ErrNoCookie {
		t.Fatalf("want http.ErrNoCookie, got %v", err)
	}
}

func TestSetThenFromRequest(t *testing.T) {
	m := newTestManager(t)
	op := uuid.New()
	s := m.Issue(op)
	rec := httptest.NewRecorder()
	m.SetCookie(rec, s)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		r.AddCookie(c)
	}
	got, err := m.FromRequest(r)
	if err != nil {
		t.Fatalf("from request: %v", err)
	}
	if got.OperatorID != op {
		t.Errorf("operator id mismatch: got %v want %v", got.OperatorID, op)
	}
}

func TestNewManagerRejectsShortSecret(t *testing.T) {
	if _, err := NewManager([]byte("short"), false); err == nil {
		t.Fatal("expected error for short secret")
	}
}

func flipChar(s string) string {
	if s == "" {
		return s
	}
	c := s[0]
	if c == 'A' {
		return "B"
	}
	return "A"
}
