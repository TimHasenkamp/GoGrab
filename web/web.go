// Package web exposes the embedded SvelteKit build output.
//
// The Docker build runs `npm run build` in this directory before compiling
// the Go binary, populating ./build with hashed assets and an index.html.
// A placeholder.txt keeps the directory non-empty so go:embed succeeds even
// before the first frontend build; the SPA handler falls back gracefully
// when only the placeholder is present.
package web

import "embed"

//go:embed all:build
var FS embed.FS
