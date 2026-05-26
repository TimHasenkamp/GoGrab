# GoGrab

Self-hosted, DSGVO-compliant **secret-request** service — the reverse of Yopass.

An operator creates a one-time *request link*, sends it to a customer, and the
customer enters a secret into a simple page. The secret is **encrypted in the
customer's browser** with a key the server never sees, retrievable exactly
once by the operator.

## How it works

1. Operator clicks **New request** in the admin UI.
2. Browser generates a fresh AES-256 key (Web Crypto API). Server only learns
   a description + expiry; the key never leaves the operator's browser.
3. Browser builds the share URL: `https://gograb.example.com/r/<token>#<key>`.
   The fragment after `#` is **never** sent by browsers in HTTP requests, so
   the server has no chance to see the key.
4. Customer opens the link, sees the description, enters the secret. The
   browser encrypts with AES-256-GCM and POSTs `{ciphertext, iv}`.
5. Operator opens the request detail, pastes the original share URL (or just
   the key). The admin UI fetches the ciphertext, decrypts in-browser, and the
   server **deletes the ciphertext** on the same call. One-shot.

The server holds ciphertext + IV only between submit and retrieve. There is no
server-side key escrow.

## Stack

- Go 1.25 stdlib `net/http`, `pgx/v5`, sqlc, goose
- SvelteKit 2 (`adapter-static`) + Svelte 5 Runes + TypeScript strict
- TailwindCSS for the admin SPA; vanilla CSS for the public submit page
- Authentik forward-auth via Traefik for `/admin/*` and `/api/admin/*`
- Distroless static container, Linux ARM64 target

## Layout

```
cmd/server/        single Go binary, embeds web/build via go:embed
internal/auth/     Authentik forward-auth middleware
internal/config/   env-driven config
internal/db/       sqlc-generated queries
internal/handlers/ HTTP handlers
internal/notify/   webhook dispatcher (v1)
internal/token/    crypto/rand URL-safe token generator
migrations/        goose .sql migrations
web/               SvelteKit project (Tailwind admin + minimal public page)
web/web.go         //go:embed of web/build/
Dockerfile         node → go → distroless
docker-compose.yml app + postgres + Traefik labels (Authentik forward-auth)
```

## Quickstart (local dev)

```bash
# 1. Start a local Postgres (docker is fine)
docker run --name gograb-pg -e POSTGRES_PASSWORD=gograb -e POSTGRES_USER=gograb \
  -e POSTGRES_DB=gograb -p 5432:5432 -d postgres:16-alpine

# 2. Apply migrations
export GOGRAB_DATABASE_URL="postgres://gograb:gograb@localhost:5432/gograb?sslmode=disable"
goose -dir migrations postgres "$GOGRAB_DATABASE_URL" up

# 3. Build the frontend (populates web/build/ for go:embed)
make build-web

# 4. Run the server with a dev user (no Authentik required)
GOGRAB_DEV_USER=alice GOGRAB_TRUSTED_PROXY=0 make run
# in another shell:
cd web && npm run dev   # vite proxies /api → :8080

# Then open http://localhost:5173/admin
```

## Environment variables

See [.env.example](./.env.example). The required one is
`GOGRAB_DATABASE_URL`. Set `GOGRAB_DEV_USER` for local development without
Authentik. In production behind Traefik+Authentik, set
`GOGRAB_TRUSTED_PROXY=1` and leave `GOGRAB_DEV_USER` unset.

## Deployment

The shipping artifact is a single image (`gograb:latest`) plus Postgres.

```bash
# Build (ARM64 for Hetzner CAX, etc.)
make docker-build

# Run via docker-compose
cp .env.example .env  # fill in POSTGRES_PASSWORD, GOGRAB_NOTIFY_WEBHOOK_URL, ...
docker compose up -d

# Apply migrations against the running database
GOGRAB_DATABASE_URL="postgres://gograb:...@localhost:5432/gograb?sslmode=disable" \
  goose -dir migrations postgres "$GOGRAB_DATABASE_URL" up
```

The `docker-compose.yml` declares Traefik labels for two routers — one
catching `/admin*` and `/api/admin/*` with the `authentik@docker` middleware,
the other catching everything else without auth. Both terminate TLS via the
`cloudflare` certresolver (DNS-01 wildcard).

Adjust `gograb.example.com` and certresolver name to match your infra.

## API contract

See the [bootstrap brief](#) for the full spec. Briefly:

**Admin** (Authentik-protected):
- `POST   /api/admin/requests` → `{token, request_id}`
- `GET    /api/admin/requests`
- `GET    /api/admin/requests/{id}`
- `POST   /api/admin/requests/{id}/retrieve` (one-shot; purges on success)
- `DELETE /api/admin/requests/{id}`

**Public**:
- `GET  /api/requests/{token}/meta`
- `POST /api/requests/{token}/submit`

All JSON. Errors as `{error, message}`.

## Security notes

- The server NEVER sees plaintext, the AES key, or the URL fragment.
- Authentik forward-auth headers (`X-Authentik-Username`/`-Email`) are only
  honored when `GOGRAB_TRUSTED_PROXY=1`. The deployment must guarantee the
  app is reachable only through Traefik (Docker network isolation).
- Public endpoints are rate-limited per IP via `golang.org/x/time/rate`.
- Tokens are 16 random bytes → 22 base64url chars (128-bit entropy).
- A periodic sweeper (every 5 minutes) marks expired pending requests.

## TODO

- Authentik session cookie validation as defense-in-depth on the Go side.
- Per-request notification routing (Slack/Email/etc.) once needed.
- Audit log table for create/retrieve/delete events.
