# GoGrab — Threat Model

This document spells out what GoGrab protects against, what it doesn't, and
where the seams are. Pair with [SECURITY.md](../SECURITY.md) for the
short version and reporting flow.

## Assets

| # | Asset | Where it lives | Sensitivity |
|---|---|---|---|
| A1 | Customer's plaintext secret | Customer browser memory; encrypted at rest server-side; operator browser after retrieve | **High** |
| A2 | Per-request AES key | Customer URL fragment; operator browser after wrap; server holds the *envelope-wrapped* form | **High** |
| A3 | Operator's master KEK | Operator browser memory only (non-extractable via wrapKey path) | **High** |
| A4 | WebAuthn PRF output | Operator browser memory only | **High** (master-KEK precursor) |
| A5 | WebAuthn credential public material | DB + browser | Low (public-by-design) |
| A6 | Audit log | DB | Medium |
| A7 | Operator identity via Authentik | Authentik instance + headers | Medium |

## Adversaries

| # | Adversary | Capabilities |
|---|---|---|
| T1 | Curious sysadmin / hoster | Read DB + disk + memory snapshot of the running server |
| T2 | DB backup leak | Snapshot of postgres dump |
| T3 | Network MITM between operator/customer and server | Inject/observe TLS-terminated requests |
| T4 | Compromised Authentik | Mint headers / cookies as any user |
| T5 | Active server compromise | Replace served JS bundle / Go binary |
| T6 | Token brute-force | Anonymous internet client guessing URL tokens |
| T7 | Operator endpoint compromise | Malware on the operator's workstation while session is unlocked |
| T8 | Phishing of operator | Fake login page tries to steal credentials |
| T9 | Customer endpoint compromise | Customer's workstation already pwned before they fill the form |

## What GoGrab defends against

### T1 / T2 — DB or disk leak

The server stores only:
- `ciphertext` + `iv` (AES-GCM output, unbreakable without the key)
- `wrapped_key` + `wrap_iv` (per-request key wrapped with the master KEK)
- `wrapped_master` per credential (master KEK wrapped with the PRF-derived
  key for that authenticator)

Without the operator's WebAuthn authenticator AND a fresh PRF eval, none of
this decrypts. A leaked backup yields **zero plaintext**.

### T3 — Network MITM

All traffic terminates TLS at Traefik. WebAuthn refuses to run over plain
HTTP, so the unlock flow is bound to the operator's actual origin (origin
pinning is part of the spec).

### T4 — Compromised Authentik

If Authentik is compromised, the attacker can forge `X-Authentik-Username`
headers. They reach the admin API. **BUT** they cannot unlock the operator's
master KEK without the operator's hardware authenticator — the WebAuthn
ceremony is gated by the physical YubiKey or platform TPM. They can:
- Create new requests (the new wrapped_key would be wrapped under a master
  KEK they don't have — useless to them, useless to the real operator too)
- List and read metadata of existing requests
- Delete requests
- Register a new WebAuthn credential under their account — but THAT
  credential's wrapped_master is encrypted with PRF from their *own*
  authenticator, not the real operator's, so it can't unwrap existing
  per-request keys

So: Authentik compromise lets an attacker disrupt the service for a given
user but **not decrypt existing or future secrets**. The audit log records
all of their actions.

### T6 — Token brute-force

Tokens are 16 random bytes (128 bits, base64url). At 10 guesses/sec from
the IP rate limiter, brute-force takes ~10^31 years. Defense in depth via
`internal/handlers/notfound_backoff.go`: an IP hitting 10 unknown tokens in
60 seconds gets a 5-minute backoff.

### T7 (partial) — Operator endpoint compromise while LOCKED

If the operator's browser tab is *closed* or the session is *locked*
(no master KEK in memory), the local machine being pwned does not yield
plaintext for old or future requests. Attacker would need to also steal
the YubiKey or trigger an unlock (which requires physical tap on the YubiKey
in most configurations).

### T8 — Phishing

WebAuthn credentials are origin-bound by the FIDO2 spec. A phishing page at
`gograb.evil.com` cannot trigger the authenticator to sign anything that's
valid for `gograb.your-domain.com`. The browser refuses.

## What GoGrab does NOT defend against

### T5 — Active server compromise

If the attacker can replace the JS bundle delivered to operator/customer
browsers, the game is over. The malicious JS can:
- Capture plaintext before encryption (customer side)
- Capture decrypted plaintext after retrieval (operator side)
- Exfiltrate the PRF output / master KEK
- Mint a backup credential under the operator's account during unlock

This is **inherent to all browser-side end-to-end crypto**. ProtonMail's
web client, the Signal web client, Yopass, etc. all share this limit.
The only mitigations involve a native app or browser extension with
pinned code — out of scope for GoGrab.

Operational reduction: keep the host minimal (distroless image, no shell),
sign + verify Docker images, restrict access to the deployment pipeline,
enable Dependabot + CodeQL.

### T7 (full) — Operator endpoint compromise while UNLOCKED

If malware runs in the operator's browser context while the master KEK is
in memory, it can call subtle.encrypt/decrypt with it, exfiltrate the raw
key bytes (since the key is `extractable: true` for the wrapKey path),
and decrypt anything wrapped under it past and present.

Reduction: only unlock when you need to retrieve. Lock when done (Lock
button in the top bar). Don't unlock on shared workstations.

### T9 — Customer endpoint compromise

The customer sees their own plaintext anyway (they typed it). Nothing
GoGrab can do.

### Operator losing all WebAuthn credentials

If the operator loses BOTH primary and backup authenticators, all wrapped
masters become permanently undecryptable, taking every wrapped per-request
key (and therefore every ciphertext) with them. The audit log and metadata
survive but the secrets are unrecoverable.

Mitigation: enforced backup credential registration is recommended in the
UI but not technically enforced. Operators MUST register at least two
credentials.

## Trust seams

| Seam | What flows | Trust level |
|---|---|---|
| Operator's WebAuthn authenticator → Browser | PRF output (32 bytes) | Cryptographically hard to break |
| Browser → Server | Wrapped material only (never PRF, never plaintext) | TLS-protected |
| Server → DB | All wrapped + ciphertext | Internal Docker network |
| Authentik → App | `X-Authentik-Username`, optional CIDR-gated | App trusts the header within `GOGRAB_TRUSTED_PROXY_CIDRS` |
| Operator MUA → Customer | Share URL with key in URL fragment | Out of band; operator's responsibility |

## Recommended operational posture

1. **Two WebAuthn credentials per operator**, primary on-person, backup in a
   safe.
2. **`GOGRAB_TRUSTED_PROXY_CIDRS` set** to your Docker bridge network.
3. **TLS via Traefik with HSTS preloaded** at the certresolver level.
4. **`gograb prune-audit`** scheduled via cron / systemd timer to bound the
   audit log size.
5. **Postgres backups encrypted at rest** (LUKS / age / restic).
6. **Pin the Docker image to a tag**, not `:latest`, in compose. Rotate
   intentionally on release.
7. **Authentik 2FA enabled** so the upstream identity layer isn't a single
   password.
