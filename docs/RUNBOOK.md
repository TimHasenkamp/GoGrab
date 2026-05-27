# GoGrab — Operator Runbook

Step-by-step for the situations that come up when you actually run this
thing. Read [THREAT_MODEL.md](THREAT_MODEL.md) once for context, then keep
this nearby.

---

## "I lost my primary YubiKey"

Pre-condition: you previously registered a backup credential. If you didn't,
see [No backup credential — what now?](#no-backup-credential--what-now)
below.

1. Plug in the backup YubiKey.
2. Open the admin UI. The unlock prompt appears.
3. Tap the backup key — session unlocks normally.
4. Navigate to `/admin/security`.
5. The lost primary key is still in the list. Click **Entfernen** on its
   row. The server requires that at least one credential remain; with the
   backup unlocked, removal of the primary succeeds.
6. Register a replacement primary: plug in a new YubiKey → **Backup-Key
   hinzufügen** → label it (e.g. "YubiKey Schreibtisch v2") → tap.
7. The new credential's `wrapped_master` is computed by wrapping the
   in-memory master KEK with the new key's PRF output. Both backup and new
   primary now unlock the same data.

If the lost YubiKey ever turns up: it can still unlock until step 5. Don't
skip that step.

## No backup credential — what now?

You're locked out of all wrapped data. There is no recovery path on the
server side by design.

- For *future* requests: register a fresh primary on a new YubiKey at
  `/admin/setup` (the operator row will offer setup again once all
  credentials are gone — server enforces this via `CountCredentialsByOperator`).
- For *existing* unretrieved requests: gone. The wrapped per-request keys
  cannot be unwrapped without the master KEK. Cancel them and apologise
  to the customer.

The lesson: register a backup on day one.

## "I want to change my Authentik password / username"

Authentik identity ↔ GoGrab `operators.username` is the binding. The
username is what GoGrab uses to look up the operator row. If it changes
*upstream*:

- New username = new operator row → new setup, no access to old wrapped data
- Same username = no impact, your YubiKeys still unlock

Therefore: keep the Authentik username stable, or migrate manually before
changing it (see below).

To migrate operator data across a username change:
```sql
-- DANGER: only run when you know what you're doing, after a backup.
UPDATE operators SET username = 'new-username' WHERE username = 'old-username';
```
Authentik is the only thing that injects that username via the header.
After the SQL update + Authentik change, the next login finds the renamed
row and everything continues to work.

## "Authentik is down or compromised — emergency access"

GoGrab's admin endpoints are gated by Authentik forward-auth. If Authentik
is down you have **no** admin access. Options:

1. **Bypass via `GOGRAB_DEV_USER`**: stop the app, set
   `GOGRAB_DEV_USER=your-username` and `GOGRAB_TRUSTED_PROXY=0`, restart.
   The dev middleware injects a fixed user, bypassing forward-auth. **Only
   do this on a host you control physically/network-wise.** Authentik
   compromise is bad enough — don't expose this dev-mode endpoint to the
   internet.
2. **Direct DB inspection** while Authentik is down lets you read metadata
   (descriptions, statuses) but not plaintext. Useful for triage.

After Authentik recovery: rotate any credentials that may have been
exfiltrated, and review the audit log for `request.retrieve` or
`credential.register` events you didn't initiate.

## "I need to deploy a new release"

```bash
docker compose pull            # if using tagged images from ghcr.io
docker compose up -d           # rolling restart
# If GOGRAB_MIGRATE_ON_BOOT=1, migrations apply on start.
# Otherwise:
docker compose exec app /app/gograb migrate up
```

Check `docker compose logs app | head -20` for `migrations applied` (if
auto) or for `webauthn ready` indicating a clean start. The migration step
is idempotent — running it twice does nothing the second time.

## "I want to migrate to a new server"

1. **Backup**: dump the DB plus the `GOGRAB_SESSION_SECRET` value from
   `.env`. The session secret only matters for in-flight WebAuthn ceremonies
   (seconds), so technically optional; but stable across restarts is nice.
2. **Restore**: `pg_restore` into the new Postgres. Set the same env vars
   (`GOGRAB_RP_ID`, `GOGRAB_RP_ORIGINS`, `GOGRAB_DATABASE_URL`) on the new
   host.
3. **Constraint**: the new host must use the **same `RPID`** (the domain
   name in WebAuthn). Existing credentials are bound to that domain. If
   the domain changes, all existing credentials are invalidated → run
   through "No backup credential — what now?" for each operator.

## "The audit log is getting huge"

```bash
docker compose exec app /app/gograb prune-audit 90   # 90 days retention
```

Schedule daily via cron / systemd timer:
```
0 4 * * *  docker compose exec -T app /app/gograb prune-audit 180
```

## "Postgres is full"

Two main growth vectors:
1. `audit_log` — see above.
2. `requests` where status is `retrieved` or `expired`. Metadata only
   (no ciphertext after purge) — small per row, but unbounded. SQL prune:
   ```sql
   DELETE FROM requests
     WHERE status IN ('retrieved', 'expired')
     AND created_at < NOW() - INTERVAL '180 days';
   ```
   No dedicated subcommand yet — feel free to PR one.

## "I want to test changes without affecting prod"

```bash
# Spin up a parallel docker-compose project pointing at a separate DB.
COMPOSE_PROJECT_NAME=gograb-staging docker compose up -d
```

Or run `go run ./cmd/server` against a throwaway Postgres on a different
port. With `GOGRAB_DEV_USER=test` set, you skip Authentik entirely.

## "A customer says they got an error on the submit page"

Check the audit log for `request.view` entries on that token. If they exist
but `request.submit` does not, the customer reached the page but something
broke client-side. Likely culprits:

- Their browser doesn't support WebCrypto subtle API (very old, very rare).
  The page errors out at module load.
- The link's `#fragment` got stripped en route (mail clients sometimes do
  this when wrapping URLs). Verify the link they actually have ends with
  `#<long-base64-string>`.
- They opened the URL in a context that strips the fragment (e.g. their
  password manager auto-fill flow). Send the URL as a *plain text* attachment
  if possible.

## "I need to revoke a customer's link"

Open `/admin/<request-id>` and click **Request löschen** in the pending
panel. The row is hard-deleted; the share URL now 404s.
