// Package auth contains middleware for Authentik forward-auth.
//
// In production, Traefik fronts the app and forwards requests to Authentik's
// outpost. After successful auth the outpost rewrites the request and adds
// the X-Authentik-Username and X-Authentik-Email headers. Traefik then sends
// the request to the app.
//
// Because these headers are spoofable by anyone with direct network access
// to the app, the deployment must ensure the app is only reachable through
// Traefik (Docker network isolation). The TrustedProxy config flag is a
// safety toggle: when false, the middleware refuses to honor the headers
// at all, which is useful for local dev without Authentik.
package auth

import (
	"context"
	"net/http"
)

type ctxKey struct{}

// User is the authenticated principal extracted from Authentik headers.
type User struct {
	Username string
	Email    string
}

const (
	headerUsername = "X-Authentik-Username"
	headerEmail    = "X-Authentik-Email"
)

// Middleware returns an http middleware that requires an Authentik-authenticated
// user. When trustedProxy is true, the X-Authentik-* headers are read; otherwise
// the middleware fails closed (HTTP 401) — preventing accidental exposure.
func Middleware(trustedProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !trustedProxy {
				http.Error(w, `{"error":"unauthorized","message":"forward-auth disabled"}`, http.StatusUnauthorized)
				return
			}
			username := r.Header.Get(headerUsername)
			if username == "" {
				http.Error(w, `{"error":"unauthorized","message":"missing X-Authentik-Username"}`, http.StatusUnauthorized)
				return
			}
			u := User{Username: username, Email: r.Header.Get(headerEmail)}
			ctx := context.WithValue(r.Context(), ctxKey{}, u)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// FromContext extracts the User placed by Middleware. Returns ok=false if
// the request was not run through the middleware.
func FromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}

// WithUser returns ctx with u attached. Exposed for tests; production code
// should reach a handler via Middleware / DevMiddleware instead.
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// DevMiddleware injects a fixed user for local development without Authentik.
// Never use in production.
func DevMiddleware(username, email string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := User{Username: username, Email: email}
			ctx := context.WithValue(r.Context(), ctxKey{}, u)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
