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
  import Icon from '$lib/Icon.svelte';

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

<div class="min-h-screen bg-background text-foreground">
  <header class="sticky top-0 z-10 border-b border-border bg-background/80 backdrop-blur">
    <div class="mx-auto flex max-w-5xl items-center justify-between px-6 py-3">
      <a href="/admin" class="flex items-center gap-2.5 text-foreground">
        <span class="grid h-8 w-8 place-items-center rounded-lg bg-accent text-background shadow-accent-glow">
          <Icon name="lock" size={16} strokeWidth={2.5} />
        </span>
        <div class="leading-tight">
          <div class="text-sm font-semibold tracking-tight">GoGrab</div>
          <div class="text-[11px] text-muted-foreground">
            Secret requests<span class="text-accent">.</span>
          </div>
        </div>
      </a>

      <nav class="flex items-center gap-1">
        {#if session.isUnlocked}
          <a
            href="/admin"
            class="rounded-md px-3 py-1.5 text-sm font-medium {isList
              ? 'bg-muted text-foreground'
              : 'text-muted-foreground hover:text-foreground'}"
          >
            Requests
          </a>
          <a
            href="/admin/security"
            class="rounded-md px-3 py-1.5 text-sm font-medium text-muted-foreground hover:text-foreground"
          >
            Security
          </a>
          <a
            href="/admin/audit"
            class="rounded-md px-3 py-1.5 text-sm font-medium text-muted-foreground hover:text-foreground"
          >
            Audit
          </a>
          <a
            href="/admin/new"
            class="ml-2 inline-flex items-center gap-1.5 rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-background transition hover:bg-accent-hover hover:shadow-accent-glow"
          >
            <Icon name="plus" size={14} strokeWidth={2.5} />
            Neuer Request
          </a>
          <button
            type="button"
            onclick={lock}
            class="ml-2 inline-flex items-center gap-1.5 rounded-md border border-border px-2 py-1 text-xs text-muted-foreground hover:border-border-strong hover:text-foreground"
            title="Session sperren"
          >
            <Icon name="log-out" size={12} />
            Lock
          </button>
        {/if}
      </nav>
    </div>
  </header>

  <main>
    {#if booting}
      <div class="mx-auto max-w-2xl p-8 text-sm text-muted-foreground">Lade …</div>
    {:else if bootError}
      <div class="mx-auto max-w-2xl p-8">
        <div class="rounded-lg border border-danger/30 bg-danger/10 p-4 text-sm text-danger">
          {bootError}
        </div>
      </div>
    {:else if isSetup || (session.hasCredentials && session.isUnlocked)}
      {@render children?.()}
    {:else if session.hasCredentials && !session.isUnlocked}
      <div class="mx-auto max-w-md px-6 py-12">
        <div class="rounded-xl border border-border bg-card p-6 text-center">
          <div
            class="mx-auto mb-3 grid h-12 w-12 place-items-center rounded-full bg-accent/10 text-accent"
          >
            <Icon name="lock" size={22} strokeWidth={1.8} />
          </div>
          <h1 class="text-base font-semibold tracking-tight text-foreground">Session entsperren</h1>
          <p class="mt-1 text-sm text-muted-foreground">
            Tippe deinen YubiKey an, um den Master-Schlüssel zu laden. Bleibt bis du diesen Tab
            schließt.
          </p>
          {#if unlockError}
            <p class="mt-3 rounded-md border border-danger/30 bg-danger/10 px-3 py-2 text-sm text-danger">
              {unlockError}
            </p>
          {/if}
          <button
            type="button"
            onclick={unlock}
            disabled={unlocking}
            class="mt-4 inline-flex items-center gap-1.5 rounded-md bg-accent px-4 py-2 text-sm font-medium text-background transition hover:bg-accent-hover hover:shadow-accent-glow disabled:opacity-50"
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
