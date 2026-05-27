package webauthn

import (
	"bytes"
	"crypto/rand"
	"testing"
	"time"

	gowebauthn "github.com/go-webauthn/webauthn/webauthn"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatalf("rand: %v", err)
	}
	svc, err := New(Config{
		RPDisplayName: "GoGrab Test",
		RPID:          "localhost",
		RPOrigins:     []string{"http://localhost:8080"},
		SessionSecret: secret,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return svc
}

func TestPackUnpackSessionRoundtrip(t *testing.T) {
	svc := newTestService(t)
	original := &gowebauthn.SessionData{
		Challenge: "test-challenge-abc",
		UserID:    []byte{0x01, 0x02, 0x03},
		Expires:   time.Now().Add(5 * time.Minute),
	}

	token, err := svc.PackSession(original)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}

	got, err := svc.UnpackSession(token)
	if err != nil {
		t.Fatalf("unpack: %v", err)
	}
	if got.Challenge != original.Challenge {
		t.Errorf("challenge = %q, want %q", got.Challenge, original.Challenge)
	}
	if !bytes.Equal(got.UserID, original.UserID) {
		t.Errorf("user id mismatch: got %x, want %x", got.UserID, original.UserID)
	}
}

func TestUnpackSession_RejectsTampered(t *testing.T) {
	svc := newTestService(t)
	token, err := svc.PackSession(&gowebauthn.SessionData{
		Challenge: "x", Expires: time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	// Flip a byte in the middle of the encoded blob.
	tampered := []byte(token)
	tampered[len(tampered)/2] ^= 0xFF
	if _, err := svc.UnpackSession(string(tampered)); err == nil {
		t.Fatal("expected tamper rejection, got nil")
	}
}

func TestUnpackSession_RejectsExpired(t *testing.T) {
	svc := newTestService(t)
	token, err := svc.PackSession(&gowebauthn.SessionData{
		Challenge: "x",
		Expires:   time.Now().Add(-1 * time.Second),
	})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if _, err := svc.UnpackSession(token); err == nil {
		t.Fatal("expected expiry rejection, got nil")
	}
}

func TestUnpackSession_RejectsWrongSecret(t *testing.T) {
	svcA := newTestService(t)
	svcB := newTestService(t) // different random secret
	token, err := svcA.PackSession(&gowebauthn.SessionData{
		Challenge: "x", Expires: time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if _, err := svcB.UnpackSession(token); err == nil {
		t.Fatal("expected wrong-secret rejection")
	}
}

func TestNew_RejectsBadSecret(t *testing.T) {
	if _, err := New(Config{
		RPID: "x", RPOrigins: []string{"http://x"}, SessionSecret: make([]byte, 16),
	}); err == nil {
		t.Fatal("expected error for 16-byte secret")
	}
}
