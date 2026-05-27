# Deploy

How the CI → image-push → SSH-deploy pipeline works, and what to set up
on the prod host before the first run.

## TL;DR

```bash
git commit -m "feat: something nice [deploy]"
git push
```

GitHub Actions runs CI. If green AND `[deploy]` is in the commit message,
the deploy job builds a multi-arch image, pushes it to GHCR with two tags
(`sha-XXXXXXX` and `main`), then SSHes to prod and runs `docker compose
pull && docker compose up -d`. Migrations apply automatically on boot
(`GOGRAB_MIGRATE_ON_BOOT=1`).

`workflow_dispatch` is also exposed: trigger from the GitHub Actions UI
when you want to redeploy without a new commit. Optional `sha:` input lets
you pin a specific revision.

## What runs when

| Trigger | CI | Image build | Deploy |
|---|---|---|---|
| Push to any branch / PR | ✓ | – | – |
| Push to main *without* `[deploy]` | ✓ | – | – |
| Push to main *with* `[deploy]` in commit msg | ✓ | ✓ | ✓ |
| GitHub Actions UI → "Run workflow" on Deploy | – | ✓ | ✓ |

Red CI = no deploy. Always. (`workflow_run` only fires the deploy job when
the CI run completed with `conclusion == 'success'`.)

## First-time prod setup

These steps run **once** when you bring up a fresh server. After that the
CI pipeline handles all updates.

### 1. Server prerequisites

- Hetzner CAX (or any Linux VPS with Docker + docker compose plugin).
- DNS A/AAAA-record for `gograb.example.com` → server IP.
- Traefik (or an equivalent reverse proxy) listening on the `proxy`
  external Docker network with TLS terminated. Authentik forward-auth
  middleware registered as `authentik@docker`.
- A user with `docker` group membership (or use `root`).

### 2. Lay out the app directory

```bash
ssh tim@prod
sudo mkdir -p /opt/gograb
sudo chown -R $USER:$USER /opt/gograb
cd /opt/gograb

# Pull the compose file from the repo (or scp it from your laptop)
curl -L https://raw.githubusercontent.com/TimHasenkamp/GoGrab/main/docker-compose.yml \
  -o docker-compose.yml

# Create .env with real values
cat > .env <<'EOF'
POSTGRES_USER=gograb
POSTGRES_PASSWORD=<generate-with-openssl-rand>
POSTGRES_DB=gograb

GOGRAB_PUBLIC_BASE_URL=https://gograb.example.com
GOGRAB_RP_ID=gograb.example.com
GOGRAB_RP_ORIGINS=https://gograb.example.com
GOGRAB_SESSION_SECRET=<head -c 32 /dev/urandom | basenc --base64url | tr -d '=' >
GOGRAB_TRUSTED_PROXY_CIDRS=172.16.0.0/12,127.0.0.0/8

GOGRAB_BRAND_NAME=GoGrab
GOGRAB_BRAND_LOGO_URL=
GOGRAB_BRAND_COLOR=

GOGRAB_NOTIFY_WEBHOOK_URL=
EOF
chmod 600 .env
```

Required env vars (compose will refuse to start without them):
`POSTGRES_PASSWORD`, `GOGRAB_RP_ID`, `GOGRAB_RP_ORIGINS`,
`GOGRAB_SESSION_SECRET`.

### 3. GHCR pull access

If your repo is **public** (default), no auth needed.

If **private**, log Docker into GHCR on prod with a PAT that has
`read:packages`:

```bash
echo "$GHCR_PAT" | docker login ghcr.io -u TimHasenkamp --password-stdin
```

### 4. Traefik network

```bash
docker network ls | grep proxy || docker network create proxy
```

### 5. First pull + up

```bash
cd /opt/gograb
docker compose pull
docker compose up -d
docker compose logs -f app   # watch for "listening" + "webauthn ready"
```

Open `https://gograb.example.com/admin` → Authentik gates → `/admin/setup`
→ register your first YubiKey.

## GitHub secrets to set

Once for the repo (Settings → Secrets and variables → Actions → Repository
secrets):

| Secret | Value |
|---|---|
| `PROD_SSH_HOST` | IP or hostname of the prod box, e.g. `203.0.113.42` |
| `PROD_SSH_USER` | The user the deploy job SSHes as. Must own `/opt/gograb` and be in the `docker` group. |
| `PROD_SSH_KEY` | Private key (full PEM, multi-line). Paste verbatim. |
| `PROD_SSH_PORT` | *optional*, defaults to 22 |

Generate a dedicated deploy key:

```bash
ssh-keygen -t ed25519 -f ./deploy_key -N '' -C 'github-actions-deploy'
# Copy the .pub side to the prod user's ~/.ssh/authorized_keys
ssh-copy-id -i ./deploy_key.pub tim@prod
# Add the private side (./deploy_key) to PROD_SSH_KEY
```

Lock the key down on prod (optional, recommended):

```ssh
# In ~/.ssh/authorized_keys on prod, prefix the key with:
command="cd /opt/gograb && docker compose pull && docker compose up -d --remove-orphans && docker image prune -f",no-pty,no-agent-forwarding,no-port-forwarding ssh-ed25519 AAAA... github-actions-deploy
```

That restricts the key to *only* run the deploy command — full SSH access
isn't granted, just `pull → up → prune`. Trade-off: you'd lose flexibility
in the workflow's script block.

## How to deploy

### Routine deploy (most of the time)

Write `[deploy]` somewhere in the commit message:

```bash
git commit -m "feat(forms): support hidden fields [deploy]"
git push
```

CI runs (2-3 min) → deploy job builds + pushes (3-5 min on first run, ~1
min with cache) → SSH-up on prod (10-30 sec).

### Redeploy without code changes

GitHub Actions UI → Deploy workflow → "Run workflow" → pick the SHA or
leave blank for latest.

### Pin an older revision

Same as above, set the `sha:` input to the desired commit SHA.

Or on the prod box directly:

```bash
cd /opt/gograb
IMAGE_TAG=sha-abc1234 docker compose up -d
```

The compose file defaults to `:main` but accepts an `IMAGE_TAG` override.

## How to roll back

The current Image stays in GHCR as `sha-XXXXXXX` for at least 90 days
(GHCR's default retention). To roll back:

```bash
ssh tim@prod
cd /opt/gograb
IMAGE_TAG=sha-<previous-good-sha> docker compose up -d
```

Or via GitHub UI → Deploy → "Run workflow" → enter the older SHA.

## What gets deployed

The CI workflow builds `web/build/` for the embed step. The Docker build
in `deploy.yml` re-runs that build inside the multi-stage Dockerfile, so
there's no "stale frontend" trap: the image is always self-consistent with
the SHA being deployed.

## Migrations

`GOGRAB_MIGRATE_ON_BOOT=1` is set in the compose file. On every `up`, the
app opens the DB, applies any pending `migrations/*.sql` files via goose,
then starts the HTTP listener. Idempotent — applying twice is a no-op.

If a migration needs special attention (rare), run it manually first:

```bash
ssh tim@prod
cd /opt/gograb
docker compose exec app /app/gograb migrate status
docker compose exec app /app/gograb migrate up
```

## What does NOT get deployed

- The `.env` file on prod. Compose loads from `/opt/gograb/.env` at runtime.
  Update env vars by editing the file manually + `docker compose up -d` to
  apply.
- The Postgres data volume. Mounted at `gograb-pg`, survives container
  recreates. Take backups separately (e.g. `pg_dump` via cron).
- The Traefik / Authentik config. Those live in their own
  docker-compose.yml elsewhere on the host.

## Backups

Not automated yet. Recommended cron on prod:

```bash
# /etc/cron.daily/gograb-backup
#!/bin/sh
set -e
cd /opt/gograb
docker compose exec -T postgres pg_dump -U gograb gograb \
  | gzip > /var/backups/gograb-$(date +%F).sql.gz
find /var/backups -name 'gograb-*.sql.gz' -mtime +30 -delete
```

Restore (very-not-tested-on-prod): `gunzip -c <file>.sql.gz | docker compose exec -T postgres psql -U gograb -d gograb`.

## Monitoring

`/healthz` returns `200 OK` for a Traefik healthcheck or external uptime
monitor. Hook it up via Better Uptime / Uptimerobot / etc.

`docker compose logs app` is structured JSON. Pipe to your log
aggregator (Better Stack Logs, Grafana Loki, Axiom) when traffic
warrants.
