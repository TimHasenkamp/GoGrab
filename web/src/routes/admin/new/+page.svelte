<script lang="ts">
  import { goto } from '$app/navigation';
  import { adminApi, type ApiError } from '$lib/api';
  import { generateKey, exportKeyB64url } from '$lib/crypto';

  let description = $state('');
  let expiresInHours = $state(72);
  let submitting = $state(false);
  let error = $state<string | null>(null);
  let shareUrl = $state<string | null>(null);
  let requestId = $state<string | null>(null);
  let copied = $state(false);

  async function submit(e: Event) {
    e.preventDefault();
    if (submitting) return;
    submitting = true;
    error = null;
    try {
      const key = await generateKey();
      const keyB64 = await exportKeyB64url(key);
      const res = await adminApi.create(description, expiresInHours);
      requestId = res.request_id;
      shareUrl = `${location.origin}/r/${res.token}#${keyB64}`;
    } catch (e) {
      error = (e as ApiError).message || 'failed to create';
    } finally {
      submitting = false;
    }
  }

  async function copyShare() {
    if (!shareUrl) return;
    await navigator.clipboard.writeText(shareUrl);
    copied = true;
    setTimeout(() => (copied = false), 1500);
  }
</script>

<svelte:head><title>GoGrab — New request</title></svelte:head>

<main class="mx-auto max-w-xl p-6">
  <a href="/admin" class="text-sm text-slate-500 hover:underline">&larr; Back</a>
  <h1 class="mb-6 mt-2 text-2xl font-semibold text-slate-900">New secret request</h1>

  {#if shareUrl}
    <div class="space-y-4 rounded border border-emerald-200 bg-emerald-50 p-4">
      <p class="text-sm font-medium text-emerald-900">
        Share this URL with the customer. The key after <code>#</code> never reaches our server —
        if you lose this URL, the secret cannot be decrypted.
      </p>
      <textarea
        readonly
        rows="3"
        class="w-full break-all rounded border border-emerald-300 bg-white p-2 font-mono text-xs text-slate-900"
        >{shareUrl}</textarea
      >
      <div class="flex gap-2">
        <button
          onclick={copyShare}
          class="rounded bg-emerald-700 px-3 py-1.5 text-sm font-medium text-white hover:bg-emerald-800"
        >
          {copied ? 'Copied!' : 'Copy URL'}
        </button>
        <a
          href="/admin/{requestId}"
          class="rounded border border-slate-300 px-3 py-1.5 text-sm text-slate-700 hover:bg-slate-100"
          >Open detail</a
        >
      </div>
    </div>
  {:else}
    <form onsubmit={submit} class="space-y-4">
      <label class="block">
        <span class="text-sm font-medium text-slate-700">Description (shown to customer)</span>
        <input
          required
          maxlength="200"
          bind:value={description}
          class="mt-1 block w-full rounded border border-slate-300 px-3 py-2 text-sm"
          placeholder="e.g. Bitte hinterlege dein WLAN-Passwort"
        />
      </label>
      <label class="block">
        <span class="text-sm font-medium text-slate-700">Expires in (hours)</span>
        <input
          type="number"
          min="1"
          max="720"
          bind:value={expiresInHours}
          class="mt-1 block w-32 rounded border border-slate-300 px-3 py-2 text-sm"
        />
      </label>
      {#if error}<p class="text-sm text-rose-600">{error}</p>{/if}
      <button
        type="submit"
        disabled={submitting || !description}
        class="rounded bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-700 disabled:opacity-50"
      >
        {submitting ? 'Creating…' : 'Create request'}
      </button>
    </form>
  {/if}
</main>
