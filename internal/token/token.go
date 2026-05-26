// Package token generates URL-safe one-time tokens via crypto/rand.
package token

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// Length is the number of random bytes per token. 16 bytes encodes to 22
// base64url characters (no padding) and provides 128 bits of entropy — far
// beyond what's needed to make tokens unguessable, while keeping URLs short.
const Length = 16

// New returns a fresh base64url-encoded random token without padding.
func New() (string, error) {
	buf := make([]byte, Length)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
