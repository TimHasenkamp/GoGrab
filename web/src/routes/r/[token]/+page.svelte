<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { publicApi, type PublicMeta, type ApiError } from '$lib/api';
  import { importKeyB64url, encrypt } from '$lib/crypto';
  import { defaultSchema, type FormField } from '$lib/forms';
  import {
    generate,
    entropyBits,
    strengthLevel,
    strengthColor,
    type PwOptions
  } from '$lib/pwgen';
  import { i18n } from '$lib/i18n.svelte';
  import Icon from '$lib/Icon.svelte';
  import { theme } from '$lib/theme.svelte';

  i18n.init();
  const t = (k: any, p?: any) => i18n.t(k, p);

  const token = $derived($page.params.token ?? '');

  let meta = $state<PublicMeta | null>(null);
  let loading = $state(true);
  let loadError = $state<string | null>(null);
  let keyError = $state<string | null>(null);
  let key = $state<CryptoKey | null>(null);

  let submitting = $state(false);
  let submitError = $state<string | null>(null);
  let done = $state(false);
  let confirmOpen = $state(false);

  // Per-field state. Indexed by field.id.
  let values = $state<Record<string, string>>({});
  let pwReveal = $state<Record<string, boolean>>({});
  let pwSaved = $state<Record<string, boolean>>({});
  let pwOpts = $state<Record<string, PwOptions>>({});

  const schema = $derived<FormField[]>(meta?.form_schema ?? defaultSchema());
  const hasPasswordField = $derived(schema.some((f) => f.type === 'password'));
  const allPasswordsAcknowledged = $derived.by(() => {
    return schema
      .filter((f) => f.type === 'password' && values[f.id])
      .every((f) => pwSaved[f.id]);
  });
  const canSubmit = $derived.by(() => {
    if (submitting || !key) return false;
    for (const f of schema) {
      if (!values[f.id] || values[f.id]!.length === 0) return false;
    }
    return allPasswordsAcknowledged;
  });

  function initStateForSchema(s: FormField[]) {
    const v: Record<string, string> = {};
    const r: Record<string, boolean> = {};
    const sv: Record<string, boolean> = {};
    const o: Record<string, PwOptions> = {};
    for (const f of s) {
      v[f.id] = '';
      r[f.id] = false;
      sv[f.id] = false;
      o[f.id] = { length: 24, symbols: true };
    }
    values = v;
    pwReveal = r;
    pwSaved = sv;
    pwOpts = o;
  }

  function regenerate(id: string) {
    const opts = pwOpts[id] ?? { length: 24, symbols: true };
    values = { ...values, [id]: generate(opts) };
    pwSaved = { ...pwSaved, [id]: false };
    pwReveal = { ...pwReveal, [id]: false };
    void copyValue(id);
  }

  async function copyValue(id: string) {
    const v = values[id];
    if (!v) return;
    try {
      await navigator.clipboard.writeText(v);
    } catch {
      // ignored — browser may block outside user gesture
    }
  }

  function setLen(id: string, len: number) {
    pwOpts = { ...pwOpts, [id]: { ...pwOpts[id]!, length: len } };
    regenerate(id);
  }

  function toggleSymbols(id: string) {
    pwOpts = { ...pwOpts, [id]: { ...pwOpts[id]!, symbols: !pwOpts[id]!.symbols } };
    regenerate(id);
  }

  onMount(async () => {
    const hash = location.hash.startsWith('#') ? location.hash.slice(1) : '';
    if (!hash) {
      keyError = t('card.missing_key.text');
    } else {
      try {
        key = await importKeyB64url(hash);
      } catch {
        keyError = t('card.bad_key.text');
      }
    }
    try {
      meta = await publicApi.meta(token);
      initStateForSchema(meta.form_schema ?? defaultSchema());
    } catch (e) {
      loadError = (e as ApiError).message || t('card.unavailable.fallback');
    } finally {
      loading = false;
    }
  });

  // First click on submit just opens the confirmation panel — gives the
  // customer a clear "you're about to send to <brand>" moment before the
  // values leave their browser.
  function openConfirm(e: Event) {
    e.preventDefault();
    if (!canSubmit) return;
    submitError = null;
    confirmOpen = true;
  }

  async function reallySubmit() {
    if (!canSubmit) return;
    submitting = true;
    submitError = null;
    try {
      const plaintext = JSON.stringify(values);
      const { ciphertextB64, ivB64 } = await encrypt(plaintext, key!);
      await publicApi.submit(token, ciphertextB64, ivB64);
      done = true;
      confirmOpen = false;
    } catch (e) {
      submitError = (e as ApiError).message || t('error.submit_failed');
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:head>
  <title>{t('page.title', { brand: meta?.branding.name ?? 'GoGrab' })}</title>
  <meta name="robots" content="noindex" />
</svelte:head>

<main class="wrap" style:--brand-color={meta?.branding.color || '#0f172a'}>
  <header class="brand">
    {#if meta?.branding.logo_url}
      <img src={meta.branding.logo_url} alt="" class="brand-logo" />
    {/if}
    <h1>{meta?.branding.name ?? 'GoGrab'}</h1>
    <div class="header-actions">
      <button
        type="button"
        onclick={() => theme.toggle()}
        class="icon-pill"
        title={theme.current === 'dark' ? 'Heller Modus' : 'Dunkler Modus'}
        aria-label="Theme wechseln"
      >
        <Icon name={theme.current === 'dark' ? 'sun' : 'moon'} size={14} />
      </button>
      <button
        type="button"
        onclick={() => i18n.toggle()}
        class="lang-switch"
        title="Sprache wechseln / Switch language"
      >
        {t('lang.switch')}
      </button>
    </div>
  </header>

  {#if loading}
    <p class="muted">{t('loading')}</p>
  {:else if loadError}
    <div class="card error">
      <h2>{t('card.unavailable.title')}</h2>
      <p>{loadError}</p>
    </div>
  {:else if !meta}
    <p class="muted">{t('card.no_data')}</p>
  {:else if keyError}
    <div class="card error">
      <h2>{t('card.missing_key.title')}</h2>
      <p>{keyError}</p>
    </div>
  {:else if meta.status === 'expired' || new Date(meta.expires_at) < new Date()}
    <div class="card error">
      <h2>{t('card.expired.title')}</h2>
      <p>{t('card.expired.text')}</p>
    </div>
  {:else if meta.status !== 'pending'}
    <div class="card success">
      <h2>{t('card.already.title')}</h2>
      <p>{t('card.already.text')}</p>
    </div>
  {:else if done}
    <div class="card success">
      <h2>{t('card.done.title')}</h2>
      <p>{t('card.done.text')}</p>
      {#if hasPasswordField}
        <p class="hint">{t('card.done.pw_hint')}</p>
      {/if}
    </div>
  {:else}
    <div class="card">
      <p class="desc">{meta.description}</p>

      <form onsubmit={openConfirm}>
        {#each schema as f (f.id)}
          <div class="field">
            <label for="f-{f.id}">{f.label}</label>

            {#if f.type === 'textarea'}
              <textarea
                id="f-{f.id}"
                required
                rows="4"
                maxlength="32000"
                bind:value={values[f.id]}
                placeholder={f.placeholder || ''}
              ></textarea>
            {:else if f.type === 'text'}
              <input
                id="f-{f.id}"
                type="text"
                required
                maxlength="1000"
                bind:value={values[f.id]}
                placeholder={f.placeholder || ''}
              />
            {:else if f.type === 'password'}
              <div class="pw-row">
                <input
                  id="f-{f.id}"
                  type={pwReveal[f.id] ? 'text' : 'password'}
                  required
                  maxlength="1000"
                  bind:value={values[f.id]}
                  placeholder={f.placeholder || t('pw.placeholder')}
                  oninput={() => { pwSaved = { ...pwSaved, [f.id]: false }; }}
                />
                <button
                  type="button"
                  class="icon-btn"
                  onclick={() => (pwReveal = { ...pwReveal, [f.id]: !pwReveal[f.id] })}
                  title={pwReveal[f.id] ? t('pw.hide') : t('pw.show')}
                  aria-label={pwReveal[f.id] ? t('pw.hide') : t('pw.show')}
                >
                  <Icon name={pwReveal[f.id] ? 'eye-off' : 'eye'} size={16} />
                </button>
              </div>

              <div class="pw-actions">
                <button type="button" class="btn-secondary inline-icon-btn" onclick={() => regenerate(f.id)}>
                  <Icon name="rotate-cw" size={14} />
                  <span>{t('pw.generate')}</span>
                </button>
                <button
                  type="button"
                  class="btn-secondary inline-icon-btn"
                  disabled={!values[f.id]}
                  onclick={() => copyValue(f.id)}
                >
                  <Icon name="copy" size={14} />
                  <span>{t('pw.copy')}</span>
                </button>
              </div>

              {#if values[f.id]}
                {@const bits = entropyBits(pwOpts[f.id] ?? { length: 24, symbols: true })}
                <div class="strength">
                  <div class="strength-bar">
                    <div class="strength-fill" style:width={Math.min(100, (bits / 160) * 100) + '%'} style:background={strengthColor(bits)}></div>
                  </div>
                  <span class="strength-label" style:color={strengthColor(bits)}>
                    {t('pw.strength.' + strengthLevel(bits) as any)} · {t('pw.bits', { n: bits })}
                  </span>
                </div>

                <details class="opts">
                  <summary>{t('pw.adjust')}</summary>
                  <div class="opts-row">
                    <span class="opts-label">{t('pw.length')}</span>
                    <div class="chips">
                      {#each [16, 24, 32, 48] as n (n)}
                        <button
                          type="button"
                          class="chip"
                          class:active={pwOpts[f.id]?.length === n}
                          onclick={() => setLen(f.id, n)}
                        >
                          {n}
                        </button>
                      {/each}
                    </div>
                  </div>
                  <div class="opts-row">
                    <span class="opts-label">{t('pw.symbols')}</span>
                    <button
                      type="button"
                      class="chip"
                      class:active={pwOpts[f.id]?.symbols}
                      onclick={() => toggleSymbols(f.id)}
                    >
                      {pwOpts[f.id]?.symbols ? t('pw.on') : t('pw.off')}
                    </button>
                  </div>
                </details>

                <label class="confirm">
                  <input type="checkbox" bind:checked={pwSaved[f.id]} />
                  <span>{t('pw.confirm_saved', { label: f.label })}</span>
                </label>
              {/if}
            {/if}
          </div>
        {/each}

        {#if submitError}<p class="error-text">{submitError}</p>{/if}

        {#if hasPasswordField}
          <div class="save-warn">{t('warn.save_passwords')}</div>
        {/if}

        <button type="submit" class="btn-submit" disabled={!canSubmit}>
          {submitting ? t('submit.button.busy') : t('submit.button.idle')}
        </button>
      </form>

      <p class="note">{t('submit.note')}</p>
    </div>
  {/if}

  {#if confirmOpen && meta}
    <div class="modal-backdrop" role="dialog" aria-modal="true">
      <div class="modal">
        <h2 class="modal-title">{t('confirm.title')}</h2>
        <p class="modal-lead">
          {@html t('confirm.lead', { brand: `<strong>${meta.branding.name}</strong>` })}
        </p>
        <ul class="modal-fields">
          {#each schema as f (f.id)}
            <li>
              <span class="field-label">{f.label}</span>
              {#if f.type === 'password'}
                <span class="field-value">••••••••</span>
              {:else if (values[f.id] ?? '').length > 60}
                <span class="field-value">{(values[f.id] ?? '').slice(0, 57)}…</span>
              {:else}
                <span class="field-value">{values[f.id] ?? ''}</span>
              {/if}
            </li>
          {/each}
        </ul>
        <p class="modal-note">{t('confirm.note', { brand: meta.branding.name })}</p>
        {#if submitError}<p class="error-text">{submitError}</p>{/if}
        <div class="modal-actions">
          <button type="button" class="btn-secondary" onclick={() => (confirmOpen = false)} disabled={submitting}>
            {t('confirm.back')}
          </button>
          <button type="button" class="btn-submit modal-confirm" onclick={reallySubmit} disabled={submitting}>
            {submitting ? t('confirm.sending') : t('confirm.send', { brand: meta.branding.name })}
          </button>
        </div>
      </div>
    </div>
  {/if}

  <footer>
    {#if meta?.branding.name && meta.branding.name !== 'GoGrab'}
      {@html t('footer.branded', { brand: meta.branding.name }).replace('GoGrab', '<strong>GoGrab</strong>')}
    {:else}
      {t('footer.unbranded')}
    {/if}
  </footer>
</main>

<style>
  /* Customer surface adopts the same palette as the admin (mirrored from
     hasenkamp.dev): dark base by default, light theme reachable via the
     <html data-theme="light"> attribute set by the init script. The
     per-operator --brand-color cascades on top of these and overrides the
     accent for primary CTAs / heading. */
  :global(:root),
  :global(:root[data-theme='dark']) {
    --c-bg: #0a0a0f;
    --c-fg: #e4e4e7;
    --c-muted: #71717a;
    --c-card: #111118;
    --c-border: #1e1e2e;
    --c-border-strong: #2a2a3d;
    --c-accent: #00e5ff;
    --c-accent-hover: #00b8d4;
    --c-accent-glow: rgba(0, 229, 255, 0.18);
    --c-success: #05df72;
    --c-warning: #fac800;
    --c-danger: #ff6568;
    --c-backdrop: rgba(10, 10, 15, 0.75);
  }
  :global(:root[data-theme='light']) {
    --c-bg: #ffffff;
    --c-fg: #18181b;
    --c-muted: #71717a;
    --c-card: #fafafa;
    --c-border: #e4e4e7;
    --c-border-strong: #d4d4d8;
    --c-accent: #0891b2;
    --c-accent-hover: #0e7490;
    --c-accent-glow: rgba(8, 145, 178, 0.2);
    --c-success: #16a34a;
    --c-warning: #b45309;
    --c-danger: #dc2626;
    --c-backdrop: rgba(24, 24, 27, 0.45);
  }

  :global(body) {
    margin: 0;
    background: var(--c-bg);
    color: var(--c-fg);
    font-family: 'Geist', ui-sans-serif, system-ui, -apple-system, sans-serif;
    line-height: 1.5;
    min-height: 100dvh;
  }
  .wrap {
    max-width: 32rem;
    margin: 0 auto;
    padding: 2rem 1rem 4rem;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    margin: 0 0 1.5rem;
  }
  .brand-logo {
    height: 1.75rem;
    width: auto;
    max-width: 6rem;
    object-fit: contain;
  }
  .header-actions {
    margin-left: auto;
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
  }
  .lang-switch,
  .icon-pill {
    background: transparent;
    border: 1px solid var(--c-border-strong);
    color: var(--c-muted);
    border-radius: 999px;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .lang-switch {
    padding: 0.15rem 0.6rem;
    font-size: 0.7rem;
    font-weight: 600;
    letter-spacing: 0.05em;
  }
  .icon-pill {
    padding: 0.25rem;
    width: 1.6rem;
    height: 1.6rem;
  }
  .lang-switch:hover,
  .icon-pill:hover {
    border-color: var(--c-accent);
    color: var(--c-accent);
  }
  h1 {
    font-size: 1.25rem;
    font-weight: 600;
    margin: 0;
    letter-spacing: -0.025em;
    color: var(--brand-color, var(--c-accent));
  }
  .muted { color: var(--c-muted); }
  .card {
    background: var(--c-card);
    border: 1px solid var(--c-border);
    border-radius: 12px;
    padding: 1.25rem;
  }
  .card.error { border-color: rgba(255, 101, 104, 0.3); background: rgba(255, 101, 104, 0.06); }
  .card.success { border-color: rgba(5, 223, 114, 0.3); background: rgba(5, 223, 114, 0.06); }
  .card h2 { font-size: 1rem; margin: 0 0 0.5rem; letter-spacing: -0.01em; }
  .card.error h2 { color: var(--c-danger); }
  .card.success h2 { color: var(--c-success); }
  .desc {
    font-weight: 500;
    margin: 0 0 1rem;
    color: var(--c-fg);
  }

  .field {
    margin-bottom: 1.1rem;
  }
  .field + .field {
    border-top: 1px dashed var(--c-border);
    padding-top: 1.1rem;
  }
  label {
    display: block;
    font-size: 0.875rem;
    font-weight: 500;
    margin-bottom: 0.35rem;
    color: var(--c-fg);
  }
  input[type='text'],
  input[type='password'],
  textarea {
    width: 100%;
    box-sizing: border-box;
    padding: 0.55rem 0.6rem;
    border: 1px solid var(--c-border-strong);
    border-radius: 6px;
    font-family: 'Geist Mono', ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.875rem;
    background: var(--c-bg);
    color: var(--c-fg);
  }
  input::placeholder, textarea::placeholder { color: var(--c-muted); }
  textarea { resize: vertical; }
  input:focus, textarea:focus {
    outline: none;
    border-color: var(--c-accent);
    box-shadow: 0 0 0 1px var(--c-accent);
  }

  .pw-row {
    display: flex;
    gap: 0.4rem;
    align-items: stretch;
  }
  .pw-row input { flex: 1; }
  .icon-btn {
    background: var(--c-card);
    border: 1px solid var(--c-border-strong);
    color: var(--c-muted);
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0 0.6rem;
    border-radius: 6px;
  }
  .icon-btn:hover { color: var(--c-accent); border-color: var(--c-accent); }

  .pw-actions {
    display: flex;
    gap: 0.4rem;
    margin-top: 0.45rem;
  }
  .btn-secondary {
    background: transparent;
    border: 1px solid var(--c-border-strong);
    color: var(--c-fg);
    padding: 0.35rem 0.65rem;
    border-radius: 6px;
    font-size: 0.8125rem;
    cursor: pointer;
  }
  .inline-icon-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
  }
  .btn-secondary:hover:not(:disabled) { border-color: var(--c-accent); color: var(--c-accent); }
  .btn-secondary:disabled { opacity: 0.5; cursor: not-allowed; }

  .strength {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    margin-top: 0.55rem;
    font-size: 0.75rem;
  }
  .strength-bar {
    flex: 1;
    height: 5px;
    background: var(--c-border);
    border-radius: 3px;
    overflow: hidden;
  }
  .strength-fill { height: 100%; transition: width 0.2s, background 0.2s; }
  .strength-label { font-weight: 500; min-width: 8rem; text-align: right; font-variant-numeric: tabular-nums; }

  .opts {
    margin-top: 0.45rem;
    font-size: 0.8125rem;
  }
  .opts > summary {
    cursor: pointer;
    color: var(--c-muted);
    user-select: none;
    padding: 0.25rem 0;
  }
  .opts > summary:hover { color: var(--c-fg); }
  .opts-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.4rem;
    flex-wrap: wrap;
  }
  .opts-label {
    width: 6rem;
    color: var(--c-muted);
    font-weight: 500;
  }
  .chips { display: flex; gap: 0.25rem; }
  .chip {
    background: transparent;
    border: 1px solid var(--c-border-strong);
    color: var(--c-muted);
    padding: 0.2rem 0.55rem;
    border-radius: 999px;
    font-size: 0.75rem;
    cursor: pointer;
  }
  .chip:hover { border-color: var(--c-accent); color: var(--c-fg); }
  .chip.active {
    background: var(--c-accent);
    border-color: var(--c-accent);
    color: var(--c-bg);
  }

  .save-warn {
    margin-top: 1rem;
    background: rgba(250, 200, 0, 0.08);
    border: 1px solid rgba(250, 200, 0, 0.3);
    color: var(--c-warning);
    border-radius: 6px;
    padding: 0.55rem 0.7rem;
    font-size: 0.8125rem;
    line-height: 1.4;
  }
  .confirm {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    margin-top: 0.55rem;
    font-size: 0.8125rem;
    color: var(--c-fg);
    font-weight: 400;
    cursor: pointer;
  }
  .confirm input {
    width: 0.95rem;
    height: 0.95rem;
    accent-color: var(--c-accent);
  }

  .btn-submit {
    margin-top: 1rem;
    background: var(--brand-color, var(--c-accent));
    color: var(--c-bg);
    border: none;
    padding: 0.7rem 1rem;
    border-radius: 6px;
    font-weight: 600;
    cursor: pointer;
    font-size: 0.9375rem;
    width: 100%;
    transition: box-shadow 0.15s, background 0.15s;
  }
  .btn-submit:hover:not(:disabled) {
    background: var(--c-accent-hover);
    box-shadow: 0 0 0 1px var(--c-accent-glow), 0 8px 28px -8px var(--c-accent-glow);
  }
  .btn-submit:disabled { opacity: 0.4; cursor: not-allowed; }

  .note {
    margin-top: 1rem;
    font-size: 0.75rem;
    color: var(--c-muted);
  }
  .hint {
    margin-top: 0.75rem;
    font-size: 0.8125rem;
    color: var(--c-success);
  }
  .error-text {
    margin-top: 0.5rem;
    color: var(--c-danger);
    font-size: 0.875rem;
  }
  footer {
    margin-top: 2rem;
    font-size: 0.75rem;
    color: var(--c-muted);
    text-align: center;
  }
  /* :global() because @html-injected children aren't visible to scoped CSS */
  footer :global(strong) { color: var(--c-fg); }

  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(10, 10, 15, 0.75);
    backdrop-filter: blur(4px);
    display: grid;
    place-items: center;
    padding: 1rem;
    z-index: 50;
  }
  .modal {
    background: var(--c-card);
    border: 1px solid var(--c-border);
    border-radius: 12px;
    max-width: 26rem;
    width: 100%;
    padding: 1.25rem;
    box-shadow: 0 20px 50px rgba(0, 0, 0, 0.5);
    max-height: 90vh;
    overflow-y: auto;
  }
  .modal-title {
    font-size: 1rem;
    font-weight: 600;
    margin: 0 0 0.5rem;
    letter-spacing: -0.01em;
    color: var(--c-fg);
  }
  .modal-lead {
    font-size: 0.875rem;
    color: var(--c-muted);
    margin: 0 0 0.75rem;
  }
  .modal-lead :global(strong) { color: var(--c-fg); }
  .modal-fields {
    list-style: none;
    padding: 0;
    margin: 0 0 0.75rem;
    border: 1px solid var(--c-border);
    border-radius: 8px;
    overflow: hidden;
  }
  .modal-fields li {
    display: grid;
    grid-template-columns: minmax(7rem, auto) 1fr;
    gap: 0.75rem;
    padding: 0.5rem 0.75rem;
    font-size: 0.8125rem;
    background: var(--c-bg);
  }
  .modal-fields li + li {
    border-top: 1px solid var(--c-border);
  }
  .field-label {
    color: var(--c-muted);
    font-weight: 500;
  }
  .field-value {
    color: var(--c-fg);
    font-family: 'Geist Mono', ui-monospace, SFMono-Regular, Menlo, monospace;
    word-break: break-all;
  }
  .modal-note {
    font-size: 0.75rem;
    color: var(--c-muted);
    margin: 0 0 1rem;
  }
  .modal-actions {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .modal-confirm {
    margin-top: 0;
  }
  @media (min-width: 28rem) {
    .modal-actions {
      flex-direction: row-reverse;
      justify-content: flex-start;
    }
    .modal-confirm {
      flex: 1;
    }
  }
</style>
