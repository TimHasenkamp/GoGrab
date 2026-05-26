<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { adminApi, type AdminRequestSummary, type ApiError } from '$lib/api';
  import { importKeyB64url, decrypt } from '$lib/crypto';
  import {
    relativeTime,
    absoluteTime,
    statusLabel,
    statusBadge,
    type Status
  } from '$lib/format';

  const id = $derived($page.params.id ?? '');

  let request = $state<AdminRequestSummary | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let keyInput = $state('');
  let decrypted = $state<string | null>(null);
  let decrypting = $state(false);
  let decryptError = $state<string | null>(null);
  let copiedSecret = $state(false);

  async function load() {
    loading = true;
    error = null;
    try {
      request = await adminApi.get(id);
    } catch (e) {
      error = (e as ApiError).message || 'Konnte Request nicht laden';
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
      await load();
    } catch (e) {
      decryptError =
        (e as ApiError).message || (e as Error).message || 'Entschlüsselung fehlgeschlagen';
    } finally {
      decrypting = false;
    }
  }

  async function copySecret() {
    if (decrypted === null) return;
    await navigator.clipboard.writeText(decrypted);
    copiedSecret = true;
    setTimeout(() => (copiedSecret = false), 1500);
  }

  async function cancel() {
    if (!confirm('Diesen Request endgültig löschen?')) return;
    try {
      await adminApi.remove(id);
      history.back();
    } catch (e) {
      error = (e as ApiError).message || 'Konnte nicht löschen';
    }
  }

  // status-driven timeline steps (current, past, pending)
  function timelineState(r: AdminRequestSummary): { step: string; state: 'done' | 'current' | 'todo' }[] {
    const out: { step: string; state: 'done' | 'current' | 'todo' }[] = [];
    out.push({ step: 'Angelegt', state: 'done' });
    if (r.status === 'pending') {
      out.push({ step: 'Warte auf Einreichung', state: 'current' });
      out.push({ step: 'Abrufen', state: 'todo' });
    } else if (r.status === 'submitted') {
      out.push({ step: 'Eingereicht', state: 'done' });
      out.push({ step: 'Bereit zum Abruf', state: 'current' });
    } else if (r.status === 'retrieved') {
      out.push({ step: 'Eingereicht', state: 'done' });
      out.push({ step: 'Abgerufen & gelöscht', state: 'done' });
    } else if (r.status === 'expired') {
      out.push({ step: 'Abgelaufen', state: 'current' });
    }
    return out;
  }
</script>

<svelte:head><title>GoGrab — Request</title></svelte:head>

<div class="mx-auto max-w-2xl px-6 py-8">
  <a href="/admin" class="mb-4 inline-flex items-center gap-1 text-sm text-slate-500 hover:text-slate-900">
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="15 18 9 12 15 6" />
    </svg>
    Zurück zur Liste
  </a>

  {#if loading}
    <div class="h-40 animate-pulse rounded-lg border border-slate-200 bg-white"></div>
  {:else if error}
    <div class="rounded-lg border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800">
      {error}
    </div>
  {:else if request}
    <!-- Header card -->
    <div class="rounded-xl border border-slate-200 bg-white shadow-sm">
      <div class="border-b border-slate-100 px-6 py-5">
        <div class="flex items-start justify-between gap-3">
          <h1 class="text-lg font-semibold text-slate-900">{request.description}</h1>
          <span class="shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ring-1 {statusBadge[request.status as Status]}">
            {statusLabel[request.status as Status]}
          </span>
        </div>
        <dl class="mt-3 grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
          <dt class="text-slate-500">Angelegt</dt>
          <dd class="text-slate-900" title={absoluteTime(request.created_at)}>
            {relativeTime(request.created_at)}
          </dd>
          <dt class="text-slate-500">Läuft ab</dt>
          <dd class="text-slate-900" title={absoluteTime(request.expires_at)}>
            {relativeTime(request.expires_at)}
          </dd>
          {#if request.submitted_at}
            <dt class="text-slate-500">Eingereicht</dt>
            <dd class="text-slate-900" title={absoluteTime(request.submitted_at)}>
              {relativeTime(request.submitted_at)}
            </dd>
          {/if}
          {#if request.retrieved_at}
            <dt class="text-slate-500">Abgerufen</dt>
            <dd class="text-slate-900" title={absoluteTime(request.retrieved_at)}>
              {relativeTime(request.retrieved_at)}
            </dd>
          {/if}
        </dl>
      </div>

      <!-- Timeline -->
      <div class="border-b border-slate-100 px-6 py-4">
        <ol class="space-y-2">
          {#each timelineState(request) as t (t.step)}
            <li class="flex items-center gap-3 text-sm">
              {#if t.state === 'done'}
                <span class="grid h-5 w-5 place-items-center rounded-full bg-emerald-500 text-white">
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12" /></svg>
                </span>
                <span class="text-slate-700">{t.step}</span>
              {:else if t.state === 'current'}
                <span class="grid h-5 w-5 place-items-center rounded-full bg-slate-900">
                  <span class="h-1.5 w-1.5 animate-pulse rounded-full bg-white"></span>
                </span>
                <span class="font-medium text-slate-900">{t.step}</span>
              {:else}
                <span class="h-5 w-5 rounded-full border-2 border-dashed border-slate-300"></span>
                <span class="text-slate-400">{t.step}</span>
              {/if}
            </li>
          {/each}
        </ol>
      </div>

      <!-- Action panel -->
      <div class="p-6">
        {#if request.status === 'pending'}
          <div class="rounded-md border border-slate-200 bg-slate-50 p-4 text-sm">
            <p class="font-medium text-slate-900">Wartet auf den Kunden</p>
            <p class="mt-1 text-slate-600">
              Sobald der Kunde einreicht, kannst du das Geheimnis hier mit deinem Schlüssel abrufen.
              Hast du den Share-Link parat?
            </p>
            <button
              type="button"
              onclick={cancel}
              class="mt-3 inline-flex items-center gap-1.5 rounded-md border border-rose-200 bg-white px-3 py-1.5 text-xs font-medium text-rose-700 hover:bg-rose-50"
            >
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="3 6 5 6 21 6" />
                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
              </svg>
              Request löschen
            </button>
          </div>
        {:else if request.status === 'submitted'}
          <div>
            <h2 class="text-sm font-semibold text-slate-900">Geheimnis abrufen</h2>
            <p class="mt-1 text-xs text-slate-600">
              Füge den vollständigen Share-Link (oder nur den Teil hinter <code class="rounded bg-slate-100 px-1">#</code>) ein.
              Der Abruf ist einmalig — danach wird der Chiffretext serverseitig gelöscht.
            </p>
            <form onsubmit={reveal} class="mt-4 space-y-3">
              <input
                required
                bind:value={keyInput}
                placeholder="https://…/r/TOKEN#SCHLÜSSEL  oder  SCHLÜSSEL"
                class="block w-full rounded-md border border-slate-300 px-3 py-2 font-mono text-xs shadow-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500"
              />
              {#if decryptError}
                <p class="rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800">{decryptError}</p>
              {/if}
              <button
                type="submit"
                disabled={decrypting || !keyInput}
                class="inline-flex items-center gap-1.5 rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {#if decrypting}
                  Entschlüssele …
                {:else}
                  Abrufen & entschlüsseln
                {/if}
              </button>
            </form>
          </div>
        {:else if request.status === 'expired'}
          <div class="rounded-md border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800">
            Dieser Request ist abgelaufen, ohne dass der Kunde eingereicht hat.
          </div>
        {:else if request.status === 'retrieved'}
          <div class="rounded-md border border-slate-200 bg-slate-50 p-4 text-sm text-slate-700">
            Das Geheimnis wurde bereits abgerufen und auf dem Server gelöscht. Es ist nicht erneut abrufbar.
          </div>
        {/if}
      </div>
    </div>

    {#if decrypted !== null}
      <div class="mt-4 rounded-xl border border-emerald-200 bg-white shadow-sm">
        <div class="flex items-center justify-between gap-3 border-b border-emerald-100 bg-emerald-50 px-6 py-3">
          <div class="flex items-center gap-2">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" class="text-emerald-700">
              <polyline points="20 6 9 17 4 12" />
            </svg>
            <h2 class="text-sm font-semibold text-emerald-900">Entschlüsseltes Geheimnis</h2>
          </div>
          <button
            type="button"
            onclick={copySecret}
            class="inline-flex items-center gap-1.5 rounded-md bg-emerald-700 px-3 py-1 text-xs font-medium text-white hover:bg-emerald-800"
          >
            {copiedSecret ? 'Kopiert' : 'Kopieren'}
          </button>
        </div>
        <pre class="whitespace-pre-wrap break-words p-4 font-mono text-sm text-slate-900">{decrypted}</pre>
        <p class="border-t border-emerald-100 px-6 py-2 text-xs text-emerald-700">
          Schließe dieses Tab oder kopiere den Wert, sobald du fertig bist. Der Server hat keinen Zugriff mehr darauf.
        </p>
      </div>
    {/if}
  {/if}
</div>
