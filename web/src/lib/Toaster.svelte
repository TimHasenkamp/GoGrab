<script lang="ts">
  import { toast, type ToastItem } from './toast.svelte';

  const kindStyle: Record<ToastItem['kind'], string> = {
    success: 'border-success bg-success/10 text-success',
    error: 'border-danger bg-danger/10 text-danger',
    info: 'border-border-strong bg-card text-foreground',
    warning: 'border-warning bg-warning/10 text-warning'
  };
  const kindIcon: Record<ToastItem['kind'], string> = {
    success: 'M20 6 9 17 4 12',
    error: 'M18 6 6 18 M6 6 18 18',
    info: 'M12 16v-4 M12 8h.01 M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20z',
    warning: 'M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z M12 9v4 M12 17h.01'
  };
</script>

<div
  class="pointer-events-none fixed inset-x-0 top-4 z-50 flex flex-col items-center gap-2 px-4 sm:items-end sm:px-6"
  aria-live="polite"
  aria-atomic="false"
>
  {#each toast.items as t (t.id)}
    <div
      role="status"
      class="pointer-events-auto flex w-full max-w-sm items-start gap-2.5 rounded-lg border px-3 py-2.5 shadow-sm {kindStyle[t.kind]}"
    >
      <svg
        width="18"
        height="18"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        class="mt-0.5 shrink-0"
      >
        <path d={kindIcon[t.kind]} />
      </svg>
      <p class="flex-1 text-sm leading-snug">{t.text}</p>
      <button
        type="button"
        onclick={() => toast.dismiss(t.id)}
        class="-mr-1 -mt-1 rounded p-1 text-current/60 hover:bg-black/5"
        aria-label="Schließen"
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
          <line x1="18" y1="6" x2="6" y2="18" />
          <line x1="6" y1="6" x2="18" y2="18" />
        </svg>
      </button>
    </div>
  {/each}
</div>
