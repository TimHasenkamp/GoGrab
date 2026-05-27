# GoGrab — SaaS Roadmap

Konkreter Plan vom aktuellen Stand („self-hosted single-tenant Tool, läuft
für einen Operator hinter Authentik") zu „launchable SaaS, das mehrere
zahlende Kunden auf einer geteilten Instanz isoliert bedient".

Reihenfolge ist die empfohlene Sequenz — frühere Phasen blockieren spätere.

---

## Phase 0 — Foundations (vor allem anderen)

Ohne diese Schicht geht **gar nichts**. Datenleck zwischen Tenants ist die
einzige Sünde, die du dir bei einem Security-Tool nicht erlauben kannst.

### Multi-Tenancy

- [ ] **Schema-Refactor**: `organizations`-Tabelle mit `id`, `slug` (URL-safe,
      unique), `name`, `plan_id`, `suspended_at`, `created_at`, `updated_at`.
- [ ] **`org_id`-FK auf jeder relevanten Tabelle**: `operators`, `requests`,
      `webauthn_credentials`, `audit_log`. Migration mit Default auf eine
      Initial-Org für bestehende Daten.
- [ ] **Postgres Row-Level-Security aktivieren** auf allen Org-scoped
      Tabellen. Policy: `org_id = current_setting('app.org_id')::uuid`. App
      setzt das Setting nach Auth, vor jeder Query. Strukturell unmöglich,
      dass eine Query versehentlich Cross-Tenant-Daten zieht.
- [ ] **Membership-Tabelle**: `operator_id ↔ org_id` mit `role`
      (owner/admin/member). Ein Operator kann in mehreren Orgs sein.
- [ ] **Org-Switcher in der Topbar** wenn der Operator > 1 Org hat.
- [ ] **Slug-Validierung**: Reserved-Words-Liste (`admin`, `api`, `app`,
      `www`, `support`, etc.), 3-32 Zeichen, `[a-z0-9-]`.

### Tenant-Routing

- [ ] **Subdomain-Routing**: `{slug}.app.gograb.io` → App liest den Slug aus
      dem Host-Header, lädt die Org, setzt sie als Request-Kontext.
- [ ] **Wildcard-TLS** via Traefik mit DNS-01-Challenge (Cloudflare etc.).
- [ ] **Cookie-Scope**: Auth-Session-Cookies pro Subdomain — Cross-Org-Leak
      auch auf der Cookie-Ebene verhindern.
- [ ] **404 / Redirect für unbekannte Slugs** — kein Info-Leak welcher Slug
      existiert vs. nicht.

### Self-Service Auth

Zwei Pfade, beide funktionieren — entscheide dich für einen:

#### Option A: Authentik (eigene Infra, kein Vendor-Lockin)

- [ ] **Authentik OIDC-Mode** statt Forward-Auth: GoGrab redirected auf
      Authentik's `/authorize` und verarbeitet den Callback.
- [ ] **Signup-Flow in Authentik**: Email + Passwort, Verifizierungs-Mail
      (Authentik-Templates customizen), Anti-Spam (hCaptcha / Cloudflare Turnstile).
- [ ] **Programmatic User → Org Bridge**: nach Signup ruft GoGrab Authentik's
      API auf, legt User in die richtige Authentik-Group (= Org), anschließend
      neuer Auth-Token. Webhook von Authentik → unsere App wäre cleaner.
- [ ] **Password-Reset** über Authentik (built-in, braucht SMTP-Setup).
- [ ] **2FA-Option** über Authentik (TOTP/WebAuthn — letzteres nutzen wir eh schon
      für KEK-Unlock, hier ginge's um Login-MFA als zusätzliche Schicht).
- [ ] **Branding pro Org**: Authentik kann Flow-Branding pro Tenant nicht
      out-of-the-box → entweder Pull-Request zu Authentik, oder wir
      machen einen Sub-Flow pro Org und vergeben dem User dynamisch das
      richtige Branding — Aufwand.

#### Option B: Clerk / WorkOS (SaaS-Identity-Provider)

- [ ] **Clerk SDK** ([clerk-svelte](https://clerk.com/docs/quickstarts/sveltekit))
      oder [WorkOS AuthKit](https://workos.com/docs/user-management) integrieren.
- [ ] **Organizations-Feature**: Clerk/WorkOS hat ein natives Org-Konzept,
      bridgen mit unserer `organizations`-Tabelle (Webhook `organization.created`).
- [ ] **JWT-Verifizierung** im Go-Backend statt Header-Trust.
- [ ] **Email + Passkey-Login** (Clerk macht beides).
- [ ] **Branding-API**: Clerk kann Login-UI pro Org einfärben — out-of-the-box.

Empfehlung: **Wenn du Authentik schon mit Liebe pflegst → A. Wenn du
Time-to-Market priorisierst → B.** B ist 3-5 Tage, A eher 1-2 Wochen.

### Transactional Email

- [ ] **Provider entscheiden**: [Resend](https://resend.com),
      [Postmark](https://postmarkapp.com), [Mailgun](https://mailgun.com).
      Resend hat den nettesten Developer-Flow, ~$0/Monat bei < 3k Mails.
- [ ] **`internal/mail`-Package** mit Templates für: Signup-Verifizierung,
      Password-Reset, Submission-Notification, Invoice-Receipt,
      Suspension-Warning.
- [ ] **HTML + Plaintext** beide Versionen jeder Mail.
- [ ] **Per-Org-Branding** im Template (Name, Logo, Akzentfarbe).
- [ ] **Bounce-Webhook**: Hard-Bounces den User markieren.
- [ ] **DKIM/SPF/DMARC** auf der Versand-Domain (kosmetisch wichtig).

---

## Phase 1 — Launch-ready

Sobald Phase 0 steht, hast du eine geteilte Plattform. Phase 1 macht sie
verkaufbar.

### Pläne & Quotas

- [ ] **Plan-Definitionen** als Code (`internal/plans/plans.go`): Free / Pro /
      Team / Enterprise mit:
      - Max Operators pro Org
      - Max Requests pro Monat
      - Max Pending Requests gleichzeitig
      - Audit-Retention (Free: 30d, Pro: 180d, etc.)
      - Custom Branding (ab Pro)
      - Custom Domain (ab Team)
- [ ] **Quota-Enforcement** in den Handlers: vor jedem `CreateRequest`
      Plan-Check.
- [ ] **Usage-Counter** in `usage_monthly`-Tabelle, pro Org pro Monat
      inkrementell.
- [ ] **Soft-Limits** (Warnung bei 80%, Block bei 100%) statt harter Cliffs.

### Billing

- [ ] **Stripe-Account** + Products + Prices in Stripe Dashboard anlegen
      (matched die Pläne aus oben).
- [ ] **Stripe Checkout** für initialen Plan-Kauf nach Signup.
- [ ] **Stripe Customer Portal** für Plan-Wechsel + Karten-Update + Cancel.
- [ ] **Webhook-Endpoint** `/api/stripe/webhook` für
      `customer.subscription.{created,updated,deleted}`,
      `invoice.payment_failed`, etc.
- [ ] **Trial-Period**: 14 Tage, keine Karte nötig. Nach Trial: Karte
      eingeben oder Lese-only / Lösch-Countdown.
- [ ] **Dunning**: 3 Erinnerungen bei `payment_failed` über 14 Tage, dann
      Suspension.

Optional für Start: **manuelle Rechnungen** für die ersten 5-10 Kunden.
Stripe einbauen wenn der Schmerz größer wird als das Bauen.

### Onboarding

- [ ] **Post-Signup-Wizard**: Org-Name setzen → erstes Branding → erstes
      WebAuthn-Setup → Test-Request anlegen → Pricing/Plan auswählen.
- [ ] **Empty-States** auf den Hauptseiten mit „so machst du den ersten
      Request"-CTA.
- [ ] **Sample-Request** vorgenerieren, damit die UI nicht leer wirkt.
- [ ] **Welcome-Email** nach 1 Tag, Tipps nach 3 Tagen, Nudge bei
      Inaktivität nach 7 Tagen.

### Branding pro Org

- [ ] **Settings-Page** `/admin/settings/branding`: Name, Logo-Upload (oder
      URL), Akzentfarbe, Custom-Footer-Text.
- [ ] **`branding`-Spalte** auf `organizations` (JSONB).
- [ ] **Logo-Storage**: S3-kompatibel (Cloudflare R2, Backblaze B2, Hetzner
      Object Storage) — kein Inline-Upload in DB.
- [ ] **Customer-Page liest Org-Branding** statt der globalen `GOGRAB_BRAND_*`-Env-Vars.

### Legal & DSGVO

- [ ] **Impressum** (für DE-Markt Pflicht).
- [ ] **Datenschutzerklärung**: was, wo, wie lange, wer Zugriff hat. Liste
      aller Subprozessoren (Stripe, Resend, Hetzner, Sentry, …).
- [ ] **AVV/DPA-Template** als PDF + per-Klick-akzeptierbar im Onboarding für
      B2B-Kunden.
- [ ] **AGB / Terms of Service**.
- [ ] **Cookie-Banner**: nur wenn nicht-essentielle Cookies. Tipp: Plausible
      Analytics statt Google → kein Banner nötig.
- [ ] **Recht auf Datenmitnahme**: „Export all my org data"-Button → ZIP mit
      DB-Dump (org-scoped) + Audit-Log.
- [ ] **Recht auf Löschung**: „Delete org" Self-Service mit 30d Soft-Delete,
      dann hard-delete.
- [ ] **Anwaltliche Review** der Templates: rechne mit ~€800-2000 einmalig.

### Marketing & Discovery

- [ ] **Marketing-Site**: Landing-Page mit Hero, Feature-Sections, Pricing,
      Vergleich vs. Yopass / SecureSafe / 1Password Share, FAQ, Sign-up-CTA.
      Kann SvelteKit static + ein paar Sections sein.
- [ ] **Docs-Site**: Quickstart, API-Doku (kommt in Phase 2), FAQ, Security-FAQ,
      DPA-Download. Kandidaten: SvelteKit-Static, mdBook, Astro Starlight.
- [ ] **`/blog`** mit 2-3 initialen Posts (Launch-Announcement,
      Architecture-Deep-Dive, Compare-to-Alternatives).
- [ ] **OG-Images** + Twitter-Cards für Social-Sharing.
- [ ] **SEO-Basics**: Sitemap, robots.txt, structured data für Pricing-Page.

### Observability

- [ ] **Error-Tracking**: [Sentry](https://sentry.io) (oder GlitchTip self-hosted)
      für Frontend (Svelte) + Backend (Go). Source-Maps fürs Frontend.
- [ ] **Uptime-Monitoring**: extern via [Better Uptime](https://betterstack.com),
      [Uptimerobot](https://uptimerobot.com), Healthcheck.io, … 60s-Probe auf
      `/healthz` + `/api/admin/auth/status`.
- [ ] **Status-Page** öffentlich: Better Uptime oder Instatus, integriert
      mit Monitoring.
- [ ] **Application-Metrics**: Prometheus-`/metrics`-Endpoint im Go-Server,
      Grafana-Dashboard mit P99-Latency, request/min, error-rate.
- [ ] **Strukturierte Logs** (haben wir schon — slog JSON). Aggregator wie
      Better Stack Logs, Grafana Loki, oder Axiom.

---

## Phase 2 — SaaS Table-Stakes

Hier wird's „professional grade".

### API für Programmatic-Access

- [ ] **API-Tokens** pro Org (`api_tokens`-Tabelle: token-hash, scopes,
      created_at, last_used_at).
- [ ] **`/api/v1/`-Namespace** mit Token-Auth statt Cookie.
- [ ] **OpenAPI/Swagger-Spec** generieren + öffentlich hosten.
- [ ] **Rate-Limits per Token** (zusätzlich zu per-IP).
- [ ] **Endpoints**: list/create/get/delete requests, list audit, get usage.

### Audit-Log-Export

- [ ] **CSV / JSON / NDJSON-Export** pro Org, optional Zeitbereich.
- [ ] **Compliance-Mode**: Audit-Log unveränderlich (kein DELETE, kein UPDATE)
      auf DB-Ebene erzwingen.

### Outbound-Webhooks

- [ ] **Webhook-Config-UI**: URL + Secret + Event-Types wählen pro Org.
- [ ] **Retry-Logik** mit exponential backoff, max 24h, danach Disable.
- [ ] **Signed Payloads** (HMAC-SHA256 mit Per-Webhook-Secret) damit Empfänger
      Authentizität prüfen können.
- [ ] **Webhook-Logs** in der UI: letzte 100 Deliveries mit Status.

### Custom Domains (Team-Plan+)

- [ ] **Settings**: Custom-Domain eintragen → DNS-Verify per TXT-Record.
- [ ] **Auto-Provisioning** TLS-Cert via Traefik DNS-01.
- [ ] **Routing-Anpassung**: App muss den eingehenden Host zusätzlich gegen
      `org.custom_domain` matchen.

### Support-Tools

- [ ] **In-App-Help-Center**: Knowledge-Base mit Suchfeld. Kann Statik-Site
      sein die in iframe oder als Pop-up läuft.
- [ ] **Support-Inbox**: Email-Adresse + Ticket-System. Anfangs reicht
      `support@gograb.io` → deine normale Inbox. Bei Wachstum: Plain, Help
      Scout, Frontapp.
- [ ] **Operator-Impersonation** für Support: du kannst dich als Customer
      einloggen (mit großer Banner-Warnung + Audit-Eintrag) um Probleme zu
      reproduzieren. **Heikel — sauberes Logging + DSGVO-Klausel zwingend**.

### Backup & Recovery

- [ ] **Automatische DB-Dumps** täglich (Hetzner-eigene Backup oder pg_dump
      → S3-kompatibel).
- [ ] **Encrypted at Rest** mit age oder gpg.
- [ ] **Restore-Drill alle 6 Monate** mit Dokumentation.
- [ ] **Point-in-Time-Recovery** mit WAL-Archivierung (Postgres-native) ab
      einer gewissen Größe.

---

## Phase 3 — Wachstum

Wenn die ersten 10-50 Kunden glücklich sind und Enterprise-Pitches kommen:

### SSO & Enterprise-Auth

- [ ] **SAML 2.0** Support (über Authentik / WorkOS).
- [ ] **SCIM 2.0** Provisioning für IdPs wie Okta, Azure AD.
- [ ] **IP-Allowlist** pro Org für besonders sensitive Kunden.

### Compliance

- [ ] **SOC 2 Type 1** Audit (~$10-30k einmalig, Drata/Vanta beschleunigen
      das Sammeln der Evidence).
- [ ] **ISO 27001** wenn DACH-Enterprise-Kunden ankommen (~€20-50k).
- [ ] **Penetration-Test** durch externe Firma vor SOC 2.
- [ ] **TOM (Technische und organisatorische Maßnahmen)** dokumentieren.
- [ ] **Data Processing Impact Assessment (DPIA)** für DSGVO.

### Skalierung

- [ ] **Postgres-HA** (Patroni-Cluster, oder Managed wie Neon, Supabase).
- [ ] **Stateless App-Tier hinter Load-Balancer**: schon möglich, Session-Secret
      env-var stabil halten.
- [ ] **Region-Splitting**: EU-only-Storage für DACH-Markt (manche Enterprise-
      Kunden verlangen das). Erfordert Multi-Region-Architektur.
- [ ] **CDN für statische Assets**: Cloudflare oder bunny.net vor der App.

### Whitelabel & Enterprise-Features

- [ ] **Whitelabel-Plan**: kein „Powered by GoGrab"-Footer, eigene Domain,
      eigenes Email-From.
- [ ] **Reseller-API**: B2B-Partner können Sub-Tenants in ihrem Account
      anlegen.
- [ ] **Dedicated Instance**: Pro-Enterprise-Kunde isolierte Postgres + App-
      Container (höchste Preis-Stufe).

---

## Geschätzte Effort & Kosten

| Phase | Zeit (Solo, Vollzeit) | Laufende Kosten (klein) |
|---|---|---|
| 0 — Foundations | 3-5 Wochen | ~$50-150/mo (DB, App-Host, TLS) |
| 1 — Launch-Ready | 3-5 Wochen | +$50-100/mo (Email, Error-Tracking, Status-Page) |
| 2 — Table-Stakes | 4-8 Wochen | +$100-300/mo (Logs, Object-Storage, Support-Tools) |
| 3 — Wachstum | nach Bedarf | abhängig von Kundenmenge + Compliance |

**Realistischer Pfad**: 2-4 Monate Vollzeit-Solo bis Launch-bereit, plus
geringe Side-Project-Wartung danach. Bei 1-2 Tagen/Woche → 6-12 Monate.

---

## Wann **nicht** SaaS bauen

Bevor du loslegst, ehrlich überlegen:

- **Hast du 5+ Leute, die dafür heute schon zahlen würden?** Wenn nicht →
  zuerst Sales / Discovery, nicht Code.
- **Willst du Operations machen?** SaaS = du bist 24/7 verantwortlich für
  Uptime + Support. Kein „ich nehm mir mal ne Woche frei".
- **Alternative: OSS-Distribution + Support-Verträge.** Lässt dich
  konzentriert programmieren, weniger Ops-Last. Beispiele: Plausible,
  Wireguard, Sentry-Self-Hosted.

Wenn die Antworten alle „ja, weiß ich, will ich" sind → Phase 0 ist dein
nächster Schritt.
