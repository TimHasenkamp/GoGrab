<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { adminApi, type AdminRequestSummary, type ApiError } from '$lib/api';
  import { importKeyB64url, decrypt } from '$lib/crypto';

  const id = $derived($page.params.id ?? '');

  let request = $state<AdminRequestSummary | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let keyInput = $state('');
  let decrypted = $state<string | null>(null);
  let decrypting = $state(false);
  let decryptError = $state<string | null>(null);

  async function load() {
    loading = true;
    error = null;
    try {
      request = await adminApi.get(id);
    } catch (e) {
      error = (e as ApiError).message || 'failed to load';
    } finally {
      loading = false;
    }
  }

  onMount(load);

  function extractKey(input: string): string {
    const trimmed = input.trim();
    const hashIdx = trimmed.indexOf('#');
    return hashIdx >= 0 ? trimmed.slice(hashIdx + 1) : trimmed;
  }

  async function reveal(e: Event) {
    e.preventDefault();
    if (decrypting) return;
    decrypting = true;
    decryptError = null;
    decrypted = null;
    try {
      const keyB64 = extractKey(keyInput);
      const key = await importKeyB64url(keyB64);
      const payload = await adminApi.retrieve(id);
      decrypted = await decrypt(payload.ciphertext_b64, payload.iv_b64, key);
      // refresh status (retrieved + ciphertext purged)
      await load();
    } catch (e) {
      decryptError = (e as ApiError).message || (e as Error).message || 'decryption failed';
    } finally {
      decrypting = false;
    }
  }

  async function cancel() {
    if (!confirm('Cancel this request? This cannot be undone.')) return;
    try {
      await adminApi.remove(id);
      history.back();
    } catch (e) {
      error = (e as ApiError).message || 'failed to cancel';
    }
  }
</script>

<svelte:head><title>GoGrab — Request</title></svelte:head>

<main class="mx-auto max-w-2xl p-6">
  <a href="/admin" class="text-sm text-slate-500 hover:underline">&larr; Back</a>

  {#if loading}
    <p class="mt-6 text-slate-500">Loading…</p>
  {:else if error}
    <p class="mt-6 text-rose-600">Error: {error}</p>
  {:else if request}
    <h1 class="mb-1 mt-2 text-2xl font-semibold text-slate-900">{request.description}</h1>
    <dl class="mb-6 grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
      <dt class="text-slate-500">Status</dt>
      <dd class="text-slate-900">{request.status}</dd>
      <dt class="text-slate-500">Created</dt>
      <dd class="text-slate-900">{new Date(request.created_at).toLocaleString()}</dd>
      <dt class="text-slate-500">Expires</dt>
      <dd class="text-slate-900">{new Date(request.expires_at).toLocaleString()}</dd>
      {#if request.submitted_at}
        <dt class="text-slate-500">Submitted</dt>
        <dd class="text-slate-900">{new Date(request.submitted_at).toLocaleString()}</dd>
      {/if}
      {#if request.retrieved_at}
        <dt class="text-slate-500">Retrieved</dt>
        <dd class="text-slate-900">{new Date(request.retrieved_at).toLocaleString()}</dd>
      {/if}
    </dl>

    {#if request.status === 'submitted'}
      <section class="rounded border border-slate-200 bg-white p-4">
        <h2 class="mb-2 text-sm font-semibold text-slate-900">Decrypt secret</h2>
        <p class="mb-3 text-xs text-slate-600">
          Paste the original share URL (or just the key after <code>#</code>). Retrieval is
          one-shot — the server deletes the ciphertext after this call.
        </p>
        <form onsubmit={reveal} class="space-y-3">
          <input
            required
            bind:value={keyInput}
            placeholder="https://…/r/TOKEN#KEY  or  KEY"
            class="block w-full rounded border border-slate-300 px-3 py-2 font-mono text-xs"
          />
          {#if decryptError}<p class="text-sm text-rose-600">{decryptError}</p>{/if}
          <button
            type="submit"
            disabled={decrypting || !keyInput}
            class="rounded bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-700 disabled:opacity-50"
          >
            {decrypting ? 'Decrypting…' : 'Retrieve & decrypt'}
          </button>
        </form>
      </section>
    {:else if request.status === 'pending'}
      <p class="text-sm text-slate-500">Waiting for customer to submit…</p>
      <button
        onclick={cancel}
        class="mt-4 rounded border border-rose-300 px-3 py-1.5 text-sm text-rose-700 hover:bg-rose-50"
      >
        Cancel request
      </button>
    {:else if request.status === 'expired'}
      <p class="text-sm text-rose-600">This request has expired.</p>
    {:else if request.status === 'retrieved'}
      <p class="text-sm text-slate-500">
        Ciphertext has been retrieved and deleted from the server.
      </p>
    {/if}

    {#if decrypted !== null}
      <section class="mt-6 rounded border border-emerald-200 bg-emerald-50 p-4">
        <h2 class="mb-2 text-sm font-semibold text-emerald-900">Decrypted secret</h2>
        <pre class="whitespace-pre-wrap break-words font-mono text-sm text-slate-900">{decrypted}</pre>
      </section>
    {/if}
  {/if}
</main>
