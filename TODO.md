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
- [x] **#7 Audit-Log-Retention.** `gograb prune-audit [days]`-Subcommand
  (default 180d) löscht alte Einträge. Operator-Cron / systemd-Timer
  exekutiert das tägliches. SQL ist execrows — geloggt wird die
  gelöschte Zeilenanzahl.
- [x] **#8 Customer-Page-Views-Signal.** Public-Meta-Endpoint feuert
  jetzt einen `request.view`-Audit-Eintrag. `AdminGet` zählt diese und
  liefert `view_count`. Detail-Seite zeigt „N× vom Kunden geöffnet —
  aber noch keine Einreichung" im Pending-Zustand. Audit-UI hat ein
  eigenes Label (hellblau) für `request.view`.

## 🟢 Polish (irgendwann)

- [x] **#9 Customer-Side-Vorschau vor Submit.** Erster Submit-Klick öffnet
  eine Modal mit der Feldliste (Werte gemasked bei Passwort, gekürzt bei
  langem Text), erklärt nochmal an wen das Ganze geht und hat den eigentlichen
  „An $BRAND senden"-Button.
- [x] **#10 i18n.** Customer-Seite jetzt `de`/`en`. Auto-Detection via
  `navigator.language`, Override per `?lang=de|en`. DE/EN-Pill oben rechts
  als Sprachschalter. Strength-Label nutzt jetzt eine `StrengthLevel`-Enum
  statt fest „schwach/ok/stark/sehr stark" — i18n-fähig.
- [x] **#11 Mailto-Quick-Action.** „Per Mail senden"-Button auf der
  Erfolgsseite und im Resend-Block — öffnet `mailto:` mit vorbefülltem
  Subject + Body inkl. Share-URL.
- [x] **#12 Docs.** Drei neue Dokumente:
  - [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md): Assets / Adversaries /
    Trust-Seams / was geht / was nicht / operational posture
  - [docs/RUNBOOK.md](docs/RUNBOOK.md): YubiKey-Verlust, Authentik-Down,
    Release-Deploy, Server-Migration, Audit-Pruning, …
  - [migrations/README.md](migrations/README.md): how to add a new
    migration, sqlc-resync hinweis, why embedded

## Deferred (manuell migrieren, dependabot ignoriert)

- [ ] **tailwindcss 3 → 4.** Drop `@tailwind base/components/utilities` →
  `@import "tailwindcss";`. Install `@tailwindcss/postcss`, postcss.config.js
  umstellen. tailwind.config.js entweder portieren oder durch `@theme`-Blöcke
  in CSS ersetzen. v3-Utilities gegenchecken (manche entfernt/umbenannt).
- [ ] **vite 6 → 8 + @sveltejs/vite-plugin-svelte 5 → 7.** Müssen
  zusammen mit kompatiblen Versionen hochgezogen werden (peer-dep-Koppelung).
  Sveltekit-Range checken bevor's losgeht.

## SaaS-Pfad

Für den Schritt von „self-hosted Single-Tenant" zu „launchable SaaS für
mehrere zahlende Orgs": [docs/SAAS_ROADMAP.md](docs/SAAS_ROADMAP.md).
4 Phasen, ~2-4 Monate Solo-Vollzeit bis Launch.

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
