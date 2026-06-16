// Package session implements GoGrab's first-party login session.
//
// A session is a tuple {operator_id, expires_at} serialised as
//
//	<operator_id_b64>.<expires_unix>.<hmac_b64>
//
// where the HMAC is computed over the first two fields with a key derived
// from GOGRAB_SESSION_SECRET via HKDF-SHA256 using a session-specific label.
// The same env var also seeds the WebAuthn ceremony AES key — they share a
// secret but use distinct sub-keys so a compromise of one half does not let
// you forge the other.
//
// The cookie is HttpOnly, Secure (set conditionally — see Manager.SetCookie)
// and SameSite=Lax. Expiry is rolling: SetCookie issues a fresh signature
// each time the manager renews a session.
package session

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/hkdf"
)

// CookieName is the HTTP cookie that carries a signed session.
const CookieName = "gograb_session"

// DefaultLifetime is how long a new session stays valid after issuance.
const DefaultLifetime = 30 * 24 * time.Hour

// Session is the payload encoded into the session cookie. Constructed by
// Manager.Issue and read back by Manager.FromRequest.
type Session struct {
	OperatorID uuid.UUID
	ExpiresAt  time.Time
}

// Manager signs and verifies session cookies. Construct once at startup with
// the raw GOGRAB_SESSION_SECRET; the manager derives an HMAC sub-key from it
// internally.
type Manager struct {
	hmacKey []byte
	// Secure controls the cookie's Secure flag. Production: true. Local dev
	// without TLS: false (browsers reject Secure cookies on http://).
	Secure bool
	// Lifetime is how long Issue() makes new sessions valid for.
	Lifetime time.Duration
}

// NewManager derives the session HMAC key from secret via HKDF-SHA256 and
// returns a manager configured with sensible defaults.
func NewManager(secret []byte, secure bool) (*Manager, error) {
	if len(secret) < 16 {
		return nil, fmt.Errorf("session: secret too short (%d bytes, need ≥16)", len(secret))
	}
	r := hkdf.New(sha256.New, secret, nil, []byte("gograb.session.v1"))
	key := make([]byte, 32)
	if _, err := r.Read(key); err != nil {
		return nil, fmt.Errorf("session: derive key: %w", err)
	}
	return &Manager{
		hmacKey:  key,
		Secure:   secure,
		Lifetime: DefaultLifetime,
	}, nil
}

// Issue returns a new Session bound to operatorID, valid for the manager's
// Lifetime. Expiry is truncated to the second so it round-trips losslessly
// through the cookie's integer-seconds encoding.
func (m *Manager) Issue(operatorID uuid.UUID) Session {
	return Session{
		OperatorID: operatorID,
		ExpiresAt:  time.Now().Add(m.Lifetime).Truncate(time.Second),
	}
}

// sign produces the token form: opID.expUnix.hmac
func (m *Manager) sign(s Session) string {
	idStr := base64.RawURLEncoding.EncodeToString(s.OperatorID[:])
	expStr := strconv.FormatInt(s.ExpiresAt.Unix(), 10)
	payload := idStr + "." + expStr
	mac := hmac.New(sha256.New, m.hmacKey)
	mac.Write([]byte(payload))
	sigStr := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + "." + sigStr
}

// verify parses a token and constant-time-checks the HMAC. Returns ErrExpired
// when the signature is valid but the expiry passed.
func (m *Manager) verify(token string) (Session, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Session{}, errors.New("session: malformed token")
	}
	payload := parts[0] + "." + parts[1]
	wantMAC, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Session{}, errors.New("session: malformed signature")
	}
	mac := hmac.New(sha256.New, m.hmacKey)
	mac.Write([]byte(payload))
	if !hmac.Equal(wantMAC, mac.Sum(nil)) {
		return Session{}, errors.New("session: bad signature")
	}
	idBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(idBytes) != 16 {
		return Session{}, errors.New("session: bad operator id")
	}
	var id uuid.UUID
	copy(id[:], idBytes)
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return Session{}, errors.New("session: bad expiry")
	}
	expTime := time.Unix(exp, 0)
	if time.Now().After(expTime) {
		return Session{}, ErrExpired
	}
	return Session{OperatorID: id, ExpiresAt: expTime}, nil
}

// ErrExpired is returned by Manager.FromRequest when the cookie is present
// and signed correctly but past its expiry.
var ErrExpired = errors.New("session: expired")

// FromRequest returns the session encoded in the request cookie. Errors
// distinguish three cases: no cookie at all (http.ErrNoCookie), bad
// signature/format (generic error), or ErrExpired.
func (m *Manager) FromRequest(r *http.Request) (Session, error) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return Session{}, err
	}
	return m.verify(c.Value)
}

// SetCookie writes a signed cookie carrying s. Path is /, SameSite=Lax,
// HttpOnly. The Secure flag follows m.Secure.
func (m *Manager) SetCookie(w http.ResponseWriter, s Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    m.sign(s),
		Path:     "/",
		Expires:  s.ExpiresAt,
		MaxAge:   int(time.Until(s.ExpiresAt).Seconds()),
		HttpOnly: true,
		Secure:   m.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearCookie writes an expired cookie to instruct the browser to drop the
// session. Use on logout.
func (m *Manager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// --- context plumbing -----------------------------------------------------

type ctxKey struct{}

// WithSession returns ctx with s attached. Public so tests can craft a ctx
// without going through HTTP.
func WithSession(ctx context.Context, s Session) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// FromContext extracts a Session placed by Middleware or WithSession.
func FromContext(ctx context.Context) (Session, bool) {
	s, ok := ctx.Value(ctxKey{}).(Session)
	return s, ok
}
