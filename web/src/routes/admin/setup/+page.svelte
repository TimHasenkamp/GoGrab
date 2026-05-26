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
    <div class="rounded-xl border border-emerald-200 bg-white shadow-sm">
      <div class="border-b border-emerald-100 bg-emerald-50 px-6 py-4">
        <h1 class="text-lg font-semibold text-emerald-900">Setup abgeschlossen</h1>
        <p class="mt-1 text-sm text-emerald-800">
          Dein YubiKey ist registriert. Wir empfehlen jetzt **dringend**, einen zweiten Key als Backup zu hinterlegen.
        </p>
      </div>
      <div class="space-y-3 p-6">
        <p class="text-sm text-slate-700">
          Ohne Backup gilt: <strong>YubiKey verloren = alle künftigen Secrets nicht mehr abrufbar.</strong>
          Du kannst Backup-Keys jetzt direkt im Security-Tab anlegen.
        </p>
        <div class="flex gap-2">
          <a
            href="/admin/security"
            class="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-slate-800"
            >Backup-Key hinzufügen</a
          >
          <button
            type="button"
            onclick={() => goto('/admin', { replaceState: true })}
            class="rounded-md border border-slate-300 px-4 py-2 text-sm text-slate-700 hover:bg-slate-50"
          >
            Später, zum Dashboard
          </button>
        </div>
      </div>
    </div>
  {:else}
    <div class="rounded-xl border border-slate-200 bg-white shadow-sm">
      <div class="border-b border-slate-200 px-6 py-4">
        <h1 class="text-lg font-semibold text-slate-900">Erstes Mal hier</h1>
        <p class="mt-1 text-sm text-slate-600">
          Registriere deinen Authenticator. Wir empfehlen einen YubiKey 5 mit Firmware ≥ 5.7 (PRF-Extension).
          Danach kannst du Sessions per Tap entsperren — kein Link mehr aufheben.
        </p>
      </div>

      <form onsubmit={registerFirstKey} class="space-y-4 p-6">
        <div>
          <label for="label" class="block text-sm font-medium text-slate-700">
            Bezeichnung des Keys
            <span class="font-normal text-slate-500">— nur für dich, z.B. „YubiKey Schreibtisch"</span>
          </label>
          <input
            id="label"
            required
            maxlength="64"
            bind:value={label}
            class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500"
          />
        </div>

        <div class="rounded-md border border-slate-200 bg-slate-50 p-3 text-sm text-slate-700">
          <p class="font-medium text-slate-900">Was beim Klick passiert</p>
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
          <div class="rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800">{error}</div>
        {/if}

        <button
          type="submit"
          disabled={busy}
          class="inline-flex items-center gap-1.5 rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-slate-800 disabled:opacity-50"
        >
          {busy ? 'Warte auf Authenticator …' : 'YubiKey registrieren'}
        </button>
      </form>
    </div>
  {/if}
</div>
