<script lang="ts">
  import { onMount } from 'svelte';
  import { authApi, type ApiError, type CredentialSummary } from '$lib/api';
  import { session } from '$lib/session.svelte';
  import { register, authenticate, b64urlToBytes } from '$lib/webauthn';
  import { relativeTime, absoluteTime } from '$lib/format';
  import { toast } from '$lib/toast.svelte';
  import { confirmStore } from '$lib/confirm.svelte';

  let credentials = $state<CredentialSummary[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let label = $state('YubiKey Backup');
  let adding = $state(false);
  let addError = $state<string | null>(null);

  async function refresh() {
    loading = true;
    error = null;
    try {
      credentials = await authApi.listCredentials();
    } catch (e) {
      error = (e as ApiError).message || 'Liste konnte nicht geladen werden';
    } finally {
      loading = false;
    }
  }

  onMount(refresh);

  async function addBackup(e: Event) {
    e.preventDefault();
    if (adding) return;
    if (!session.isUnlocked) {
      addError = 'Session muss entsperrt sein, um einen Backup-Key hinzuzufügen.';
      return;
    }
    adding = true;
    addError = null;
    try {
      const begin = await authApi.registerBegin();
      const salt = b64urlToBytes(begin.prf_salt_b64);
      const { response, prfOutput } = await register(salt, begin.options);

      if (prfOutput) {
        // One-shot: PRF available during create()
        const { wrappedMasterB64, wrapIvB64 } = await session.wrapExistingMaster(prfOutput);
        await authApi.registerFinish({
          credential_response: response,
          session_token: begin.session_token,
          label: label.trim() || 'YubiKey Backup',
          wrapped_master_b64: wrappedMasterB64,
          wrap_iv_b64: wrapIvB64
        });
      } else {
        // Two-shot: PRF not returned during create() — save credential first,
        // then collect PRF via a second assertion.
        const cred = await authApi.registerFinish({
          credential_response: response,
          session_token: begin.session_token,
          label: label.trim() || 'YubiKey Backup',
          wrapped_master_b64: '',
          wrap_iv_b64: ''
        });
        toast.info('Noch ein Touch nötig, um die Verschlüsselung einzurichten…');
        const loginData = await authApi.loginBegin({ username: session.username! });
        const prfSalt = b64urlToBytes(loginData.prf_salt_b64);
        const { prfOutput: prf } = await authenticate(prfSalt, loginData.options);
        const { wrappedMasterB64, wrapIvB64 } = await session.wrapExistingMaster(prf);
        await authApi.signupSetMaster({
          credential_id: cred.id,
          wrapped_master_b64: wrappedMasterB64,
          wrap_iv_b64: wrapIvB64
        });
      }

      label = 'YubiKey Backup';
      await refresh();
    } catch (e) {
      addError = (e as Error).message || (e as ApiError).message || 'Hinzufügen fehlgeschlagen';
    } finally {
      adding = false;
    }
  }

  async function revoke(c: CredentialSummary) {
    if (credentials.length <= 1) {
      toast.warning('Du kannst den letzten Key nicht löschen — registriere zuerst einen Backup-Key.');
      return;
    }
    const ok = await confirmStore.ask({
      title: 'Key entfernen?',
      body: `„${c.label}" kann danach keine Sessions mehr entsperren. Stelle sicher, dass du einen anderen Authenticator zur Hand hast.`,
      confirmLabel: 'Entfernen',
      destructive: true
    });
    if (!ok) return;
    try {
      await authApi.deleteCredential(c.id);
      toast.success(`„${c.label}" entfernt.`);
      await refresh();
    } catch (e) {
      toast.error('Löschen fehlgeschlagen: ' + ((e as ApiError).message || ''));
    }
  }

</script>

<svelte:head><title>GoGrab — Security</title></svelte:head>

<div class="mx-auto max-w-3xl px-6 py-8">
  <header class="mb-6">
    <h1 class="text-2xl font-semibold tracking-tight text-foreground">Security</h1>
    <p class="mt-1 text-sm text-muted-foreground">
      Verwalte deine WebAuthn-Authenticators. Wir empfehlen mindestens zwei (Primary + Backup im Safe).
    </p>
  </header>

  <!-- Backup-warning -->
  {#if credentials.length === 1}
    <div class="mb-6 flex gap-3 rounded-md border border-warning/30 bg-warning/10 p-4">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="mt-0.5 shrink-0 text-warning">
        <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
        <line x1="12" y1="9" x2="12" y2="13" />
        <line x1="12" y1="17" x2="12.01" y2="17" />
      </svg>
      <div class="text-sm text-warning">
        <p class="font-medium">Kein Backup-Key registriert</p>
        <p class="mt-1 text-warning">
          Wenn dein einziger Key verloren oder defekt geht, kannst du keine Sessions mehr entsperren und nichts mehr abrufen.
          Registriere jetzt einen zweiten Key und hinterlege ihn im Safe.
        </p>
      </div>
    </div>
  {/if}

  <!-- Existing credentials -->
  <div class="mb-6 rounded-xl border border-border bg-card shadow-sm">
    <div class="border-b border-border px-6 py-3 text-sm font-medium text-foreground">
      Registrierte Keys ({credentials.length})
    </div>
    {#if loading}
      <div class="p-6 text-sm text-muted-foreground">Lade …</div>
    {:else if error}
      <div class="p-6 text-sm text-danger">{error}</div>
    {:else if credentials.length === 0}
      <div class="p-6 text-sm text-muted-foreground">Keine Keys registriert.</div>
    {:else}
      <ul class="divide-y divide-border">
        {#each credentials as c (c.id)}
          <li class="flex items-center justify-between gap-3 px-6 py-3">
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-muted-foreground">
                  <rect x="3" y="11" width="18" height="11" rx="2" />
                  <path d="M7 11V7a5 5 0 0 1 10 0v4" />
                </svg>
                <span class="text-sm font-medium text-foreground">{c.label}</span>
                {#if c.transports.length > 0}
                  <span class="text-xs text-muted-foreground">{c.transports.join(', ')}</span>
                {/if}
              </div>
              <p class="mt-0.5 text-xs text-muted-foreground">
                Registriert <span title={absoluteTime(c.created_at)}>{relativeTime(c.created_at)}</span>
                {#if c.last_used_at}
                  · Zuletzt benutzt <span title={absoluteTime(c.last_used_at)}>{relativeTime(c.last_used_at)}</span>
                {:else}
                  · Noch nicht benutzt
                {/if}
              </p>
            </div>
            <button
              type="button"
              onclick={() => revoke(c)}
              disabled={credentials.length <= 1}
              class="rounded-md border border-danger/30 px-2 py-1 text-xs text-danger hover:bg-danger/10 disabled:cursor-not-allowed disabled:opacity-40"
              title={credentials.length <= 1 ? 'Letzter Key — nicht entfernbar' : 'Diesen Key entfernen'}
            >
              Entfernen
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  <!-- Add new credential -->
  <div class="rounded-xl border border-border bg-card shadow-sm">
    <div class="border-b border-border px-6 py-3">
      <h2 class="text-sm font-medium text-foreground">Backup-Key hinzufügen</h2>
      <p class="mt-0.5 text-xs text-muted-foreground">
        Tippe den neuen Key an. Dein bestehender Master-Schlüssel wird zusätzlich mit dem PRF dieses Keys
        gewrappt — beide Keys entsperren danach dieselbe Session.
      </p>
    </div>
    <form onsubmit={addBackup} class="flex items-end gap-2 p-4">
      <div class="flex-1">
        <label for="newlabel" class="block text-xs font-medium text-muted-foreground">Bezeichnung</label>
        <input
          id="newlabel"
          required
          maxlength="64"
          bind:value={label}
          class="mt-1 block w-full rounded-md border border-border-strong px-3 py-1.5 text-sm shadow-sm focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
        />
      </div>
      <button
        type="submit"
        disabled={adding || !session.isUnlocked}
        class="rounded-md bg-accent px-4 py-2 text-sm font-medium text-background shadow-sm hover:bg-accent-hover disabled:opacity-50"
      >
        {adding ? 'Warte auf Key …' : 'Registrieren'}
      </button>
    </form>
    {#if addError}
      <p class="mx-4 mb-4 rounded-md border border-danger/30 bg-danger/10 px-3 py-2 text-sm text-danger">{addError}</p>
    {/if}
    {#if !session.isUnlocked}
      <p class="mx-4 mb-4 text-xs text-muted-foreground">
        Session muss entsperrt sein — Lock-Button oben rechts deaktiviert die Session.
      </p>
    {/if}
  </div>
</div>
