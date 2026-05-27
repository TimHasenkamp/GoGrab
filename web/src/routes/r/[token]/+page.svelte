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
    <button
      type="button"
      onclick={() => i18n.toggle()}
      class="lang-switch"
      title="Sprache wechseln / Switch language"
    >
      {t('lang.switch')}
    </button>
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
                  {pwReveal[f.id] ? '🙈' : '👁'}
                </button>
              </div>

              <div class="pw-actions">
                <button type="button" class="btn-secondary" onclick={() => regenerate(f.id)}>
                  {t('pw.generate')}
                </button>
                <button
                  type="button"
                  class="btn-secondary"
                  disabled={!values[f.id]}
                  onclick={() => copyValue(f.id)}
                >
                  {t('pw.copy')}
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
  :global(body) {
    margin: 0;
    background: #f5f6f8;
    color: #1e293b;
    font-family: system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif;
    line-height: 1.5;
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
  .lang-switch {
    margin-left: auto;
    background: transparent;
    border: 1px solid #cbd5e1;
    color: #475569;
    border-radius: 999px;
    padding: 0.15rem 0.6rem;
    font-size: 0.7rem;
    font-weight: 600;
    cursor: pointer;
    letter-spacing: 0.05em;
  }
  .lang-switch:hover {
    background: #f1f5f9;
    color: #0f172a;
  }
  h1 {
    font-size: 1.25rem;
    font-weight: 600;
    margin: 0;
    color: var(--brand-color, #0f172a);
  }
  .muted { color: #64748b; }
  .card {
    background: #fff;
    border: 1px solid #e2e8f0;
    border-radius: 8px;
    padding: 1.25rem;
  }
  .card.error { border-color: #fecaca; background: #fef2f2; }
  .card.success { border-color: #bbf7d0; background: #f0fdf4; }
  .card h2 { font-size: 1rem; margin: 0 0 0.5rem; }
  .desc {
    font-weight: 500;
    margin: 0 0 1rem;
    color: #0f172a;
  }

  .field {
    margin-bottom: 1.1rem;
  }
  .field + .field {
    border-top: 1px dashed #e2e8f0;
    padding-top: 1.1rem;
  }
  label {
    display: block;
    font-size: 0.875rem;
    font-weight: 500;
    margin-bottom: 0.35rem;
    color: #334155;
  }
  input[type='text'],
  input[type='password'],
  textarea {
    width: 100%;
    box-sizing: border-box;
    padding: 0.55rem 0.6rem;
    border: 1px solid #cbd5e1;
    border-radius: 4px;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.875rem;
  }
  textarea { resize: vertical; }
  input:focus, textarea:focus {
    outline: 2px solid #0f172a;
    outline-offset: -1px;
    border-color: #0f172a;
  }

  .pw-row {
    display: flex;
    gap: 0.4rem;
    align-items: stretch;
  }
  .pw-row input { flex: 1; }
  .icon-btn {
    background: #f1f5f9;
    border: 1px solid #cbd5e1;
    color: #334155;
    cursor: pointer;
    font-size: 0.9rem;
    padding: 0 0.55rem;
    border-radius: 4px;
  }
  .icon-btn:hover { background: #e2e8f0; }

  .pw-actions {
    display: flex;
    gap: 0.4rem;
    margin-top: 0.45rem;
  }
  .btn-secondary {
    background: #fff;
    border: 1px solid #cbd5e1;
    color: #334155;
    padding: 0.35rem 0.65rem;
    border-radius: 4px;
    font-size: 0.8125rem;
    cursor: pointer;
  }
  .btn-secondary:hover:not(:disabled) { background: #f8fafc; }
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
    background: #e2e8f0;
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
    color: #475569;
    user-select: none;
    padding: 0.25rem 0;
  }
  .opts > summary:hover { color: #0f172a; }
  .opts-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.4rem;
    flex-wrap: wrap;
  }
  .opts-label {
    width: 6rem;
    color: #475569;
    font-weight: 500;
  }
  .chips { display: flex; gap: 0.25rem; }
  .chip {
    background: #fff;
    border: 1px solid #cbd5e1;
    color: #475569;
    padding: 0.2rem 0.55rem;
    border-radius: 999px;
    font-size: 0.75rem;
    cursor: pointer;
  }
  .chip:hover { border-color: #94a3b8; }
  .chip.active {
    background: #0f172a;
    border-color: #0f172a;
    color: #fff;
  }

  .save-warn {
    margin-top: 1rem;
    background: #fffbeb;
    border: 1px solid #fde68a;
    color: #78350f;
    border-radius: 4px;
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
    color: #334155;
    font-weight: 400;
    cursor: pointer;
  }
  .confirm input {
    width: 0.95rem;
    height: 0.95rem;
    accent-color: #0f172a;
  }

  .btn-submit {
    margin-top: 1rem;
    background: var(--brand-color, #0f172a);
    color: #fff;
    border: none;
    padding: 0.65rem 1rem;
    border-radius: 4px;
    font-weight: 500;
    cursor: pointer;
    font-size: 0.9375rem;
    width: 100%;
  }
  .btn-submit:hover:not(:disabled) { background: #1e293b; }
  .btn-submit:disabled { opacity: 0.5; cursor: not-allowed; }

  .note {
    margin-top: 1rem;
    font-size: 0.75rem;
    color: #64748b;
  }
  .hint {
    margin-top: 0.75rem;
    font-size: 0.8125rem;
    color: #166534;
  }
  .error-text {
    margin-top: 0.5rem;
    color: #b91c1c;
    font-size: 0.875rem;
  }
  footer {
    margin-top: 2rem;
    font-size: 0.75rem;
    color: #94a3b8;
    text-align: center;
  }

  /* confirmation modal */
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(15, 23, 42, 0.55);
    display: grid;
    place-items: center;
    padding: 1rem;
    z-index: 50;
  }
  .modal {
    background: #fff;
    border-radius: 10px;
    max-width: 26rem;
    width: 100%;
    padding: 1.25rem;
    box-shadow: 0 20px 50px rgba(0, 0, 0, 0.25);
    max-height: 90vh;
    overflow-y: auto;
  }
  .modal-title {
    font-size: 1rem;
    font-weight: 600;
    margin: 0 0 0.5rem;
    color: #0f172a;
  }
  .modal-lead {
    font-size: 0.875rem;
    color: #334155;
    margin: 0 0 0.75rem;
  }
  .modal-fields {
    list-style: none;
    padding: 0;
    margin: 0 0 0.75rem;
    border: 1px solid #e2e8f0;
    border-radius: 6px;
    overflow: hidden;
  }
  .modal-fields li {
    display: grid;
    grid-template-columns: minmax(7rem, auto) 1fr;
    gap: 0.75rem;
    padding: 0.5rem 0.75rem;
    font-size: 0.8125rem;
  }
  .modal-fields li + li {
    border-top: 1px solid #e2e8f0;
  }
  .field-label {
    color: #64748b;
    font-weight: 500;
  }
  .field-value {
    color: #0f172a;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    word-break: break-all;
  }
  .modal-note {
    font-size: 0.75rem;
    color: #64748b;
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
