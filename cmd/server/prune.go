package main

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/timhasenkamp/gograb/internal/db"
)

// runPruneAuditCommand backs the `gograb prune-audit [days]` subcommand.
// Deletes audit_log rows older than `days` (default 180). Operators can
// schedule this via cron / systemd timer:
//
//	0 4 * * *  /app/gograb prune-audit
//
// Idempotent. Logs the row count.
func runPruneAuditCommand(ctx context.Context, args []string, log *slog.Logger, dbURL string) error {
	days := 180
	if len(args) > 0 && args[0] != "" {
		n, err := strconv.Atoi(args[0])
		if err != nil || n <= 0 {
			return fmt.Errorf("invalid days argument %q (must be a positive integer)", args[0])
		}
		days = n
	}
	cutoff := time.Now().AddDate(0, 0, -days)

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	q := db.New(pool)
	deleted, err := q.PruneAuditOlderThan(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true})
	if err != nil {
		return fmt.Errorf("prune: %w", err)
	}
	log.Info("audit log pruned", "rows_deleted", deleted, "older_than_days", days, "cutoff", cutoff.UTC().Format(time.RFC3339))
	return nil
}
