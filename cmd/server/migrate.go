package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/timhasenkamp/gograb/migrations"
)

// runMigrations applies all pending goose migrations from the embedded
// `migrations` package against the given pgxpool. Bridges to database/sql
// via pgx's stdlib adapter — goose still uses sql.DB under the hood.
func runMigrations(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	sqldb := stdlib.OpenDBFromPool(pool)
	defer sqldb.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}
	goose.SetLogger(gooseSlog{log: log})

	if err := goose.UpContext(ctx, sqldb, "."); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}

// runMigrateCommand backs the `gograb migrate [up|down|status|version]`
// subcommand. Opens its own pool, applies the requested action, and exits.
func runMigrateCommand(ctx context.Context, action string, log *slog.Logger, dbURL string) error {
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()
	sqldb := stdlib.OpenDBFromPool(pool)
	defer sqldb.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	goose.SetLogger(gooseSlog{log: log})

	switch action {
	case "", "up":
		return goose.UpContext(ctx, sqldb, ".")
	case "down":
		return goose.DownContext(ctx, sqldb, ".")
	case "status":
		return goose.StatusContext(ctx, sqldb, ".")
	case "version":
		return goose.VersionContext(ctx, sqldb, ".")
	case "redo":
		return goose.RedoContext(ctx, sqldb, ".")
	case "reset":
		return goose.ResetContext(ctx, sqldb, ".")
	default:
		return fmt.Errorf("unknown migrate action %q (use up | down | status | version | redo | reset)", action)
	}
}

// gooseSlog adapts goose's logger interface onto slog.
type gooseSlog struct{ log *slog.Logger }

func (g gooseSlog) Fatalf(fmtStr string, args ...any) {
	g.log.Error("goose: " + fmt.Sprintf(fmtStr, args...))
}
func (g gooseSlog) Printf(fmtStr string, args ...any) {
	g.log.Info("goose: " + fmt.Sprintf(fmtStr, args...))
}
