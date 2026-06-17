// Package webauthn wraps go-webauthn for GoGrab's "Master-KEK unlock"
// flow. The package itself never sees PRF outputs or the master KEK: the
// browser handles the PRF extension client-side and posts back only opaque
// wrapped material that we persist alongside the credential.
//
// Session data between Begin and Finish ceremonies travels via a stateless,
// AES-GCM-encrypted token (no server-side session store needed).
package webauthn

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	gowebauthn "github.com/go-webauthn/webauthn/webauthn"

	"github.com/timhasenkamp/gograb/internal/db"
)

// Service exposes Begin/Finish helpers and packs/unpacks SessionData.
type Service struct {
	wa     *gowebauthn.WebAuthn
	secret []byte
}

// Config is the subset of server config needed to construct the WebAuthn RP.
type Config struct {
	RPDisplayName string
	RPID          string
	RPOrigins     []string
	SessionSecret []byte // 32 bytes for AES-256-GCM
}

func New(cfg Config) (*Service, error) {
	if len(cfg.SessionSecret) != 32 {
		return nil, fmt.Errorf("session secret must be 32 bytes")
	}
	wa, err := gowebauthn.New(&gowebauthn.Config{
		RPDisplayName: cfg.RPDisplayName,
		RPID:          cfg.RPID,
		RPOrigins:     cfg.RPOrigins,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			UserVerification: protocol.VerificationRequired,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("init webauthn: %w", err)
	}
	return &Service{wa: wa, secret: cfg.SessionSecret}, nil
}

// WA exposes the underlying *gowebauthn.WebAuthn so handlers can use the
// type directly if needed. Most callers should prefer the helpers below.
func (s *Service) WA() *gowebauthn.WebAuthn { return s.wa }

// --- session token codec ---------------------------------------------------

// PackSession encrypts SessionData with AES-256-GCM and returns base64url.
// The resulting token is opaque, integrity-protected, and round-trips between
// Begin and Finish without any server-side state.
func (s *Service) PackSession(sd *gowebauthn.SessionData) (string, error) {
	plain, err := json.Marshal(sd)
	if err != nil {
		return "", fmt.Errorf("marshal session: %w", err)
	}
	block, err := aes.NewCipher(s.secret)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plain, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// UnpackSession reverses PackSession. Returns an error if the token was
// tampered with, malformed, or older than 5 minutes (the ceremony TTL).
func (s *Service) UnpackSession(token string) (*gowebauthn.SessionData, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("decode session: %w", err)
	}
	block, err := aes.NewCipher(s.secret)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return nil, fmt.Errorf("session token too short")
	}
	plain, err := gcm.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return nil, fmt.Errorf("session token invalid")
	}
	var sd gowebauthn.SessionData
	if err := json.Unmarshal(plain, &sd); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	if !sd.Expires.IsZero() && time.Now().After(sd.Expires) {
		return nil, fmt.Errorf("session token expired")
	}
	return &sd, nil
}

// --- User adapter ----------------------------------------------------------

// Operator implements gowebauthn.User for a stored operator + its credentials.
type Operator struct {
	op    db.Operator
	creds []db.WebauthnCredential
}

func NewOperator(op db.Operator, creds []db.WebauthnCredential) *Operator {
	return &Operator{op: op, creds: creds}
}

func (o *Operator) WebAuthnID() []byte {
	// 16-byte UUID is well within the 64-byte WebAuthn limit.
	id := o.op.ID
	return id[:]
}

func (o *Operator) WebAuthnName() string {
	return o.op.Username
}

func (o *Operator) WebAuthnDisplayName() string {
	if o.op.Email != "" {
		return o.op.Email
	}
	return o.op.Username
}

func (o *Operator) WebAuthnCredentials() []gowebauthn.Credential {
	out := make([]gowebauthn.Credential, len(o.creds))
	for i, c := range o.creds {
		out[i] = gowebauthn.Credential{
			ID:        c.CredentialID,
			PublicKey: c.PublicKey,
			Authenticator: gowebauthn.Authenticator{
				AAGUID:    c.Aaguid,
				SignCount: uint32(c.SignCount),
			},
			Transport: parseTransports(c.Transports),
		}
	}
	return out
}

func parseTransports(in []string) []protocol.AuthenticatorTransport {
	out := make([]protocol.AuthenticatorTransport, 0, len(in))
	for _, t := range in {
		out = append(out, protocol.AuthenticatorTransport(t))
	}
	return out
}

// SerializeTransports flattens the typed transports for DB storage.
func SerializeTransports(in []protocol.AuthenticatorTransport) []string {
	out := make([]string, 0, len(in))
	for _, t := range in {
		out = append(out, string(t))
	}
	return out
}
