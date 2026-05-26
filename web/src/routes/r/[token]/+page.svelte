<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { publicApi, type PublicMeta, type ApiError } from '$lib/api';
  import { importKeyB64url, encrypt } from '$lib/crypto';
  import {
    generate,
    entropyBits,
    strengthLabel,
    strengthColor,
    type PwOptions
  } from '$lib/pwgen';

  const token = $derived($page.params.token ?? '');

  let meta = $state<PublicMeta | null>(null);
  let loading = $state(true);
  let loadError = $state<string | null>(null);
  let keyError = $state<string | null>(null);

  let mode = $state<'manual' | 'generate'>('manual');
  let secret = $state('');
  let submitting = $state(false);
  let submitError = $state<string | null>(null);
  let done = $state(false);

  let key = $state<CryptoKey | null>(null);

  // --- generator state ---
  let pwOpts = $state<PwOptions>({ length: 24, symbols: true });
  let pwReveal = $state(false);
  let pwCopied = $state(false);
  let pwSavedConfirmed = $state(false);

  const entropy = $derived(entropyBits(pwOpts));
  const strength = $derived(strengthLabel(entropy));
  const strengthHex = $derived(strengthColor(entropy));

  function regenerate() {
    secret = generate(pwOpts);
    pwCopied = false;
    pwSavedConfirmed = false;
    // Auto-copy: convenience — best-effort, may fail in some browsers.
    void copySecret();
  }

  async function copySecret() {
    if (!secret) return;
    try {
      await navigator.clipboard.writeText(secret);
      pwCopied = true;
      setTimeout(() => (pwCopied = false), 2000);
    } catch {
      pwCopied = false;
    }
  }

  function switchMode(next: 'manual' | 'generate') {
    if (mode === next) return;
    mode = next;
    secret = '';
    pwCopied = false;
    pwSavedConfirmed = false;
    if (next === 'generate') regenerate();
  }

  onMount(async () => {
    const hash = location.hash.startsWith('#') ? location.hash.slice(1) : '';
    if (!hash) {
      keyError =
        'Dieser Link enthält keinen Verschlüsselungs-Schlüssel. Bitte den Absender, den Link erneut zu schicken.';
    } else {
      try {
        key = await importKeyB64url(hash);
      } catch {
        keyError = 'Der Schlüssel in diesem Link ist ungültig.';
      }
    }
    try {
      meta = await publicApi.meta(token);
    } catch (e) {
      loadError = (e as ApiError).message || 'Anfrage nicht gefunden';
    } finally {
      loading = false;
    }
  });

  async function submit(e: Event) {
    e.preventDefault();
    if (submitting || !key) return;
    if (mode === 'generate' && !pwSavedConfirmed) {
      submitError = 'Bitte bestätige, dass du das Passwort gespeichert hast.';
      return;
    }
    submitting = true;
    submitError = null;
    try {
      const { ciphertextB64, ivB64 } = await encrypt(secret, key);
      await publicApi.submit(token, ciphertextB64, ivB64);
      done = true;
    } catch (e) {
      submitError = (e as ApiError).message || 'Senden fehlgeschlagen';
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:head>
  <title>GoGrab — Geheimnis übermitteln</title>
  <meta name="robots" content="noindex" />
</svelte:head>

<main class="wrap">
  <h1>GoGrab</h1>

  {#if loading}
    <p class="muted">Lade …</p>
  {:else if loadError}
    <div class="card error">
      <h2>Nicht verfügbar</h2>
      <p>{loadError}</p>
    </div>
  {:else if !meta}
    <p class="muted">Keine Daten.</p>
  {:else if keyError}
    <div class="card error">
      <h2>Schlüssel fehlt</h2>
      <p>{keyError}</p>
    </div>
  {:else if meta.status === 'expired' || new Date(meta.expires_at) < new Date()}
    <div class="card error">
      <h2>Abgelaufen</h2>
      <p>Diese Anfrage ist abgelaufen und akzeptiert keine Einreichungen mehr.</p>
    </div>
  {:else if meta.status !== 'pending'}
    <div class="card success">
      <h2>Bereits eingereicht</h2>
      <p>Für diese Anfrage wurde bereits ein Geheimnis eingereicht.</p>
    </div>
  {:else if done}
    <div class="card success">
      <h2>Übermittelt</h2>
      <p>
        Dein Geheimnis wurde in deinem Browser verschlüsselt und sicher gesendet. Du kannst dieses
        Tab jetzt schließen.
      </p>
      {#if mode === 'generate'}
        <p class="hint">
          Vergiss nicht: das generierte Passwort liegt jetzt in deiner Zwischenablage / wo du es
          gespeichert hast. Der Empfänger sieht es nach dem Abruf nicht erneut.
        </p>
      {/if}
    </div>
  {:else}
    <div class="card">
      <p class="desc">{meta.description}</p>

      <div class="tabs" role="tablist">
        <button
          type="button"
          role="tab"
          aria-selected={mode === 'manual'}
          class="tab"
          class:active={mode === 'manual'}
          onclick={() => switchMode('manual')}
        >
          Selbst eingeben
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={mode === 'generate'}
          class="tab"
          class:active={mode === 'generate'}
          onclick={() => switchMode('generate')}
        >
          Passwort generieren
        </button>
      </div>

      <form onsubmit={submit}>
        {#if mode === 'manual'}
          <label for="secret">Dein Geheimnis</label>
          <textarea
            id="secret"
            required
            rows="6"
            maxlength="32000"
            bind:value={secret}
            placeholder="Bitte den angefragten Wert eingeben …"
          ></textarea>
        {:else}
          <div class="pw-display" data-reveal={pwReveal}>
            <span class="pw-text">{pwReveal ? secret : '•'.repeat(Math.min(secret.length, 32))}</span>
            <button
              type="button"
              class="icon-btn"
              onclick={() => (pwReveal = !pwReveal)}
              title={pwReveal ? 'Verbergen' : 'Anzeigen'}
              aria-label={pwReveal ? 'Verbergen' : 'Anzeigen'}
            >
              {pwReveal ? '🙈' : '👁'}
            </button>
          </div>

          <div class="pw-actions">
            <button type="button" class="btn-secondary" onclick={regenerate}>
              ↻ Neu generieren
            </button>
            <button type="button" class="btn-primary-sm" onclick={copySecret}>
              {pwCopied ? '✓ Kopiert' : '📋 Kopieren'}
            </button>
          </div>

          <div class="strength">
            <div class="strength-bar">
              <div
                class="strength-fill"
                style:width={Math.min(100, (entropy / 160) * 100) + '%'}
                style:background={strengthHex}
              ></div>
            </div>
            <span class="strength-label" style:color={strengthHex}>
              {strength} · {entropy} bits
            </span>
          </div>

          <details class="opts">
            <summary>Anpassen</summary>
            <div class="opts-row">
              <label class="opts-label">Länge</label>
              <div class="chips">
                {#each [16, 24, 32, 48] as n (n)}
                  <button
                    type="button"
                    class="chip"
                    class:active={pwOpts.length === n}
                    onclick={() => {
                      pwOpts = { ...pwOpts, length: n };
                      regenerate();
                    }}
                  >
                    {n}
                  </button>
                {/each}
              </div>
            </div>
            <div class="opts-row">
              <label class="opts-label">Sonderzeichen</label>
              <button
                type="button"
                class="chip"
                class:active={pwOpts.symbols}
                onclick={() => {
                  pwOpts = { ...pwOpts, symbols: !pwOpts.symbols };
                  regenerate();
                }}
              >
                {pwOpts.symbols ? 'an' : 'aus'}
              </button>
              <span class="hint-inline">
                {pwOpts.symbols ? '!@#$%^&*-_=+' : 'nur Buchstaben & Zahlen'}
              </span>
            </div>
          </details>

          <div class="save-warn">
            <strong>Speichere das Passwort jetzt selbst.</strong> Der Empfänger
            ruft es einmalig ab und es wird danach auf dem Server gelöscht.
            Du brauchst es z.B. für die Einrichtung selbst.
          </div>

          <label class="confirm">
            <input type="checkbox" bind:checked={pwSavedConfirmed} />
            <span>Ich habe das Passwort kopiert / gespeichert.</span>
          </label>
        {/if}

        {#if submitError}<p class="error-text">{submitError}</p>{/if}

        <button
          type="submit"
          class="btn-submit"
          disabled={submitting || !secret || (mode === 'generate' && !pwSavedConfirmed)}
        >
          {submitting ? 'Verschlüssele & sende …' : 'Sicher übermitteln'}
        </button>
      </form>

      <p class="note">
        Dein Eintrag wird direkt in deinem Browser verschlüsselt, bevor er gesendet wird. Der Server
        kann den Inhalt nicht lesen.
      </p>
    </div>
  {/if}

  <footer>Powered by GoGrab — Ende-zu-Ende verschlüsselte Geheimnis-Übermittlung.</footer>
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
  h1 {
    font-size: 1.25rem;
    font-weight: 600;
    margin: 0 0 1.5rem;
    color: #0f172a;
  }
  .muted {
    color: #64748b;
  }
  .card {
    background: #fff;
    border: 1px solid #e2e8f0;
    border-radius: 8px;
    padding: 1.25rem;
  }
  .card.error {
    border-color: #fecaca;
    background: #fef2f2;
  }
  .card.success {
    border-color: #bbf7d0;
    background: #f0fdf4;
  }
  .card h2 {
    font-size: 1rem;
    margin: 0 0 0.5rem;
  }
  .desc {
    font-weight: 500;
    margin: 0 0 1rem;
    color: #0f172a;
  }
  label {
    display: block;
    font-size: 0.875rem;
    font-weight: 500;
    margin-bottom: 0.25rem;
    color: #334155;
  }
  textarea {
    width: 100%;
    box-sizing: border-box;
    padding: 0.5rem;
    border: 1px solid #cbd5e1;
    border-radius: 4px;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.875rem;
    resize: vertical;
  }

  /* tabs */
  .tabs {
    display: flex;
    gap: 0.25rem;
    margin-bottom: 1rem;
    padding: 0.25rem;
    background: #f1f5f9;
    border-radius: 6px;
  }
  .tab {
    flex: 1;
    background: transparent;
    border: none;
    padding: 0.5rem 0.75rem;
    font-size: 0.875rem;
    color: #475569;
    cursor: pointer;
    border-radius: 4px;
    transition: background 0.15s, color 0.15s;
  }
  .tab:hover {
    color: #0f172a;
  }
  .tab.active {
    background: #fff;
    color: #0f172a;
    font-weight: 500;
    box-shadow: 0 1px 2px rgba(15, 23, 42, 0.08);
  }

  /* generator */
  .pw-display {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    background: #0f172a;
    color: #f1f5f9;
    border-radius: 6px;
    padding: 0.75rem 0.5rem 0.75rem 0.9rem;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.95rem;
    word-break: break-all;
  }
  .pw-display .pw-text {
    flex: 1;
    user-select: all;
    letter-spacing: 0.02em;
  }
  .icon-btn {
    background: rgba(255, 255, 255, 0.08);
    border: none;
    color: #f1f5f9;
    cursor: pointer;
    font-size: 0.9rem;
    padding: 0.25rem 0.4rem;
    border-radius: 4px;
  }
  .icon-btn:hover {
    background: rgba(255, 255, 255, 0.15);
  }

  .pw-actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.6rem;
  }
  .btn-secondary {
    background: #fff;
    border: 1px solid #cbd5e1;
    color: #334155;
    padding: 0.45rem 0.75rem;
    border-radius: 4px;
    font-size: 0.875rem;
    cursor: pointer;
  }
  .btn-secondary:hover {
    background: #f8fafc;
  }
  .btn-primary-sm {
    background: #0f172a;
    color: #fff;
    border: none;
    padding: 0.45rem 0.75rem;
    border-radius: 4px;
    font-size: 0.875rem;
    cursor: pointer;
    font-weight: 500;
  }
  .btn-primary-sm:hover {
    background: #1e293b;
  }

  .strength {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    margin-top: 0.75rem;
    font-size: 0.8125rem;
  }
  .strength-bar {
    flex: 1;
    height: 6px;
    background: #e2e8f0;
    border-radius: 3px;
    overflow: hidden;
  }
  .strength-fill {
    height: 100%;
    transition: width 0.2s, background 0.2s;
  }
  .strength-label {
    font-weight: 500;
    min-width: 8rem;
    text-align: right;
    font-variant-numeric: tabular-nums;
  }

  .opts {
    margin-top: 0.75rem;
    font-size: 0.875rem;
  }
  .opts > summary {
    cursor: pointer;
    color: #475569;
    user-select: none;
    padding: 0.25rem 0;
  }
  .opts > summary:hover {
    color: #0f172a;
  }
  .opts-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.5rem;
    flex-wrap: wrap;
  }
  .opts-label {
    margin: 0;
    width: 6rem;
    color: #475569;
    font-weight: 500;
  }
  .chips {
    display: flex;
    gap: 0.25rem;
  }
  .chip {
    background: #fff;
    border: 1px solid #cbd5e1;
    color: #475569;
    padding: 0.25rem 0.6rem;
    border-radius: 999px;
    font-size: 0.8125rem;
    cursor: pointer;
  }
  .chip:hover {
    border-color: #94a3b8;
  }
  .chip.active {
    background: #0f172a;
    border-color: #0f172a;
    color: #fff;
  }
  .hint-inline {
    color: #94a3b8;
    font-size: 0.8125rem;
  }

  .save-warn {
    margin-top: 1rem;
    background: #fffbeb;
    border: 1px solid #fde68a;
    color: #78350f;
    border-radius: 4px;
    padding: 0.6rem 0.75rem;
    font-size: 0.8125rem;
    line-height: 1.4;
  }
  .save-warn strong {
    color: #78350f;
  }

  .confirm {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.75rem;
    font-size: 0.875rem;
    color: #334155;
    font-weight: 400;
    cursor: pointer;
  }
  .confirm input {
    width: 1rem;
    height: 1rem;
    accent-color: #0f172a;
  }

  /* submit */
  .btn-submit {
    margin-top: 1rem;
    background: #0f172a;
    color: #fff;
    border: none;
    padding: 0.65rem 1rem;
    border-radius: 4px;
    font-weight: 500;
    cursor: pointer;
    font-size: 0.9375rem;
    width: 100%;
  }
  .btn-submit:hover:not(:disabled) {
    background: #1e293b;
  }
  .btn-submit:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

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
</style>
