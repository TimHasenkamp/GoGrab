<script lang="ts">
  import { onMount } from 'svelte';
  import { authApi, type ApiError, type CredentialSummary } from '$lib/api';
  import { session } from '$lib/session.svelte';
  import { register, b64urlToBytes } from '$lib/webauthn';
  import { relativeTime, absoluteTime } from '$lib/format';

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

      // Re-wrap the existing in-memory master KEK with the new key's PRF output.
      const { wrappedMasterB64, wrapIvB64 } = await session.wrapExistingMaster(prfOutput);

      await authApi.registerFinish({
        credential_response: response,
        session_token: begin.session_token,
        label: label.trim() || 'YubiKey Backup',
        wrapped_master_b64: wrappedMasterB64,
        wrap_iv_b64: wrapIvB64
      });

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
      alert('Du kannst den letzten Key nicht löschen — registriere zuerst einen Backup-Key.');
      return;
    }
    if (!confirm(`Key „${c.label}" wirklich entfernen? Dieser Key kann danach keine Sessions mehr entsperren.`)) return;
    try {
      await authApi.deleteCredential(c.id);
      await refresh();
    } catch (e) {
      alert('Löschen fehlgeschlagen: ' + ((e as ApiError).message || ''));
    }
  }

  const isCurrentSessionKey = $derived.by(() => {
    return (c: CredentialSummary) =>
      session.unlockingCredentialIdB64 !== null &&
      c.id.length > 0 &&
      false; // we only have the credential_id (raw), not the db id from session — skip the marker for now
  });
  void isCurrentSessionKey; // suppress unused
</script>

<svelte:head><title>GoGrab — Security</title></svelte:head>

<div class="mx-auto max-w-3xl px-6 py-8">
  <header class="mb-6">
    <h1 class="text-2xl font-semibold tracking-tight text-slate-900">Security</h1>
    <p class="mt-1 text-sm text-slate-600">
      Verwalte deine WebAuthn-Authenticators. Wir empfehlen mindestens zwei (Primary + Backup im Safe).
    </p>
  </header>

  <!-- Backup-warning -->
  {#if credentials.length === 1}
    <div class="mb-6 flex gap-3 rounded-md border border-amber-200 bg-amber-50 p-4">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="mt-0.5 shrink-0 text-amber-700">
        <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
        <line x1="12" y1="9" x2="12" y2="13" />
        <line x1="12" y1="17" x2="12.01" y2="17" />
      </svg>
      <div class="text-sm text-amber-900">
        <p class="font-medium">Kein Backup-Key registriert</p>
        <p class="mt-1 text-amber-800">
          Wenn dein einziger Key verloren oder defekt geht, kannst du keine Sessions mehr entsperren und nichts mehr abrufen.
          Registriere jetzt einen zweiten Key und hinterlege ihn im Safe.
        </p>
      </div>
    </div>
  {/if}

  <!-- Existing credentials -->
  <div class="mb-6 rounded-xl border border-slate-200 bg-white shadow-sm">
    <div class="border-b border-slate-100 px-6 py-3 text-sm font-medium text-slate-900">
      Registrierte Keys ({credentials.length})
    </div>
    {#if loading}
      <div class="p-6 text-sm text-slate-500">Lade …</div>
    {:else if error}
      <div class="p-6 text-sm text-rose-700">{error}</div>
    {:else if credentials.length === 0}
      <div class="p-6 text-sm text-slate-500">Keine Keys registriert.</div>
    {:else}
      <ul class="divide-y divide-slate-100">
        {#each credentials as c (c.id)}
          <li class="flex items-center justify-between gap-3 px-6 py-3">
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-slate-500">
                  <rect x="3" y="11" width="18" height="11" rx="2" />
                  <path d="M7 11V7a5 5 0 0 1 10 0v4" />
                </svg>
                <span class="text-sm font-medium text-slate-900">{c.label}</span>
                {#if c.transports.length > 0}
                  <span class="text-xs text-slate-500">{c.transports.join(', ')}</span>
                {/if}
              </div>
              <p class="mt-0.5 text-xs text-slate-500">
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
              class="rounded-md border border-rose-200 px-2 py-1 text-xs text-rose-700 hover:bg-rose-50 disabled:cursor-not-allowed disabled:opacity-40"
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
  <div class="rounded-xl border border-slate-200 bg-white shadow-sm">
    <div class="border-b border-slate-200 px-6 py-3">
      <h2 class="text-sm font-medium text-slate-900">Backup-Key hinzufügen</h2>
      <p class="mt-0.5 text-xs text-slate-600">
        Tippe den neuen Key an. Dein bestehender Master-Schlüssel wird zusätzlich mit dem PRF dieses Keys
        gewrappt — beide Keys entsperren danach dieselbe Session.
      </p>
    </div>
    <form onsubmit={addBackup} class="flex items-end gap-2 p-4">
      <div class="flex-1">
        <label for="newlabel" class="block text-xs font-medium text-slate-600">Bezeichnung</label>
        <input
          id="newlabel"
          required
          maxlength="64"
          bind:value={label}
          class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-1.5 text-sm shadow-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500"
        />
      </div>
      <button
        type="submit"
        disabled={adding || !session.isUnlocked}
        class="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-slate-800 disabled:opacity-50"
      >
        {adding ? 'Warte auf Key …' : 'Registrieren'}
      </button>
    </form>
    {#if addError}
      <p class="mx-4 mb-4 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800">{addError}</p>
    {/if}
    {#if !session.isUnlocked}
      <p class="mx-4 mb-4 text-xs text-slate-500">
        Session muss entsperrt sein — Lock-Button oben rechts deaktiviert die Session.
      </p>
    {/if}
  </div>
</div>
