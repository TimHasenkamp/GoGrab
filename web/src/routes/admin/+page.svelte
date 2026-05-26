<script lang="ts">
  import { onMount } from 'svelte';
  import { adminApi, type AdminRequestSummary, type ApiError } from '$lib/api';

  let requests = $state<AdminRequestSummary[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  async function refresh() {
    loading = true;
    error = null;
    try {
      requests = await adminApi.list();
    } catch (e) {
      error = (e as ApiError).message || 'failed to load';
    } finally {
      loading = false;
    }
  }

  onMount(refresh);

  function statusColor(s: AdminRequestSummary['status']) {
    return {
      pending: 'bg-amber-100 text-amber-800',
      submitted: 'bg-emerald-100 text-emerald-800',
      retrieved: 'bg-slate-200 text-slate-700',
      expired: 'bg-rose-100 text-rose-800'
    }[s];
  }
</script>

<svelte:head><title>GoGrab — Requests</title></svelte:head>

<main class="mx-auto max-w-4xl p-6">
  <header class="mb-6 flex items-center justify-between">
    <h1 class="text-2xl font-semibold text-slate-900">Secret requests</h1>
    <a
      href="/admin/new"
      class="rounded bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-700"
    >
      + New request
    </a>
  </header>

  {#if loading}
    <p class="text-slate-500">Loading…</p>
  {:else if error}
    <p class="text-rose-600">Error: {error}</p>
  {:else if requests.length === 0}
    <p class="text-slate-500">No requests yet. Create one to get started.</p>
  {:else}
    <ul class="divide-y divide-slate-200 rounded border border-slate-200 bg-white">
      {#each requests as r (r.id)}
        <li class="flex items-center justify-between gap-4 p-4">
          <div class="min-w-0 flex-1">
            <a
              href="/admin/{r.id}"
              class="block truncate text-sm font-medium text-slate-900 hover:underline"
            >
              {r.description}
            </a>
            <p class="mt-1 text-xs text-slate-500">
              Expires {new Date(r.expires_at).toLocaleString()}
            </p>
          </div>
          <span
            class="rounded-full px-2 py-0.5 text-xs font-medium {statusColor(r.status)}"
          >
            {r.status}
          </span>
        </li>
      {/each}
    </ul>
  {/if}
</main>
