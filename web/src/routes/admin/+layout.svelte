<script lang="ts">
  import '../../app.css';
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { authApi, type ApiError } from '$lib/api';
  import { session } from '$lib/session.svelte';
  import { authenticate } from '$lib/webauthn';
  import Toaster from '$lib/Toaster.svelte';
  import ConfirmDialog from '$lib/ConfirmDialog.svelte';

  let { children } = $props();

  let booting = $state(true);
  let bootError = $state<string | null>(null);
  let unlocking = $state(false);
  let unlockError = $state<string | null>(null);

  const path = $derived($page.url.pathname);
  const isSetup = $derived(path.startsWith('/admin/setup'));
  const isList = $derived(path === '/admin');

  onMount(async () => {
    try {
      const status = await authApi.status();
      session.username = status.username;
      session.hasCredentials = status.has_credentials;
      session.prfSaltB64 = status.prf_salt_b64;

      if (!status.has_credentials && !isSetup) {
        await goto('/admin/setup', { replaceState: true });
      }
    } catch (e) {
      bootError = (e as ApiError).message || 'Status konnte nicht geladen werden';
    } finally {
      booting = false;
    }
  });

  async function unlock() {
    if (unlocking || !session.prfSalt) return;
    unlocking = true;
    unlockError = null;
    try {
      const begin = await authApi.loginBegin();
      const salt = session.prfSalt;
      if (!salt) throw new Error('Kein PRF-Salt');
      const { response, prfOutput } = await authenticate(salt, begin.options);
      const finish = await authApi.loginFinish({
        credential_response: response,
        session_token: begin.session_token
      });
      await session.unlock(
        prfOutput,
        finish.wrapped_master_b64,
        finish.wrap_iv_b64,
        finish.credential_id_b64
      );
    } catch (e) {
      unlockError = (e as Error).message || (e as ApiError).message || 'Unlock fehlgeschlagen';
    } finally {
      unlocking = false;
    }
  }

  function lock() {
    session.lock();
  }
</script>

<div class="min-h-screen bg-slate-50 text-slate-900">
  <header class="sticky top-0 z-10 border-b border-slate-200 bg-white/80 backdrop-blur">
    <div class="mx-auto flex max-w-5xl items-center justify-between px-6 py-3">
      <a href="/admin" class="flex items-center gap-2 text-slate-900">
        <span class="grid h-8 w-8 place-items-center rounded-lg bg-slate-900 text-white">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <rect x="3" y="11" width="18" height="11" rx="2" />
            <path d="M7 11V7a5 5 0 0 1 10 0v4" />
          </svg>
        </span>
        <div class="leading-tight">
          <div class="text-sm font-semibold">GoGrab</div>
          <div class="text-[11px] text-slate-500">Secret requests</div>
        </div>
      </a>

      <nav class="flex items-center gap-2">
        {#if session.isUnlocked}
          <a
            href="/admin"
            class="rounded-md px-3 py-1.5 text-sm font-medium {isList
              ? 'bg-slate-100 text-slate-900'
              : 'text-slate-600 hover:text-slate-900'}"
          >
            Requests
          </a>
          <a
            href="/admin/security"
            class="rounded-md px-3 py-1.5 text-sm font-medium text-slate-600 hover:text-slate-900"
          >
            Security
          </a>
          <a
            href="/admin/audit"
            class="rounded-md px-3 py-1.5 text-sm font-medium text-slate-600 hover:text-slate-900"
          >
            Audit
          </a>
          <a
            href="/admin/new"
            class="inline-flex items-center gap-1.5 rounded-md bg-slate-900 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-slate-800"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
            Neuer Request
          </a>
          <button
            type="button"
            onclick={lock}
            class="ml-2 rounded-md border border-slate-300 px-2 py-1 text-xs text-slate-600 hover:bg-slate-100"
            title="Session sperren"
          >
            Lock
          </button>
        {/if}
      </nav>
    </div>
  </header>

  <main>
    {#if booting}
      <div class="mx-auto max-w-2xl p-8 text-sm text-slate-500">Lade …</div>
    {:else if bootError}
      <div class="mx-auto max-w-2xl p-8">
        <div class="rounded-lg border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800">
          {bootError}
        </div>
      </div>
    {:else if isSetup || (session.hasCredentials && session.isUnlocked)}
      {@render children?.()}
    {:else if session.hasCredentials && !session.isUnlocked}
      <!-- Unlock overlay -->
      <div class="mx-auto max-w-md px-6 py-12">
        <div class="rounded-xl border border-slate-200 bg-white p-6 text-center shadow-sm">
          <div class="mx-auto mb-3 grid h-12 w-12 place-items-center rounded-full bg-slate-100 text-slate-600">
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
              <rect x="3" y="11" width="18" height="11" rx="2" />
              <path d="M7 11V7a5 5 0 0 1 10 0v4" />
            </svg>
          </div>
          <h1 class="text-base font-semibold text-slate-900">Session entsperren</h1>
          <p class="mt-1 text-sm text-slate-600">
            Tippe deinen YubiKey an, um den Master-Schlüssel zu laden. Bleibt bis du diesen Tab schließt.
          </p>
          {#if unlockError}
            <p class="mt-3 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800">
              {unlockError}
            </p>
          {/if}
          <button
            type="button"
            onclick={unlock}
            disabled={unlocking}
            class="mt-4 inline-flex items-center gap-1.5 rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-slate-800 disabled:opacity-50"
          >
            {unlocking ? 'Warte auf YubiKey …' : 'Mit YubiKey entsperren'}
          </button>
        </div>
      </div>
    {/if}
  </main>

  <Toaster />
  <ConfirmDialog />
</div>
