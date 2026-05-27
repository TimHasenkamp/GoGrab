# GoGrab — Roadmap

Ehrliche Liste konkreter Lücken, sortiert nach Impact. Erste zwei Tiers
sind die, die ich vor v1.0-tag erledigt hätte.

## 🔴 Blocker für ernsthaften Einsatz

- [x] **#1 Tests.** Coverage erweitert um:
  - `internal/handlers`: Form-Schema-Validation, Submit→Retrieve-Roundtrip,
    Foreign-Operator-404, Bad-Input-Pfade, 404-Backoff State-Machine + Middleware
  - `internal/webauthn`: Session-Token Pack/Unpack-Roundtrip, Tamper-Erkennung,
    Expiry, Wrong-Secret
  - `internal/audit`: synchroner Insert-Pfad mit DB-Fehler-Swallow, XFF,
    UA-Truncation
- [x] **#2 Migrate-on-Boot.** Migrations sind via `//go:embed` ins Binary
  gepackt. `gograb migrate {up,down,status,version,redo,reset}` als
  Subcommand, plus `GOGRAB_MIGRATE_ON_BOOT=1` für Auto-Apply beim Start.
- [x] **#3 Trusted-Proxy-CIDR-Check.** `GOGRAB_TRUSTED_PROXY_CIDRS=10.0.0.0/8,…`
  schränkt jetzt ein, von welchen Source-IPs aus `X-Authentik-*` überhaupt
  honoriert wird. Leer = Legacy-Verhalten + Warn-Log beim Start. Tests decken
  innerhalb/außerhalb CIDR und IPv4-mapped-IPv6 ab.

## 🟡 Vor v1.0 dazupacken

- [x] **#4 Resend-Link.** `session.recentShareUrls` hält die Share-URL für den
  aktuellen Tab vor. Detail-Seite zeigt im Pending-Zustand einen
  „Share-URL nochmal anzeigen"-Button, mit Kopieren + Mailto. Beim
  Tab-Schließen weg (kein localStorage = kein Leak).
- [x] **#5 Branding der `/r/[token]`-Seite.** `GOGRAB_BRAND_NAME`,
  `GOGRAB_BRAND_LOGO_URL`, `GOGRAB_BRAND_COLOR` werden über `PublicMeta`
  durchgereicht und auf der Customer-Seite gerendert (h1 / Logo /
  Accent-Color für Buttons). Footer zeigt „Sicher übermittelt mit GoGrab
  für $BRAND_NAME" beim Custom-Branding.
- [x] **#6 Pagination + Suche.** `ListRequestsByOperator` mit `@search`
  (ILIKE), `@lim`/`@off`. `AdminList` liest `?q=&limit=&offset=`, neue
  `listResponse`-Hülle mit `items/total/limit/offset`. Frontend hat ein
  debounced Suchfeld und Vor/Zurück-Pagination ab > 50 Treffern.
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
- [x] **#11 Mailto-Quick-Action.** „Per Mail senden"-Button auf der
  Erfolgsseite und im Resend-Block — öffnet `mailto:` mit vorbefülltem
  Subject + Body inkl. Share-URL.
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
