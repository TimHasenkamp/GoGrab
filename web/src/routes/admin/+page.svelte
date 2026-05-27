<script lang="ts">
  import { onMount } from 'svelte';
  import { adminApi, type AdminRequestSummary, type ApiError } from '$lib/api';
  import {
    relativeTime,
    absoluteTime,
    statusLabel,
    statusBadge,
    statusDot,
    type Status
  } from '$lib/format';

  let requests = $state<AdminRequestSummary[]>([]);
  let total = $state(0);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let filter = $state<Status | 'all'>('all');

  const PAGE_SIZE = 50;
  let searchInput = $state('');
  let search = $state('');
  let offset = $state(0);
  let searchTimer: ReturnType<typeof setTimeout> | null = null;

  async function refresh() {
    loading = true;
    error = null;
    try {
      const res = await adminApi.list({ q: search, limit: PAGE_SIZE, offset });
      requests = res.items;
      total = res.total;
    } catch (e) {
      error = (e as ApiError).message || 'Konnte Liste nicht laden';
    } finally {
      loading = false;
    }
  }

  onMount(refresh);

  // Debounced search: wait 250ms after the last keystroke before refetching.
  function onSearchInput(v: string) {
    searchInput = v;
    if (searchTimer) clearTimeout(searchTimer);
    searchTimer = setTimeout(() => {
      search = searchInput.trim();
      offset = 0;
      void refresh();
    }, 250);
  }

  function go(delta: number) {
    const next = offset + delta;
    if (next < 0 || next >= total) return;
    offset = next;
    void refresh();
  }

  const page = $derived(Math.floor(offset / PAGE_SIZE) + 1);
  const lastPage = $derived(Math.max(1, Math.ceil(total / PAGE_SIZE)));

  const counts = $derived.by(() => {
    const c: Record<Status, number> = { pending: 0, submitted: 0, retrieved: 0, expired: 0 };
    for (const r of requests) c[r.status as Status]++;
    return c;
  });

  const filtered = $derived(
    filter === 'all' ? requests : requests.filter((r) => r.status === filter)
  );

  function statusLine(r: AdminRequestSummary): string {
    if (r.status === 'submitted' && r.submitted_at) {
      return `Eingereicht ${relativeTime(r.submitted_at)}`;
    }
    if (r.status === 'retrieved' && r.retrieved_at) {
      return `Abgerufen ${relativeTime(r.retrieved_at)}`;
    }
    if (r.status === 'expired') {
      return `Abgelaufen ${relativeTime(r.expires_at)}`;
    }
    return `Läuft ab ${relativeTime(r.expires_at)}`;
  }
</script>

<svelte:head><title>GoGrab — Requests</title></svelte:head>

<div class="mx-auto max-w-5xl px-6 py-8">
  <div class="mb-6 flex flex-wrap items-start justify-between gap-4">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight text-slate-900">Deine Requests</h1>
      <p class="mt-1 text-sm text-slate-600">
        Erstelle einen verschlüsselten Link, schicke ihn an deinen Kunden, lese die Antwort einmalig aus.
      </p>
    </div>
    <label class="relative block w-full sm:w-64">
      <span class="sr-only">Suchen</span>
      <svg class="absolute left-2.5 top-2.5 h-4 w-4 text-slate-400" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <circle cx="11" cy="11" r="8" />
        <line x1="21" y1="21" x2="16.65" y2="16.65" />
      </svg>
      <input
        type="search"
        placeholder="Beschreibung durchsuchen…"
        value={searchInput}
        oninput={(e) => onSearchInput((e.currentTarget as HTMLInputElement).value)}
        class="block w-full rounded-md border border-slate-300 bg-white pl-8 pr-3 py-2 text-sm shadow-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500"
      />
    </label>
  </div>

  <!-- Stats -->
  <div class="mb-6 grid grid-cols-2 gap-3 sm:grid-cols-4">
    {#each [
      { key: 'pending', label: 'Offen' },
      { key: 'submitted', label: 'Bereit' },
      { key: 'retrieved', label: 'Abgerufen' },
      { key: 'expired', label: 'Abgelaufen' }
    ] as s (s.key)}
      <button
        type="button"
        onclick={() => (filter = filter === (s.key as Status) ? 'all' : (s.key as Status))}
        class="group rounded-lg border bg-white p-3 text-left transition {filter === s.key
          ? 'border-slate-900 ring-2 ring-slate-900/10'
          : 'border-slate-200 hover:border-slate-300'}"
      >
        <div class="flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-slate-500">
          <span class="inline-block h-2 w-2 rounded-full {statusDot[s.key as Status]}"></span>
          {s.label}
        </div>
        <div class="mt-1 text-2xl font-semibold tabular-nums text-slate-900">
          {counts[s.key as Status]}
        </div>
      </button>
    {/each}
  </div>

  <!-- Filter pill row -->
  <div class="mb-4 flex flex-wrap items-center gap-2">
    <button
      type="button"
      onclick={() => (filter = 'all')}
      class="rounded-full px-3 py-1 text-xs font-medium {filter === 'all'
        ? 'bg-slate-900 text-white'
        : 'bg-slate-100 text-slate-700 hover:bg-slate-200'}"
    >
      Alle ({requests.length})
    </button>
    {#each ['pending', 'submitted', 'retrieved', 'expired'] as s (s)}
      {#if counts[s as Status] > 0}
        <button
          type="button"
          onclick={() => (filter = s as Status)}
          class="rounded-full px-3 py-1 text-xs font-medium {filter === s
            ? 'bg-slate-900 text-white'
            : 'bg-slate-100 text-slate-700 hover:bg-slate-200'}"
        >
          {statusLabel[s as Status]} ({counts[s as Status]})
        </button>
      {/if}
    {/each}
    <div class="ml-auto">
      <button
        type="button"
        onclick={refresh}
        class="rounded-md px-2 py-1 text-xs text-slate-500 hover:bg-slate-100 hover:text-slate-900"
        title="Neu laden"
      >
        ↻ Aktualisieren
      </button>
    </div>
  </div>

  <!-- List -->
  {#if loading}
    <div class="space-y-2">
      {#each Array(3) as _, i (i)}
        <div class="h-20 animate-pulse rounded-lg border border-slate-200 bg-white"></div>
      {/each}
    </div>
  {:else if error}
    <div class="rounded-lg border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800">
      Fehler: {error}
    </div>
  {:else if requests.length === 0}
    <!-- Empty state -->
    <div class="rounded-lg border border-dashed border-slate-300 bg-white py-16 text-center">
      <div class="mx-auto mb-4 grid h-12 w-12 place-items-center rounded-full bg-slate-100 text-slate-500">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
          <polyline points="14 2 14 8 20 8" />
          <line x1="12" y1="12" x2="12" y2="18" />
          <line x1="9" y1="15" x2="15" y2="15" />
        </svg>
      </div>
      <h2 class="text-base font-semibold text-slate-900">Noch keine Requests</h2>
      <p class="mx-auto mt-1 max-w-md text-sm text-slate-600">
        Lege einen Secret-Request an, kopiere den Link aus dem Browser und schicke ihn deinem Kunden.
        Der Kunde gibt das Geheimnis ein — du holst es einmalig ab.
      </p>
      <a
        href="/admin/new"
        class="mt-5 inline-flex items-center gap-1.5 rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-slate-800"
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
          <line x1="12" y1="5" x2="12" y2="19" />
          <line x1="5" y1="12" x2="19" y2="12" />
        </svg>
        Ersten Request anlegen
      </a>
    </div>
  {:else if filtered.length === 0}
    <div class="rounded-lg border border-dashed border-slate-300 bg-white py-12 text-center text-sm text-slate-500">
      Keine Requests mit Filter „{statusLabel[filter as Status]}".
    </div>
  {:else}
    <ul class="space-y-2">
      {#each filtered as r (r.id)}
        <li>
          <a
            href="/admin/{r.id}"
            class="flex items-center justify-between gap-4 rounded-lg border border-slate-200 bg-white p-4 transition hover:border-slate-300 hover:shadow-sm"
          >
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="inline-block h-2 w-2 shrink-0 rounded-full {statusDot[r.status as Status]}"></span>
                <h3 class="truncate text-sm font-medium text-slate-900">{r.description}</h3>
              </div>
              <p class="mt-1 ml-4 truncate text-xs text-slate-500">
                {statusLine(r)}
                <span class="mx-1">·</span>
                <span title={absoluteTime(r.created_at)}>angelegt {relativeTime(r.created_at)}</span>
              </p>
            </div>
            <span class="shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ring-1 {statusBadge[r.status as Status]}">
              {statusLabel[r.status as Status]}
            </span>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="shrink-0 text-slate-400">
              <polyline points="9 18 15 12 9 6" />
            </svg>
          </a>
        </li>
      {/each}
    </ul>

    {#if total > PAGE_SIZE}
      <div class="mt-4 flex items-center justify-between text-sm text-slate-600">
        <span>
          {offset + 1}–{Math.min(offset + requests.length, total)} von {total}
          {#if search}<span class="ml-1 text-slate-400">(gefiltert)</span>{/if}
        </span>
        <div class="flex items-center gap-1">
          <button
            type="button"
            onclick={() => go(-PAGE_SIZE)}
            disabled={offset === 0}
            class="rounded-md border border-slate-300 bg-white px-2 py-1 text-xs hover:bg-slate-50 disabled:opacity-40"
          >
            ← Vor
          </button>
          <span class="px-2 text-xs text-slate-500">Seite {page} / {lastPage}</span>
          <button
            type="button"
            onclick={() => go(PAGE_SIZE)}
            disabled={offset + PAGE_SIZE >= total}
            class="rounded-md border border-slate-300 bg-white px-2 py-1 text-xs hover:bg-slate-50 disabled:opacity-40"
          >
            Weiter →
          </button>
        </div>
      </div>
    {/if}
  {/if}
</div>
