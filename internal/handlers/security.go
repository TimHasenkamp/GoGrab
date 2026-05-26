package handlers

import (
	"net/http"
	"strings"
)

// SecurityHeaders applies a strict baseline of browser-side defenses. Intended
// to wrap the entire router so HTML, API JSON and static assets all carry the
// same security headers.
//
// CSP note: SvelteKit's adapter-static index.html contains an inline boot
// script (`window.__sveltekit_*`). To allow it without machinery to inject
// nonces into the embedded index.html at runtime, script-src/style-src use
// 'unsafe-inline'. This is acceptable here because we never render
// user-controlled content as HTML — descriptions, secrets and audit fields
// are bound as text by Svelte. If that invariant changes, switch to
// nonce-based CSP.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", strings.Join([]string{
			"accelerometer=()",
			"camera=()",
			"geolocation=()",
			"gyroscope=()",
			"magnetometer=()",
			"microphone=()",
			"payment=()",
			"usb=()",
			"interest-cohort=()",
		}, ", "))
		// HSTS — assume operator runs TLS in prod (Traefik); harmless otherwise.
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		h.Set("Content-Security-Policy", strings.Join([]string{
			"default-src 'self'",
			"script-src 'self' 'unsafe-inline'",
			"style-src 'self' 'unsafe-inline'",
			"img-src 'self' data:",
			"connect-src 'self'",
			"font-src 'self'",
			"base-uri 'self'",
			"form-action 'self'",
			"frame-ancestors 'none'",
			"object-src 'none'",
		}, "; "))
		// Cache rules: API and HTML must not be cached; immutable hashed
		// assets under /_app/immutable/ get a long cache. Static file server
		// inside the SPA handler doesn't set Cache-Control, so we do it here.
		path := r.URL.Path
		switch {
		case strings.HasPrefix(path, "/api/"):
			h.Set("Cache-Control", "no-store")
		case strings.HasPrefix(path, "/_app/immutable/"):
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		default:
			// HTML / index.html fallback
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
