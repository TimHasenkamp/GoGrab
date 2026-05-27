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
  import Icon from '$lib/Icon.svelte';

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
    if (r.status === 'submitted' && r.submitted_at) return `Eingereicht ${relativeTime(r.submitted_at)}`;
    if (r.status === 'retrieved' && r.retrieved_at) return `Abgerufen ${relativeTime(r.retrieved_at)}`;
    if (r.status === 'expired') return `Abgelaufen ${relativeTime(r.expires_at)}`;
    return `Läuft ab ${relativeTime(r.expires_at)}`;
  }
</script>

<svelte:head><title>GoGrab — Requests</title></svelte:head>

<div class="mx-auto max-w-5xl px-6 py-8">
  <div class="mb-6 flex flex-wrap items-start justify-between gap-4">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight text-foreground">
        Deine Requests<span class="text-accent">.</span>
      </h1>
      <p class="mt-1 text-sm text-muted-foreground">
        Erstelle einen verschlüsselten Link, schicke ihn an deinen Kunden, lese die Antwort einmalig aus.
      </p>
    </div>
    <label class="relative block w-full sm:w-64">
      <span class="sr-only">Suchen</span>
      <span class="pointer-events-none absolute left-2.5 top-2.5 text-muted-foreground">
        <Icon name="search" size={16} />
      </span>
      <input
        type="search"
        placeholder="Beschreibung durchsuchen…"
        value={searchInput}
        oninput={(e) => onSearchInput((e.currentTarget as HTMLInputElement).value)}
        class="block w-full rounded-md border border-border bg-card pl-8 pr-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
      />
    </label>
  </div>

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
        class="group rounded-lg border bg-card p-3 text-left transition {filter === s.key
          ? 'border-accent shadow-accent-glow'
          : 'border-border hover:border-border-strong'}"
      >
        <div class="flex items-center gap-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
          <span class="inline-block h-2 w-2 rounded-full {statusDot[s.key as Status]}"></span>
          {s.label}
        </div>
        <div class="mt-1 text-2xl font-semibold tabular-nums tracking-tight text-foreground">
          {counts[s.key as Status]}
        </div>
      </button>
    {/each}
  </div>

  <div class="mb-4 flex flex-wrap items-center gap-2">
    <button
      type="button"
      onclick={() => (filter = 'all')}
      class="rounded-full px-3 py-1 text-xs font-medium {filter === 'all'
        ? 'bg-accent text-background'
        : 'bg-muted text-muted-foreground hover:text-foreground'}"
    >
      Alle ({requests.length})
    </button>
    {#each ['pending', 'submitted', 'retrieved', 'expired'] as s (s)}
      {#if counts[s as Status] > 0}
        <button
          type="button"
          onclick={() => (filter = s as Status)}
          class="rounded-full px-3 py-1 text-xs font-medium {filter === s
            ? 'bg-accent text-background'
            : 'bg-muted text-muted-foreground hover:text-foreground'}"
        >
          {statusLabel[s as Status]} ({counts[s as Status]})
        </button>
      {/if}
    {/each}
    <div class="ml-auto">
      <button
        type="button"
        onclick={refresh}
        class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:text-foreground"
        title="Neu laden"
      >
        <Icon name="refresh-cw" size={12} />
        <span>Aktualisieren</span>
      </button>
    </div>
  </div>

  {#if loading}
    <div class="space-y-2">
      {#each Array(3) as _, i (i)}
        <div class="h-20 animate-pulse rounded-lg border border-border bg-card"></div>
      {/each}
    </div>
  {:else if error}
    <div class="rounded-lg border border-danger/30 bg-danger/10 p-4 text-sm text-danger">
      Fehler: {error}
    </div>
  {:else if requests.length === 0}
    <div class="rounded-lg border border-dashed border-border-strong bg-card py-16 text-center">
      <div class="mx-auto mb-4 grid h-12 w-12 place-items-center rounded-full bg-accent/10 text-accent">
        <Icon name="file-text" size={24} strokeWidth={1.8} />
      </div>
      <h2 class="text-base font-semibold tracking-tight text-foreground">Noch keine Requests</h2>
      <p class="mx-auto mt-1 max-w-md text-sm text-muted-foreground">
        Lege einen Secret-Request an, kopiere den Link aus dem Browser und schicke ihn deinem Kunden.
        Der Kunde gibt das Geheimnis ein — du holst es einmalig ab.
      </p>
      <a
        href="/admin/new"
        class="mt-5 inline-flex items-center gap-1.5 rounded-md bg-accent px-4 py-2 text-sm font-medium text-background transition hover:bg-accent-hover hover:shadow-accent-glow"
      >
        <Icon name="plus" size={14} strokeWidth={2.5} />
        Ersten Request anlegen
      </a>
    </div>
  {:else if filtered.length === 0}
    <div class="rounded-lg border border-dashed border-border-strong bg-card py-12 text-center text-sm text-muted-foreground">
      Keine Requests mit Filter „{statusLabel[filter as Status]}".
    </div>
  {:else}
    <ul class="space-y-2">
      {#each filtered as r (r.id)}
        <li>
          <a
            href="/admin/{r.id}"
            class="group flex items-center justify-between gap-4 rounded-lg border border-border bg-card p-4 transition hover:border-accent hover:shadow-accent-glow"
          >
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="inline-block h-2 w-2 shrink-0 rounded-full {statusDot[r.status as Status]}"></span>
                <h3 class="truncate text-sm font-medium text-foreground">{r.description}</h3>
              </div>
              <p class="mt-1 ml-4 truncate text-xs text-muted-foreground">
                {statusLine(r)}
                <span class="mx-1">·</span>
                <span title={absoluteTime(r.created_at)}>angelegt {relativeTime(r.created_at)}</span>
              </p>
            </div>
            <span class="shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ring-1 {statusBadge[r.status as Status]}">
              {statusLabel[r.status as Status]}
            </span>
            <span class="shrink-0 text-muted-foreground group-hover:text-accent">
              <Icon name="chevron-right" size={16} />
            </span>
          </a>
        </li>
      {/each}
    </ul>

    {#if total > PAGE_SIZE}
      <div class="mt-4 flex items-center justify-between text-sm text-muted-foreground">
        <span>
          {offset + 1}–{Math.min(offset + requests.length, total)} von {total}
          {#if search}<span class="ml-1 text-muted-foreground/60">(gefiltert)</span>{/if}
        </span>
        <div class="flex items-center gap-1">
          <button
            type="button"
            onclick={() => go(-PAGE_SIZE)}
            disabled={offset === 0}
            class="inline-flex items-center gap-1 rounded-md border border-border bg-card px-2 py-1 text-xs text-foreground hover:border-border-strong disabled:opacity-40"
          >
            <Icon name="chevron-left" size={14} />
            <span>Vor</span>
          </button>
          <span class="px-2 text-xs text-muted-foreground">Seite {page} / {lastPage}</span>
          <button
            type="button"
            onclick={() => go(PAGE_SIZE)}
            disabled={offset + PAGE_SIZE >= total}
            class="inline-flex items-center gap-1 rounded-md border border-border bg-card px-2 py-1 text-xs text-foreground hover:border-border-strong disabled:opacity-40"
          >
            <span>Weiter</span>
            <Icon name="chevron-right" size={14} />
          </button>
        </div>
      </div>
    {/if}
  {/if}
</div>
