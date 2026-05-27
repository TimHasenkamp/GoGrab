# Security Policy

## Status

GoGrab is **alpha** software maintained as a hobby project. It implements a
zero-knowledge secret-handoff flow with browser-side WebAuthn-PRF unlock —
non-trivial cryptography that has not been independently audited. Treat it
accordingly: useful for low-to-moderate-stakes handoffs (WLAN passwords,
license keys, dev credentials), **not** for high-stakes secrets without your
own review of the threat model.

## Threat model in one paragraph

The server holds ciphertext + envelope-wrapped per-request keys only. The
operator's master KEK lives in JS memory after a WebAuthn-PRF tap and never
touches the server. A DB leak therefore does not yield plaintext. The model
does *not* defend against an attacker who can deliver modified JavaScript to
the operator's browser (compromised host, supply-chain attack on a
dependency, etc.) — that is an inherent limitation of all browser-side E2E
crypto.

For the full version with assets, adversaries, trust seams and operational
posture: [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md).
Operator playbook for common situations: [docs/RUNBOOK.md](docs/RUNBOOK.md).

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security reports.

Email security reports to **security@hkp-solutions.de**  with:

- A clear description of the issue
- Steps to reproduce or proof-of-concept
- The version / commit SHA you tested against
- Whether you'd like public credit and under what name

Expect an initial response within 7 days. Fixes are typically prepared on a
private branch and released with a coordinated CVE if applicable.

## Scope

In scope:
- The Go server and its HTTP API
- The SvelteKit admin frontend
- The Dockerfile and docker-compose deployment
- The crypto flow as documented in the README

Out of scope:
- Issues that require physical access to the operator's unlocked workstation
- Brute-force of operator credentials managed by Authentik (report those to
  Authentik upstream)
- Denial-of-service via legitimate traffic patterns above the configured
  rate limits

## Hardening checklist for operators

If you run GoGrab, verify:

1. The host is reachable **only** through Traefik (Docker network isolation).
   The app trusts `X-Authentik-Username` when `GOGRAB_TRUSTED_PROXY=1` —
   exposing it directly bypasses Authentik.
2. `GOGRAB_SESSION_SECRET` is set to a stable 32-byte secret in production,
   not left empty.
3. Postgres credentials (`POSTGRES_PASSWORD`) are rotated from defaults.
4. TLS terminates at Traefik (Cloudflare DNS-01 or LetsEncrypt) — never run
   GoGrab over plain HTTP in prod, WebAuthn refuses to work without it.
5. Database backups are encrypted at rest. The DB contains wrapped key
   material, not plaintext, but defense-in-depth still applies.
6. Every operator has **at least two** WebAuthn credentials registered
   (Primary + Backup), with the backup stored physically separate.
