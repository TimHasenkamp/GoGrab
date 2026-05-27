<script lang="ts">
  import { goto } from '$app/navigation';
  import { authApi, type ApiError } from '$lib/api';
  import { session } from '$lib/session.svelte';
  import { register, b64urlToBytes } from '$lib/webauthn';

  let label = $state('YubiKey Primary');
  let busy = $state(false);
  let error = $state<string | null>(null);
  let done = $state(false);

  async function registerFirstKey(e: Event) {
    e.preventDefault();
    if (busy) return;
    busy = true;
    error = null;
    try {
      const begin = await authApi.registerBegin();
      const salt = b64urlToBytes(begin.prf_salt_b64);

      // Run WebAuthn ceremony in the browser; get PRF output.
      const { response, prfOutput } = await register(salt, begin.options);

      // Generate Master-KEK, wrap with PRF-derived key.
      const { wrappedMasterB64, wrapIvB64 } = await session.createMasterAndWrap(prfOutput);

      // Persist the credential + wrapped master.
      await authApi.registerFinish({
        credential_response: response,
        session_token: begin.session_token,
        label: label.trim() || 'Security Key',
        wrapped_master_b64: wrappedMasterB64,
        wrap_iv_b64: wrapIvB64
      });

      session.hasCredentials = true;
      done = true;
    } catch (e) {
      error = (e as Error).message || (e as ApiError).message || 'Registrierung fehlgeschlagen';
      // Undo any in-memory state from a partial flow.
      session.lock();
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head><title>GoGrab — Setup</title></svelte:head>

<div class="mx-auto max-w-xl px-6 py-10">
  {#if done}
    <div class="rounded-xl border border-success/30 bg-card shadow-sm">
      <div class="border-b border-success/30 bg-success/10 px-6 py-4">
        <h1 class="text-lg font-semibold text-success">Setup abgeschlossen</h1>
        <p class="mt-1 text-sm text-success">
          Dein YubiKey ist registriert. Wir empfehlen jetzt **dringend**, einen zweiten Key als Backup zu hinterlegen.
        </p>
      </div>
      <div class="space-y-3 p-6">
        <p class="text-sm text-foreground">
          Ohne Backup gilt: <strong>YubiKey verloren = alle künftigen Secrets nicht mehr abrufbar.</strong>
          Du kannst Backup-Keys jetzt direkt im Security-Tab anlegen.
        </p>
        <div class="flex gap-2">
          <a
            href="/admin/security"
            class="rounded-md bg-accent px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-accent-hover"
            >Backup-Key hinzufügen</a
          >
          <button
            type="button"
            onclick={() => goto('/admin', { replaceState: true })}
            class="rounded-md border border-border-strong px-4 py-2 text-sm text-foreground hover:bg-background"
          >
            Später, zum Dashboard
          </button>
        </div>
      </div>
    </div>
  {:else}
    <div class="rounded-xl border border-border bg-card shadow-sm">
      <div class="border-b border-border px-6 py-4">
        <h1 class="text-lg font-semibold text-foreground">Erstes Mal hier</h1>
        <p class="mt-1 text-sm text-muted-foreground">
          Registriere deinen Authenticator. Wir empfehlen einen YubiKey 5 mit Firmware ≥ 5.7 (PRF-Extension).
          Danach kannst du Sessions per Tap entsperren — kein Link mehr aufheben.
        </p>
      </div>

      <form onsubmit={registerFirstKey} class="space-y-4 p-6">
        <div>
          <label for="label" class="block text-sm font-medium text-foreground">
            Bezeichnung des Keys
            <span class="font-normal text-muted-foreground">— nur für dich, z.B. „YubiKey Schreibtisch"</span>
          </label>
          <input
            id="label"
            required
            maxlength="64"
            bind:value={label}
            class="mt-1 block w-full rounded-md border border-border-strong px-3 py-2 text-sm shadow-sm focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
          />
        </div>

        <div class="rounded-md border border-border bg-background p-3 text-sm text-foreground">
          <p class="font-medium text-foreground">Was beim Klick passiert</p>
          <ol class="mt-1 list-decimal space-y-0.5 pl-4">
            <li>Browser fordert deinen Authenticator an — bei YubiKey: einstecken & antippen.</li>
            <li>Browser leitet aus dem Schlüssel deterministisch einen Wrap-Key ab (PRF).</li>
            <li>
              Lokal wird ein zufälliger Master-Schlüssel erzeugt und mit dem Wrap-Key chiffriert auf dem Server abgelegt.
            </li>
            <li>Der Master-Schlüssel selbst verlässt nie deinen Browser.</li>
          </ol>
        </div>

        {#if error}
          <div class="rounded-md border border-danger/30 bg-danger/10 px-3 py-2 text-sm text-danger">{error}</div>
        {/if}

        <button
          type="submit"
          disabled={busy}
          class="inline-flex items-center gap-1.5 rounded-md bg-accent px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-accent-hover disabled:opacity-50"
        >
          {busy ? 'Warte auf Authenticator …' : 'YubiKey registrieren'}
        </button>
      </form>
    </div>
  {/if}
</div>
