<script lang="ts">
  import { goto } from '$app/navigation';
  import { authApi, type ApiError } from '$lib/api';
  import { session } from '$lib/session.svelte';
  import { authenticate, b64urlToBytes } from '$lib/webauthn';
  import Icon from '$lib/Icon.svelte';
  import { theme } from '$lib/theme.svelte';

  let username = $state('');
  let busy = $state(false);
  let error = $state<string | null>(null);

  async function submit(e: Event) {
    e.preventDefault();
    const u = username.trim().toLowerCase();
    if (!u || busy) return;
    busy = true;
    error = null;
    try {
      const begin = await authApi.loginBegin({ username: u });
      const salt = b64urlToBytes(begin.prf_salt_b64);
      const { response, prfOutput } = await authenticate(salt, begin.options);
      const finish = await authApi.loginFinish({
        username: u,
        credential_response: response,
        session_token: begin.session_token
      });
      await session.unlock(
        prfOutput,
        finish.wrapped_master_b64,
        finish.wrap_iv_b64,
        finish.credential_id_b64
      );
      session.username = u;
      session.hasCredentials = true;
      session.prfSaltB64 = begin.prf_salt_b64;
      await goto('/admin', { replaceState: true });
    } catch (e) {
      const m = (e as Error).message || (e as ApiError).message || 'Login fehlgeschlagen';
      error = m;
      session.lock();
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head><title>GoGrab — Login</title></svelte:head>

<div class="mx-auto flex min-h-screen max-w-md flex-col px-6 py-12">
  <div class="mb-8 flex items-center justify-between">
    <a href="/admin/login" class="flex items-center gap-2.5 text-foreground">
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
    <button
      type="button"
      onclick={() => theme.toggle()}
      class="inline-flex items-center justify-center rounded-md p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground"
      aria-label="Theme wechseln"
    >
      <Icon name={theme.current === 'dark' ? 'sun' : 'moon'} size={16} />
    </button>
  </div>

  <div class="rounded-xl border border-border bg-card p-6 shadow-sm">
    <h1 class="text-lg font-semibold tracking-tight text-foreground">Anmelden</h1>
    <p class="mt-1 text-sm text-muted-foreground">
      Username eingeben, dann Authenticator antippen.
    </p>

    <form onsubmit={submit} class="mt-5 space-y-4">
      <div>
        <label for="username" class="block text-sm font-medium text-foreground">Username</label>
        <input
          id="username"
          required
          autocomplete="username webauthn"
          autocapitalize="none"
          spellcheck="false"
          maxlength="32"
          bind:value={username}
          class="mt-1 block w-full rounded-md border border-border-strong bg-background px-3 py-2 text-sm shadow-sm focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
        />
      </div>

      {#if error}
        <div class="rounded-md border border-danger/30 bg-danger/10 px-3 py-2 text-sm text-danger">
          {error}
        </div>
      {/if}

      <button
        type="submit"
        disabled={busy || !username.trim()}
        class="inline-flex w-full items-center justify-center gap-1.5 rounded-md bg-accent px-4 py-2 text-sm font-medium text-background shadow-sm transition hover:bg-accent-hover hover:shadow-accent-glow disabled:cursor-not-allowed disabled:opacity-50"
      >
        {busy ? 'Warte auf Authenticator …' : 'Anmelden'}
      </button>
    </form>

    <div class="mt-6 border-t border-border pt-4 text-center text-sm text-muted-foreground">
      Noch kein Konto?
      <a href="/admin/signup" class="font-medium text-accent hover:underline">Registrieren</a>
    </div>
  </div>
</div>
