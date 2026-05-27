# GoGrab — Roadmap

Ehrliche Liste konkreter Lücken, sortiert nach Impact. Erste zwei Tiers
sind die, die ich vor v1.0-tag erledigt hätte.

## 🔴 Blocker für ernsthaften Einsatz

- [ ] **#1 Tests.** Außer `token` + `auth` ist die Coverage minimal. Mindestens:
  - `internal/handlers`: Request-Lifecycle (create → submit → retrieve → purge),
    Form-Schema-Validation, Auth-Fail-Paths
  - `internal/webauthn`: Session-Token Pack/Unpack-Roundtrip mit gemockten Secrets
  - `internal/audit`: nicht-blockierender Insert-Pfad
  - Mindestens ein E2E-Smoke-Test des Submit→Retrieve-Roundtrips
- [ ] **#2 Migrate-on-Boot.** `gograb migrate` Subcommand + optional
  `GOGRAB_MIGRATE_ON_BOOT=1` Env-Flag, das beim Start die Migrationen anwendet.
  Sonst wird das Update 3 Releases später vergessen.
- [ ] **#3 Trusted-Proxy-CIDR-Check.** `X-Authentik-Username`-Header nur
  akzeptieren, wenn `RemoteAddr` in einem konfigurierten Netz liegt
  (`GOGRAB_TRUSTED_PROXY_CIDR`). Verhindert Auth-Bypass falls der Port
  versehentlich direkt exposed wird.

## 🟡 Vor v1.0 dazupacken

- [ ] **#4 Resend-Link.** Im Pending-Zustand auf der Detail-Seite einen Button
  „Share-URL nochmal anzeigen". Aktuell muss man cancelen + neu anlegen wenn
  der Kunde den Link verbummelt hat. (Setzt voraus dass der Operator beim
  Anlegen die Share-URL temporär in der Session behält — keine Persistenz.)
- [ ] **#5 Branding der `/r/[token]`-Seite.** Operator-konfigurierbarer
  Anbieter-Name + optional Logo per Env oder pro-Operator-Setting. Aktuell
  sieht der Kunde „GoGrab", was er nicht kennt.
- [ ] **#6 Pagination + Suche** auf der Requests-Liste. Aktuell hardcoded
  `LIMIT 200`. Nach 6 Monaten regelmäßiger Nutzung wird's eng. Plus
  freitext-Suche im `description`-Feld.
- [ ] **#7 Audit-Log-Retention.** `gograb prune-audit --older-than=180d`
  Subcommand. Audit-Tabelle wächst sonst unbegrenzt. Idealerweise
  Retention-Window per Env, der täglich gepruned wird.
- [ ] **#8 Customer-Page-Views-Signal.** Track Meta-Aufrufe → Operator sieht
  „Link wurde 2× angeschaut, keine Submission". Hilft beim Nachhaken („Hat
  der Kunde den Link überhaupt bekommen?").

## 🟢 Polish (irgendwann)

- [ ] **#9 Customer-Side-Vorschau vor Submit.** „Du sendest diese Werte an
  _Operator-Name_." Trust-Building, eine kleine Confirm-Modal.
- [ ] **#10 i18n.** Aktuell deutsch-only auf der Customer-Seite. Sprachschalter
  (`de` / `en`) oder Auto-Detection via `Accept-Language`.
- [ ] **#11 Mailto-Quick-Action.** Auf dem Share-URL-Screen ein Button „Per
  Mail senden" der `mailto:?subject=...&body=...` öffnet. Spart Copy-Paste.
- [ ] **#12 Docs.** SECURITY.md hat einen Threat-Model-Absatz, kein
  Operator-Runbook („was tun wenn YubiKey weg / Authentik kaputt /
  DB-Migration in Prod"). Plus README pro Migrations-Datei mit Up/Down.

## Erledigt aus vorherigen Sessions

- [x] Zero-knowledge Crypto-Flow (URL-Fragment → envelope-wrap mit Master-KEK)
- [x] WebAuthn-PRF Unlock + Backup-Key + Lockout-Schutz
- [x] Audit-Log Tabelle + UI
- [x] CSP, HSTS, Permissions-Policy, 404-Backoff
- [x] Webhook-Notify auf Submit + Retrieve
- [x] Operator-definiertes Form-Schema (text/password/textarea)
- [x] Password-Generator auf Customer-Seite mit Strength + Auto-Copy
- [x] Multi-Arch Docker-Image + GHCR Release-Workflow
- [x] CI: go build/test/vet + Svelte-Check, CodeQL, Dependabot
- [x] OSS-Metadata: LICENSE (MIT), SECURITY.md, .gitattributes
- [x] `.env` Auto-Loader für Local-Dev
