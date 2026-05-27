// Package migrations exposes the embedded goose .sql migration files so the
// binary can apply them at boot or via the `gograb migrate` subcommand
// without depending on a separately-installed goose CLI in production.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
