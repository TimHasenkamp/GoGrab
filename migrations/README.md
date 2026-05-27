# Migrations

Goose-format SQL migrations. Embedded into the Go binary via
[`migrations.go`](./migrations.go) so the deployed image carries them and
no separate `goose` CLI is needed in production.

## Applying

Three equivalent paths:

```bash
# 1. embedded subcommand (preferred in prod)
/app/gograb migrate up

# 2. on every boot (set in .env)
GOGRAB_MIGRATE_ON_BOOT=1

# 3. external goose CLI against the DB directly
goose -dir migrations postgres "$GOGRAB_DATABASE_URL" up
```

Other actions: `down`, `status`, `version`, `redo`, `reset`.

## Existing migrations

| File | Up | Down |
|---|---|---|
| `00001_create_requests.sql` | v1 schema: `requests` table with token/description/ciphertext/iv | `DROP TABLE requests` |
| `00002_v2_envelope_schema.sql` | Drops v1 `requests` (clean cut, pre-prod). Creates `operators`, `webauthn_credentials`, envelope-style `requests` (operator_id FK, wrapped_key/wrap_iv columns), and `audit_log`. | Drops v2 tables, restores v1 `requests` schema for symmetry |
| `00003_form_schema.sql` | Adds `requests.form_schema JSONB NOT NULL` with a single-textarea default to keep existing rows valid | Drops the column |

## Writing a new migration

1. Pick the next number, e.g. `00004_…`.
2. Create the file with the goose convention:
   ```sql
   -- +goose Up
   -- +goose StatementBegin
   ALTER TABLE requests ADD COLUMN ... ;
   -- +goose StatementEnd

   -- +goose Down
   -- +goose StatementBegin
   ALTER TABLE requests DROP COLUMN ... ;
   -- +goose StatementEnd
   ```
3. If you also change query column orderings, **re-run sqlc**:
   ```bash
   sqlc generate
   ```
   The generated `*.go` files under `internal/db` will rebase against the
   updated schema. Watch out for changed parameter/return field orderings:
   if a SELECT changes column order but the corresponding `models.go`
   doesn't, sqlc generates a per-query `Row` struct (workable, but cleaner
   to align them).
4. Test the up + down locally:
   ```bash
   gograb migrate up
   gograb migrate down
   gograb migrate up
   ```
5. Commit the migration AND the regenerated sqlc output in the same commit.

## Why no separate migrations binary?

Considered. Decided against because:

- Multi-arch image builds become more complex (two binaries to ship).
- Coordinating "migrate before app boots" between two containers is fiddly
  in docker-compose without `depends_on` hacks.
- Embedding into the same binary makes "run migrations" a property of *the
  release artifact* — there's no possibility of mismatch between the
  migrations the binary expects and the ones the operator ran.

Trade-off: the production image has the goose library compiled in. Small
binary-size cost in exchange for a friendlier ops story.
