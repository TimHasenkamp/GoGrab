<script lang="ts">
  import { onMount } from 'svelte';
  import { auditApi, type AuditEntry, type ApiError } from '$lib/api';
  import { relativeTime, absoluteTime } from '$lib/format';
  import Icon from '$lib/Icon.svelte';

  let entries = $state<AuditEntry[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let filter = $state<string>('all');

  async function refresh() {
    loading = true;
    error = null;
    try {
      entries = await auditApi.list(200);
    } catch (e) {
      error = (e as ApiError).message || 'Konnte Audit-Log nicht laden';
    } finally {
      loading = false;
    }
  }

  onMount(refresh);

  const actionStyles: Record<string, { dot: string; label: string }> = {
    'request.create': { dot: 'bg-blue-500', label: 'Request angelegt' },
    'request.view': { dot: 'bg-sky-400', label: 'Kunde hat Link geöffnet' },
    'request.submit': { dot: 'bg-emerald-500', label: 'Kunde eingereicht' },
    'request.retrieve': { dot: 'bg-amber-500', label: 'Geheimnis abgerufen' },
    'request.delete': { dot: 'bg-rose-500', label: 'Request gelöscht' },
    'credential.register': { dot: 'bg-slate-700', label: 'Key registriert' },
    'credential.delete': { dot: 'bg-rose-500', label: 'Key entfernt' },
    'session.unlock': { dot: 'bg-violet-500', label: 'Session entsperrt' }
  };

  function actionMeta(a: string) {
    return actionStyles[a] ?? { dot: 'bg-slate-400', label: a };
  }

  const filtered = $derived(
    filter === 'all' ? entries : entries.filter((e) => e.action === filter)
  );

  const distinctActions = $derived.by(() => {
    const set = new Set<string>();
    for (const e of entries) set.add(e.action);
    return Array.from(set).sort();
  });
</script>

<svelte:head><title>GoGrab — Audit</title></svelte:head>

<div class="mx-auto max-w-4xl px-6 py-8">
  <header class="mb-6 flex items-center justify-between">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight text-slate-900">Audit-Log</h1>
      <p class="mt-1 text-sm text-slate-600">
        Append-only Spur aller security-relevanten Aktionen. Die letzten {entries.length} Einträge.
      </p>
    </div>
    <button
      type="button"
      onclick={refresh}
      class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs text-slate-500 hover:bg-slate-100 hover:text-slate-900"
    >
      <Icon name="refresh-cw" size={12} />
      <span>Aktualisieren</span>
    </button>
  </header>

  {#if distinctActions.length > 0}
    <div class="mb-4 flex flex-wrap items-center gap-2">
      <button
        type="button"
        onclick={() => (filter = 'all')}
        class="rounded-full px-3 py-1 text-xs font-medium {filter === 'all'
          ? 'bg-slate-900 text-white'
          : 'bg-slate-100 text-slate-700 hover:bg-slate-200'}"
      >
        Alle
      </button>
      {#each distinctActions as a (a)}
        <button
          type="button"
          onclick={() => (filter = a)}
          class="rounded-full px-3 py-1 text-xs font-medium {filter === a
            ? 'bg-slate-900 text-white'
            : 'bg-slate-100 text-slate-700 hover:bg-slate-200'}"
        >
          {actionMeta(a).label}
        </button>
      {/each}
    </div>
  {/if}

  {#if loading}
    <div class="rounded-lg border border-slate-200 bg-white p-6 text-sm text-slate-500">Lade …</div>
  {:else if error}
    <div class="rounded-lg border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800">{error}</div>
  {:else if filtered.length === 0}
    <div class="rounded-lg border border-dashed border-slate-300 bg-white py-12 text-center text-sm text-slate-500">
      Keine Einträge.
    </div>
  {:else}
    <div class="overflow-hidden rounded-lg border border-slate-200 bg-white">
      <ul class="divide-y divide-slate-100 text-sm">
        {#each filtered as e (e.id)}
          <li class="flex items-start gap-3 px-4 py-3">
            <span class="mt-1 inline-block h-2 w-2 shrink-0 rounded-full {actionMeta(e.action).dot}"></span>
            <div class="min-w-0 flex-1">
              <div class="flex items-center justify-between gap-3">
                <span class="font-medium text-slate-900">{actionMeta(e.action).label}</span>
                <span class="text-xs text-slate-500" title={absoluteTime(e.occurred_at)}>
                  {relativeTime(e.occurred_at)}
                </span>
              </div>
              <div class="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-slate-500">
                <span>{e.actor}</span>
                {#if e.ip}
                  <span>·</span>
                  <span class="font-mono">{e.ip}</span>
                {/if}
                {#if e.request_id}
                  <span>·</span>
                  <a href="/admin/{e.request_id}" class="font-mono hover:underline">{e.request_id.slice(0, 8)}…</a>
                {/if}
                {#if e.user_agent}
                  <span>·</span>
                  <span class="truncate" title={e.user_agent}>{e.user_agent.slice(0, 40)}{e.user_agent.length > 40 ? '…' : ''}</span>
                {/if}
              </div>
            </div>
          </li>
        {/each}
      </ul>
    </div>
  {/if}
</div>
