package token

import (
	"encoding/base64"
	"testing"
)

func TestNew_LengthAndAlphabet(t *testing.T) {
	for i := 0; i < 32; i++ {
		tok, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if len(tok) != 22 {
			t.Errorf("len(tok) = %d, want 22 (tok=%q)", len(tok), tok)
		}
		if _, err := base64.RawURLEncoding.DecodeString(tok); err != nil {
			t.Errorf("not valid base64url: %v (tok=%q)", err, tok)
		}
	}
}

func TestNew_Unique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		tok, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, dup := seen[tok]; dup {
			t.Fatalf("duplicate token: %s", tok)
		}
		seen[tok] = struct{}{}
	}
}
