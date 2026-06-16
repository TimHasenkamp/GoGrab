// Package auth carries the authenticated User identity through request
// context. It used to host Authentik forward-auth middleware; that has been
// replaced by GoGrab's first-party WebAuthn session (see internal/session and
// internal/handlers/login.go). The User struct + context helpers remain so
// existing handlers can stay agnostic about how the identity was established.
package auth

import "context"

type ctxKey struct{}

// User is the authenticated principal attached to a request context. The
// session middleware populates it from the operator row pointed at by the
// signed cookie; the dev-shim populates it from GOGRAB_DEV_USER.
type User struct {
	Username string
	Email    string
}

// WithUser returns ctx with u attached.
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// FromContext extracts a User placed by WithUser. Returns ok=false when no
// authenticated user is present on this request.
func FromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}
