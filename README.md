# GoGrab

[![CI](https://github.com/timhasenkamp/gograb/actions/workflows/ci.yml/badge.svg)](https://github.com/timhasenkamp/gograb/actions/workflows/ci.yml)
[![CodeQL](https://github.com/timhasenkamp/gograb/actions/workflows/codeql.yml/badge.svg)](https://github.com/timhasenkamp/gograb/actions/workflows/codeql.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Self-hosted, DSGVO-compliant **secret-request** service — the reverse of Yopass.

An operator creates a one-time *request link*, sends it to a customer, and the
customer enters a secret into a simple page. The secret is **encrypted in the
customer's browser** with a key the server never sees, retrievable exactly
once by the operator with a single WebAuthn tap (no link-juggling).

> ⚠️ **Alpha software, no external audit.** The crypto flow is documented and
> the dependencies are upstream-standard (`go-webauthn`, Web Crypto API), but
> nothing here has been independently reviewed. Use for low-to-moderate-stakes
> handoffs. Read [SECURITY.md](SECURITY.md) before deploying.

## How it works

```
   ┌───────────┐                                          ┌──────────────┐
   │ Operator  │  (1) generate AES-256 key in browser     │   Customer   │
   │ (browser) │  (2) wrap with master-KEK                │   (browser)  │
   └─────┬─────┘     post {wrapped_key, iv} to server     └──────┬───────┘
         │                                                       │
         │ (3) share URL: https://host/r/TOKEN#KEY ─────────────►│
         │                                                       │
         │                                       (4) encrypt secret with KEY
         │                                       (5) post {ciphertext, iv}
         │◄──────── server stores ciphertext ────────────────────┘
         │
   (6) WebAuthn-tap → master-KEK in memory
   (7) /retrieve → server returns {ciphertext, wrapped_key} then PURGES
   (8) browser unwraps key, decrypts, shows plaintext
```

The server holds wrapped material between submit and retrieve, nothing else.
A DB leak yields no plaintext. The operator unlocks the master-KEK once per
session via a hardware-bound WebAuthn-PRF tap; subsequent retrievals are
single-click.

## Stack

- Go 1.25 stdlib `net/http` with `http.ServeMux` pattern routing, `pgx/v5`,
  [`sqlc`](https://sqlc.dev/), [`goose`](https://github.com/pressly/goose)
- [`go-webauthn`](https://github.com/go-webauthn/webauthn) for the WebAuthn
  ceremonies; PRF extension for the operator unlock
- SvelteKit 2 with `adapter-static` + Svelte 5 Runes + TypeScript strict
- TailwindCSS for the admin SPA; vanilla scoped CSS for the public submit page
- Authentik forward-auth via Traefik for `/admin/*` and `/api/admin/*`
- Distroless static container, multi-arch (linux/amd64 + linux/arm64)

## Project layout

```
cmd/server/          single Go binary, embeds web/build via go:embed
internal/audit/      append-only audit-log dispatcher
internal/auth/       Authentik forward-auth middleware
internal/config/     env-driven config
internal/db/         sqlc-generated queries
internal/handlers/   HTTP handlers + security headers + 404 backoff
internal/notify/     webhook dispatcher
internal/token/      crypto/rand URL-safe token generator
internal/webauthn/   go-webauthn wrapper + Operator User adapter
migrations/          goose .sql migrations
web/                 SvelteKit project; web/web.go owns the //go:embed
Dockerfile           multi-arch build: node → go → distroless
docker-compose.yml   app + postgres + Traefik labels (Authentik forward-auth)
.github/workflows/   CI, multi-arch release, CodeQL
```

## Quickstart (local dev)

```bash
# 1. Postgres
docker run --name gograb-pg \
  -e POSTGRES_USER=gograb -e POSTGRES_PASSWORD=gograb -e POSTGRES_DB=gograb \
  -p 5433:5432 -d postgres:16-alpine

# 2. Migrations
go install github.com/pressly/goose/v3/cmd/goose@latest
export GOGRAB_DATABASE_URL="postgres://gograb:gograb@localhost:5433/gograb?sslmode=disable"
goose -dir migrations postgres "$GOGRAB_DATABASE_URL" up

# 3. Frontend build (populates web/build/ for go:embed)
make build-web

# 4. Run the server with a dev user (no Authentik required)
GOGRAB_DEV_USER=alice GOGRAB_TRUSTED_PROXY=0 make run

# In another shell, run vite for hot-reload of the admin UI:
cd web && npm run dev
# → open http://localhost:5173/admin and register your YubiKey at /admin/setup
```

You need a WebAuthn authenticator that supports the **PRF extension**:
- YubiKey 5+ with firmware ≥ 5.7
- Apple Passkeys (iOS 18 / macOS 15+)
- Windows Hello with TPM 2.0 (Win 11 23H2+)
- Bitwarden Browser Extension ≥ 2024.3.0

## Environment

See [`.env.example`](./.env.example). Required: `GOGRAB_DATABASE_URL`. Set
`GOGRAB_DEV_USER` to bypass Authentik for local development. In production,
also set:

- `GOGRAB_TRUSTED_PROXY=1` (behind Traefik+Authentik)
- `GOGRAB_RP_ID` = your public hostname (e.g. `gograb.example.com`)
- `GOGRAB_RP_ORIGINS` = the full HTTPS origin
- `GOGRAB_SESSION_SECRET` = 32 random bytes, base64url-encoded, stable across restarts
- A real `POSTGRES_PASSWORD` (rotate from the `CHANGEME` default)

## Deployment

The shipping artifact is `ghcr.io/<owner>/gograb` (multi-arch, distroless)
plus Postgres. Pushing a tag `v1.2.3` triggers
[`.github/workflows/release.yml`](.github/workflows/release.yml) which
builds + pushes linux/amd64 and linux/arm64.

```bash
# manually:
make docker-build              # local single-arch
docker buildx build --platform linux/amd64,linux/arm64 -t gograb:latest .

# docker-compose
cp .env.example .env           # fill in real POSTGRES_PASSWORD + WebAuthn vars
docker compose up -d

# apply migrations against the running DB
goose -dir migrations postgres "$GOGRAB_DATABASE_URL" up
```

The compose file declares Traefik labels for two routers:

- `gograb-admin` catches `/admin*` + `/api/admin/*` with the `authentik@docker`
  forward-auth middleware
- `gograb-public` catches everything else without auth

Both terminate TLS via the `cloudflare` certresolver (DNS-01 wildcard).
Adjust hostnames + certresolver to your infra.

## API

**Admin** (Authentik forward-auth):

| Method | Path                                       | Purpose |
|--------|--------------------------------------------|---------|
| POST   | `/api/admin/requests`                      | Create request, wrapped key in body |
| GET    | `/api/admin/requests`                      | List operator's requests |
| GET    | `/api/admin/requests/{id}`                 | Metadata |
| POST   | `/api/admin/requests/{id}/retrieve`        | Return ciphertext + wrapped_key, purge on success |
| DELETE | `/api/admin/requests/{id}`                 | Cancel |
| GET    | `/api/admin/auth/status`                   | Has credentials? PRF salt |
| POST   | `/api/admin/auth/{register,login}/{begin,finish}` | WebAuthn ceremonies |
| GET    | `/api/admin/auth/credentials`              | List operator's credentials |
| DELETE | `/api/admin/auth/credentials/{id}`         | Revoke (refuses last) |
| GET    | `/api/admin/audit`                         | Audit log |

**Public** (rate-limited + 404-backoff):

| Method | Path                                    | Purpose |
|--------|-----------------------------------------|---------|
| GET    | `/api/requests/{token}/meta`            | Description + status |
| POST   | `/api/requests/{token}/submit`          | Submit ciphertext + iv |

All JSON. Errors as `{error, message}`.

## Security model

See [SECURITY.md](SECURITY.md). Key points:

- Server never sees plaintext, master-KEK, PRF outputs, or per-request keys.
- DB stores wrapped material only.
- Operator unlock is hardware-bound via WebAuthn-PRF — phishing-resistant.
- Browser-served JS is the trust boundary: a compromised server can still
  exfiltrate plaintext by serving malicious JS. This is inherent to all
  browser-side E2E crypto.

## Contributing

Issues + PRs welcome. CI runs `go build/vet/test`, `npm run check`, and
`npm run build` on every push and PR. CodeQL runs weekly + on PRs.

Vulnerability reports: see [SECURITY.md](SECURITY.md) — please don't open
public issues for security problems.

## License

[MIT](LICENSE)
