<script lang="ts">
  import { confirmStore } from './confirm.svelte';

  function onKey(e: KeyboardEvent) {
    if (!confirmStore.current) return;
    if (e.key === 'Escape') confirmStore.resolve(false);
    if (e.key === 'Enter') confirmStore.resolve(true);
  }
</script>

<svelte:window onkeydown={onKey} />

{#if confirmStore.current}
  {@const c = confirmStore.current}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-accent/55 p-4"
    role="dialog"
    aria-modal="true"
    aria-labelledby="confirm-title"
  >
    <div class="w-full max-w-sm rounded-xl bg-card p-5 shadow-2xl">
      <h2 id="confirm-title" class="text-base font-semibold text-foreground">{c.title}</h2>
      <p class="mt-1.5 text-sm text-muted-foreground">{c.body}</p>
      <div class="mt-5 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
        <button
          type="button"
          onclick={() => confirmStore.resolve(false)}
          class="rounded-md border border-border-strong bg-card px-3 py-1.5 text-sm font-medium text-foreground hover:bg-background"
        >
          {c.cancelLabel}
        </button>
        <button
          type="button"
          onclick={() => confirmStore.resolve(true)}
          class="rounded-md px-3 py-1.5 text-sm font-medium text-background shadow-sm {c.destructive
            ? 'bg-danger hover:bg-danger/80'
            : 'bg-accent hover:bg-accent-hover'}"
        >
          {c.confirmLabel}
        </button>
      </div>
    </div>
  </div>
{/if}
