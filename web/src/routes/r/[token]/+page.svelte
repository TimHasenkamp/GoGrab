<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { publicApi, type PublicMeta, type ApiError } from '$lib/api';
  import { importKeyB64url, encrypt } from '$lib/crypto';

  const token = $derived($page.params.token ?? '');

  let meta = $state<PublicMeta | null>(null);
  let loading = $state(true);
  let loadError = $state<string | null>(null);
  let keyError = $state<string | null>(null);

  let secret = $state('');
  let submitting = $state(false);
  let submitError = $state<string | null>(null);
  let done = $state(false);

  let key = $state<CryptoKey | null>(null);

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
    submitting = true;
    submitError = null;
    try {
      const { ciphertextB64, ivB64 } = await encrypt(secret, key);
      await publicApi.submit(token, ciphertextB64, ivB64);
      done = true;
      secret = '';
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
    </div>
  {:else}
    <div class="card">
      <p class="desc">{meta.description}</p>
      <form onsubmit={submit}>
        <label for="secret">Dein Geheimnis</label>
        <textarea
          id="secret"
          required
          rows="6"
          maxlength="32000"
          bind:value={secret}
          placeholder="Bitte den angefragten Wert eingeben …"
        ></textarea>
        {#if submitError}<p class="error-text">{submitError}</p>{/if}
        <button type="submit" disabled={submitting || !secret}>
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
  button {
    margin-top: 1rem;
    background: #0f172a;
    color: #fff;
    border: none;
    padding: 0.6rem 1rem;
    border-radius: 4px;
    font-weight: 500;
    cursor: pointer;
    font-size: 0.875rem;
  }
  button:hover:not(:disabled) {
    background: #1e293b;
  }
  button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .note {
    margin-top: 1rem;
    font-size: 0.75rem;
    color: #64748b;
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
