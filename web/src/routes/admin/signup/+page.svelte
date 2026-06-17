<script lang="ts">
  import { goto } from '$app/navigation';
  import { authApi, type ApiError } from '$lib/api';
  import { session } from '$lib/session.svelte';
  import { register, authenticate, b64urlToBytes } from '$lib/webauthn';
  import { toast } from '$lib/toast.svelte';
  import Icon from '$lib/Icon.svelte';
  import { theme } from '$lib/theme.svelte';

  let username = $state('');
  let email = $state('');
  let label = $state('Primary');
  let busy = $state(false);
  let error = $state<string | null>(null);

  // username rules mirror the server-side regex so we can show inline feedback.
  const usernameRE = /^[a-z0-9](?:[a-z0-9._-]{1,30}[a-z0-9])?$/;
  const usernameValid = $derived(usernameRE.test(username.trim().toLowerCase()));

  async function submit(e: Event) {
    e.preventDefault();
    const u = username.trim().toLowerCase();
    if (!usernameValid || busy) return;
    busy = true;
    error = null;
    try {
      const begin = await authApi.signupBegin({ username: u, email: email.trim() });
      const salt = b64urlToBytes(begin.prf_salt_b64);
      const { response, prfOutput } = await register(salt, begin.options);

      if (prfOutput) {
        // One-shot path: PRF returned during create() — wrap master key immediately.
        const { wrappedMasterB64, wrapIvB64 } = await session.createMasterAndWrap(prfOutput);
        await authApi.signupFinish({
          username: u,
          credential_response: response,
          session_token: begin.session_token,
          label: label.trim() || 'Primary',
          wrapped_master_b64: wrappedMasterB64,
          wrap_iv_b64: wrapIvB64
        });
      } else {
        // Two-shot path: PRF not returned during create() (common on Linux).
        // Save credential first, then get PRF via a second assertion touch.
        const cred = await authApi.signupFinish({
          username: u,
          credential_response: response,
          session_token: begin.session_token,
          label: label.trim() || 'Primary',
          wrapped_master_b64: '',
          wrap_iv_b64: ''
        });
        toast.info('Noch ein Touch nötig, um die Verschlüsselung einzurichten…');
        const loginData = await authApi.loginBegin({ username: u });
        const prfSalt = b64urlToBytes(loginData.prf_salt_b64);
        const { prfOutput: prf } = await authenticate(prfSalt, loginData.options);
        const { wrappedMasterB64, wrapIvB64 } = await session.createMasterAndWrap(prf);
        await authApi.signupSetMaster({
          credential_id: cred.id,
          wrapped_master_b64: wrappedMasterB64,
          wrap_iv_b64: wrapIvB64
        });
      }

      session.username = u;
      session.hasCredentials = true;
      session.prfSaltB64 = begin.prf_salt_b64;
      toast.success(
        'Konto erstellt. Bitte registriere jetzt einen Backup-Authenticator – sonst sperrst du dich aus, falls dieser verloren geht.'
      );
      await goto('/admin/security', { replaceState: true });
    } catch (e) {
      const code = (e as ApiError).error;
      if (code === 'username_taken') {
        error = 'Username ist bereits vergeben.';
      } else if (code === 'signup_disabled') {
        error = 'Registrierung ist auf dieser Instanz deaktiviert.';
      } else {
        error = (e as Error).message || (e as ApiError).message || 'Registrierung fehlgeschlagen';
      }
      session.lock();
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head><title>GoGrab — Registrieren</title></svelte:head>

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
    <h1 class="text-lg font-semibold tracking-tight text-foreground">Account erstellen</h1>
    <p class="mt-1 text-sm text-muted-foreground">
      Username wählen, Authenticator registrieren — fertig. Kein Passwort.
    </p>

    <form onsubmit={submit} class="mt-5 space-y-4">
      <div>
        <label for="username" class="block text-sm font-medium text-foreground">
          Username
          <span class="font-normal text-muted-foreground">— wird klein geschrieben</span>
        </label>
        <input
          id="username"
          required
          autocomplete="username"
          autocapitalize="none"
          spellcheck="false"
          maxlength="32"
          bind:value={username}
          class="mt-1 block w-full rounded-md border border-border-strong bg-background px-3 py-2 text-sm shadow-sm focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
        />
        {#if username && !usernameValid}
          <p class="mt-1 text-xs text-danger">
            2–32 Zeichen, nur Kleinbuchstaben/Zahlen/<code>. _ -</code>, muss mit Buchstabe oder Zahl beginnen+enden.
          </p>
        {/if}
      </div>

      <div>
        <label for="email" class="block text-sm font-medium text-foreground">
          E-Mail <span class="font-normal text-muted-foreground">(optional)</span>
        </label>
        <input
          id="email"
          type="email"
          autocomplete="email"
          maxlength="254"
          bind:value={email}
          class="mt-1 block w-full rounded-md border border-border-strong bg-background px-3 py-2 text-sm shadow-sm focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
        />
      </div>

      <div>
        <label for="label" class="block text-sm font-medium text-foreground">
          Name dieses Authenticators
          <span class="font-normal text-muted-foreground">— z.B. „YubiKey Schreibtisch"</span>
        </label>
        <input
          id="label"
          required
          maxlength="64"
          bind:value={label}
          class="mt-1 block w-full rounded-md border border-border-strong bg-background px-3 py-2 text-sm shadow-sm focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
        />
      </div>

      <div class="rounded-md border border-border bg-background/40 p-3 text-xs text-muted-foreground">
        <p class="font-medium text-foreground">Wichtig:</p>
        Direkt nach der Registrierung solltest du einen <strong>zweiten Authenticator</strong> als Backup
        hinzufügen. Geht dein einziger verloren, sind Account und alle gespeicherten Secrets unwiederbringlich weg —
        es gibt kein Reset.
      </div>

      {#if error}
        <div class="rounded-md border border-danger/30 bg-danger/10 px-3 py-2 text-sm text-danger">
          {error}
        </div>
      {/if}

      <button
        type="submit"
        disabled={busy || !usernameValid}
        class="inline-flex w-full items-center justify-center gap-1.5 rounded-md bg-accent px-4 py-2 text-sm font-medium text-background shadow-sm transition hover:bg-accent-hover hover:shadow-accent-glow disabled:cursor-not-allowed disabled:opacity-50"
      >
        {busy ? 'Warte auf Authenticator …' : 'Registrieren'}
      </button>
    </form>

    <div class="mt-6 border-t border-border pt-4 text-center text-sm text-muted-foreground">
      Schon ein Konto?
      <a href="/admin/login" class="font-medium text-accent hover:underline">Anmelden</a>
    </div>
  </div>
</div>
