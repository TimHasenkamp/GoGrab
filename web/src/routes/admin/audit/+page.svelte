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
    'request.create': { dot: 'bg-accent/70', label: 'Request angelegt' },
    'request.view': { dot: 'bg-accent/50', label: 'Kunde hat Link geöffnet' },
    'request.submit': { dot: 'bg-success', label: 'Kunde eingereicht' },
    'request.retrieve': { dot: 'bg-warning', label: 'Geheimnis abgerufen' },
    'request.delete': { dot: 'bg-danger', label: 'Request gelöscht' },
    'credential.register': { dot: 'bg-muted-foreground', label: 'Key registriert' },
    'credential.delete': { dot: 'bg-danger', label: 'Key entfernt' },
    'session.unlock': { dot: 'bg-accent/80', label: 'Session entsperrt' }
  };

  function actionMeta(a: string) {
    return actionStyles[a] ?? { dot: 'bg-muted-foreground', label: a };
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
      <h1 class="text-2xl font-semibold tracking-tight text-foreground">Audit-Log</h1>
      <p class="mt-1 text-sm text-muted-foreground">
        Append-only Spur aller security-relevanten Aktionen. Die letzten {entries.length} Einträge.
      </p>
    </div>
    <button
      type="button"
      onclick={refresh}
      class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
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
          ? 'bg-accent text-background'
          : 'bg-muted text-foreground hover:bg-muted'}"
      >
        Alle
      </button>
      {#each distinctActions as a (a)}
        <button
          type="button"
          onclick={() => (filter = a)}
          class="rounded-full px-3 py-1 text-xs font-medium {filter === a
            ? 'bg-accent text-background'
            : 'bg-muted text-foreground hover:bg-muted'}"
        >
          {actionMeta(a).label}
        </button>
      {/each}
    </div>
  {/if}

  {#if loading}
    <div class="rounded-lg border border-border bg-card p-6 text-sm text-muted-foreground">Lade …</div>
  {:else if error}
    <div class="rounded-lg border border-danger/30 bg-danger/10 p-4 text-sm text-danger">{error}</div>
  {:else if filtered.length === 0}
    <div class="rounded-lg border border-dashed border-border-strong bg-card py-12 text-center text-sm text-muted-foreground">
      Keine Einträge.
    </div>
  {:else}
    <div class="overflow-hidden rounded-lg border border-border bg-card">
      <ul class="divide-y divide-border text-sm">
        {#each filtered as e (e.id)}
          <li class="flex items-start gap-3 px-4 py-3">
            <span class="mt-1 inline-block h-2 w-2 shrink-0 rounded-full {actionMeta(e.action).dot}"></span>
            <div class="min-w-0 flex-1">
              <div class="flex items-center justify-between gap-3">
                <span class="font-medium text-foreground">{actionMeta(e.action).label}</span>
                <span class="text-xs text-muted-foreground" title={absoluteTime(e.occurred_at)}>
                  {relativeTime(e.occurred_at)}
                </span>
              </div>
              <div class="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
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
