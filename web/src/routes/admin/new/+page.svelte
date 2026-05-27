<script lang="ts">
  import { adminApi, type ApiError } from '$lib/api';
  import { generateKey, exportKeyB64url } from '$lib/crypto';
  import { session } from '$lib/session.svelte';
  import {
    type FormField,
    type FieldType,
    fieldTypeLabel,
    deriveFieldId,
    defaultSchema,
    MAX_FIELDS,
    MAX_LABEL,
    MAX_PLACEHOLDER
  } from '$lib/forms';

  let description = $state('');
  let expiresInHours = $state(72);
  let customForm = $state(false);
  let fields = $state<FormField[]>(defaultSchema());
  let submitting = $state(false);

  // Schema that's actually sent to the server: the user's custom fields when
  // the builder is toggled on, otherwise the single-textarea default. We
  // never let an empty schema reach the API.
  const schemaToSend = $derived<FormField[]>(customForm ? fields : defaultSchema());
  let error = $state<string | null>(null);
  let shareUrl = $state<string | null>(null);
  let requestId = $state<string | null>(null);
  let copied = $state(false);

  function addField(type: FieldType) {
    if (fields.length >= MAX_FIELDS) return;
    const existing = new Set(fields.map((f) => f.id));
    const defaultLabel = {
      text: 'Text',
      password: 'Passwort',
      textarea: 'Notiz'
    }[type];
    const id = deriveFieldId(defaultLabel, existing);
    fields = [...fields, { id, label: defaultLabel, type, placeholder: '' }];
  }

  function removeField(idx: number) {
    if (fields.length <= 1) {
      // keep at least one field
      fields = defaultSchema();
      return;
    }
    fields = fields.filter((_, i) => i !== idx);
  }

  function moveField(idx: number, delta: -1 | 1) {
    const target = idx + delta;
    if (target < 0 || target >= fields.length) return;
    const next = fields.slice();
    [next[idx], next[target]] = [next[target]!, next[idx]!];
    fields = next;
  }

  // Derive a stable, unique id from the final label — only on blur. Doing
  // this on every keystroke would change the {#each} key when keyed by id
  // and remount the input. The id is invisible to the operator anyway, it's
  // only used as a JSON object key in the encrypted payload.
  function commitLabel(idx: number) {
    const f = fields[idx];
    if (!f) return;
    const otherIds = new Set(fields.filter((_, i) => i !== idx).map((x) => x.id));
    const newId = deriveFieldId(f.label, otherIds);
    if (newId !== f.id) f.id = newId;
  }

  const expiryPresets = [
    { hours: 1, label: '1 Stunde' },
    { hours: 24, label: '24 Stunden' },
    { hours: 72, label: '3 Tage' },
    { hours: 168, label: '1 Woche' }
  ];

  async function submit(e: Event) {
    e.preventDefault();
    if (submitting) return;
    if (!session.isUnlocked) {
      error = 'Session ist nicht entsperrt — bitte oben links entsperren.';
      return;
    }
    submitting = true;
    error = null;
    try {
      // Per-request AES key: stays extractable so we can put the raw form
      // into the URL fragment for the customer. The wrap call works on
      // extractable keys.
      const key = await generateKey();
      const keyB64 = await exportKeyB64url(key);
      const { wrappedKeyB64, wrapIvB64 } = await session.wrapRequestKey(key);

      const res = await adminApi.create({
        description: description.trim(),
        expires_in_hours: expiresInHours,
        wrapped_key_b64: wrappedKeyB64,
        wrap_iv_b64: wrapIvB64,
        form_schema: schemaToSend
      });
      requestId = res.request_id;
      shareUrl = `${location.origin}/r/${res.token}#${keyB64}`;
    } catch (e) {
      error = (e as Error).message || (e as ApiError).message || 'Konnte Request nicht anlegen';
    } finally {
      submitting = false;
    }
  }

  async function copyShare() {
    if (!shareUrl) return;
    await navigator.clipboard.writeText(shareUrl);
    copied = true;
    setTimeout(() => (copied = false), 1500);
  }
</script>

<svelte:head><title>GoGrab — Neuer Request</title></svelte:head>

<div class="mx-auto max-w-2xl px-6 py-8">
  <a href="/admin" class="mb-4 inline-flex items-center gap-1 text-sm text-slate-500 hover:text-slate-900">
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="15 18 9 12 15 6" />
    </svg>
    Zurück zur Liste
  </a>

  {#if shareUrl}
    <!-- Success / share screen -->
    <div class="rounded-xl border border-emerald-200 bg-white shadow-sm">
      <div class="border-b border-emerald-100 bg-emerald-50 px-6 py-4">
        <div class="flex items-center gap-2">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" class="text-emerald-700">
            <polyline points="20 6 9 17 4 12" />
          </svg>
          <h1 class="text-lg font-semibold text-emerald-900">Request angelegt</h1>
        </div>
        <p class="mt-1 text-sm text-emerald-800">
          Schicke diesen Link an deinen Kunden. Er gibt dort das Geheimnis ein.
        </p>
      </div>

      <div class="space-y-4 p-6">
        <!-- The URL -->
        <div>
          <div class="mb-1 flex items-center justify-between">
            <label for="share" class="text-xs font-medium uppercase tracking-wide text-slate-600">
              Share-URL
            </label>
            <button
              type="button"
              onclick={copyShare}
              class="inline-flex items-center gap-1.5 rounded-md bg-slate-900 px-3 py-1.5 text-xs font-medium text-white shadow-sm hover:bg-slate-800"
            >
              {#if copied}
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
                  <polyline points="20 6 9 17 4 12" />
                </svg>
                Kopiert
              {:else}
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                  <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                </svg>
                Kopieren
              {/if}
            </button>
          </div>
          <textarea
            id="share"
            readonly
            rows="3"
            class="w-full break-all rounded-md border border-slate-300 bg-slate-50 p-3 font-mono text-xs text-slate-900"
            >{shareUrl}</textarea
          >
        </div>

        <!-- What now -->
        <div class="rounded-md border border-slate-200 bg-slate-50 p-3 text-sm text-slate-700">
          <p class="font-medium text-slate-900">Was passiert jetzt?</p>
          <ol class="mt-1 list-decimal space-y-0.5 pl-4 text-slate-700">
            <li>Schicke den Link per Mail / Messenger an den Kunden.</li>
            <li>
              Sobald er einreicht, klick im Request-Detail auf <em>Abrufen</em> — deine
              entsperrte Session entschlüsselt das Geheimnis automatisch.
            </li>
            <li>Beim Abruf wird der Chiffretext einmalig gelesen und auf dem Server gelöscht.</li>
          </ol>
          <p class="mt-2 text-xs text-slate-500">
            Den Link musst du dir nicht merken — der Schlüssel liegt mehrfach verschlüsselt auch in deiner Datenbank.
          </p>
        </div>

        <div class="flex gap-2 pt-2">
          <a
            href="/admin/{requestId}"
            class="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-slate-800"
          >
            Zum Request
          </a>
          <a
            href="/admin/new"
            class="rounded-md border border-slate-300 bg-white px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50"
          >
            Noch einen anlegen
          </a>
        </div>
      </div>
    </div>
  {:else}
    <!-- Form -->
    <div class="rounded-xl border border-slate-200 bg-white shadow-sm">
      <div class="border-b border-slate-200 px-6 py-4">
        <h1 class="text-lg font-semibold text-slate-900">Neuer Secret-Request</h1>
        <p class="mt-1 text-sm text-slate-600">
          Beschreibe, was der Kunde eingeben soll. Der Inhalt wird im Browser des Kunden verschlüsselt — der Server bekommt nie Klartext zu sehen.
        </p>
      </div>

      <form onsubmit={submit} class="space-y-5 p-6">
        <div>
          <label for="desc" class="block text-sm font-medium text-slate-700">
            Beschreibung
            <span class="font-normal text-slate-500">— dem Kunden angezeigt</span>
          </label>
          <input
            id="desc"
            required
            maxlength="200"
            bind:value={description}
            class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500"
            placeholder="z.B. Bitte hinterlege dein WLAN-Passwort"
          />
          <p class="mt-1 text-xs text-slate-500">
            {description.length}/200 Zeichen
          </p>
        </div>

        <div>
          <span class="block text-sm font-medium text-slate-700">Ablaufzeit</span>
          <div class="mt-2 flex flex-wrap gap-2">
            {#each expiryPresets as p (p.hours)}
              <button
                type="button"
                onclick={() => (expiresInHours = p.hours)}
                class="rounded-md border px-3 py-1.5 text-sm font-medium transition {expiresInHours === p.hours
                  ? 'border-slate-900 bg-slate-900 text-white'
                  : 'border-slate-300 bg-white text-slate-700 hover:border-slate-400'}"
              >
                {p.label}
              </button>
            {/each}
            <label class="flex items-center gap-2 text-sm text-slate-600">
              <span>oder</span>
              <input
                type="number"
                min="1"
                max="720"
                bind:value={expiresInHours}
                class="w-20 rounded-md border border-slate-300 px-2 py-1 text-sm"
              />
              <span>h</span>
            </label>
          </div>
        </div>

        <!-- Form builder (collapsed by default) -->
        <div>
          <div class="flex items-start justify-between gap-3">
            <div>
              <span class="text-sm font-medium text-slate-700">Formular</span>
              <p class="mt-0.5 text-xs text-slate-500">
                {#if customForm}
                  Du baust eigene Felder. Bei Passwort-Feldern bekommt der Kunde einen Generator.
                {:else}
                  Ein mehrzeiliges Textfeld — der Kunde gibt einen freien Text ein.
                {/if}
              </p>
            </div>
            <label
              class="flex shrink-0 cursor-pointer items-center gap-2"
              title={customForm ? 'Zurück zu Standard-Textfeld' : 'Eigenes Formular bauen'}
            >
              <span class="text-xs font-medium text-slate-600">{customForm ? 'an' : 'aus'}</span>
              <span class="relative inline-block h-5 w-9">
                <input type="checkbox" bind:checked={customForm} class="peer sr-only" />
                <span class="absolute inset-0 rounded-full bg-slate-300 transition-colors peer-checked:bg-slate-900"></span>
                <span class="absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-white shadow transition-transform peer-checked:translate-x-4"></span>
              </span>
            </label>
          </div>

          {#if customForm}
          <div class="mt-3">
          <div class="mb-2 flex items-center justify-end">
            <span class="text-xs text-slate-500">{fields.length} / {MAX_FIELDS} Felder</span>
          </div>

          <ul class="space-y-2">
            {#each fields as f, idx (idx)}
              <li class="rounded-lg border border-slate-200 bg-slate-50 p-3">
                <div class="flex items-start gap-2">
                  <div class="flex flex-col gap-0.5">
                    <button
                      type="button"
                      onclick={() => moveField(idx, -1)}
                      disabled={idx === 0}
                      class="rounded p-0.5 text-slate-400 hover:bg-slate-200 hover:text-slate-700 disabled:opacity-30"
                      title="Hoch"
                      aria-label="Hoch"
                    >
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="18 15 12 9 6 15" /></svg>
                    </button>
                    <button
                      type="button"
                      onclick={() => moveField(idx, 1)}
                      disabled={idx === fields.length - 1}
                      class="rounded p-0.5 text-slate-400 hover:bg-slate-200 hover:text-slate-700 disabled:opacity-30"
                      title="Runter"
                      aria-label="Runter"
                    >
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9" /></svg>
                    </button>
                  </div>

                  <div class="grid flex-1 grid-cols-12 gap-2">
                    <div class="col-span-12 sm:col-span-7">
                      <label class="text-[11px] font-medium uppercase tracking-wide text-slate-500" for="label-{idx}">
                        Label
                      </label>
                      <input
                        id="label-{idx}"
                        required
                        maxlength={MAX_LABEL}
                        bind:value={fields[idx]!.label}
                        onblur={() => commitLabel(idx)}
                        class="mt-0.5 block w-full rounded-md border border-slate-300 bg-white px-2 py-1.5 text-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500"
                        placeholder="z.B. WLAN-Passwort"
                      />
                    </div>
                    <div class="col-span-12 sm:col-span-5">
                      <label class="text-[11px] font-medium uppercase tracking-wide text-slate-500" for="type-{idx}">
                        Typ
                      </label>
                      <select
                        id="type-{idx}"
                        bind:value={fields[idx]!.type}
                        class="mt-0.5 block w-full rounded-md border border-slate-300 bg-white px-2 py-1.5 text-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500"
                      >
                        <option value="text">{fieldTypeLabel.text}</option>
                        <option value="password">{fieldTypeLabel.password}</option>
                        <option value="textarea">{fieldTypeLabel.textarea}</option>
                      </select>
                    </div>
                    <div class="col-span-12">
                      <label class="text-[11px] font-medium uppercase tracking-wide text-slate-500" for="ph-{idx}">
                        Platzhalter <span class="font-normal lowercase text-slate-400">— optional, Hilfetext im leeren Feld</span>
                      </label>
                      <input
                        id="ph-{idx}"
                        maxlength={MAX_PLACEHOLDER}
                        bind:value={fields[idx]!.placeholder}
                        class="mt-0.5 block w-full rounded-md border border-slate-300 bg-white px-2 py-1.5 text-sm focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500"
                        placeholder="z.B. FritzBox-Standard…"
                      />
                    </div>
                  </div>

                  <button
                    type="button"
                    onclick={() => removeField(idx)}
                    class="rounded p-1 text-slate-400 hover:bg-rose-100 hover:text-rose-700"
                    title="Feld entfernen"
                    aria-label="Feld entfernen"
                  >
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <line x1="18" y1="6" x2="6" y2="18" />
                      <line x1="6" y1="6" x2="18" y2="18" />
                    </svg>
                  </button>
                </div>
              </li>
            {/each}
          </ul>

          <div class="mt-3 flex flex-wrap gap-2">
            <button
              type="button"
              onclick={() => addField('text')}
              disabled={fields.length >= MAX_FIELDS}
              class="inline-flex items-center gap-1 rounded-md border border-slate-300 bg-white px-3 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50"
            >
              + Text
            </button>
            <button
              type="button"
              onclick={() => addField('password')}
              disabled={fields.length >= MAX_FIELDS}
              class="inline-flex items-center gap-1 rounded-md border border-slate-300 bg-white px-3 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50"
            >
              + Passwort
            </button>
            <button
              type="button"
              onclick={() => addField('textarea')}
              disabled={fields.length >= MAX_FIELDS}
              class="inline-flex items-center gap-1 rounded-md border border-slate-300 bg-white px-3 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50"
            >
              + Mehrzeilig
            </button>
          </div>
          </div>
          {/if}
        </div>

        {#if error}
          <div class="rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800">
            {error}
          </div>
        {/if}

        <div class="flex items-center gap-3 border-t border-slate-100 pt-4">
          <button
            type="submit"
            disabled={submitting || !description.trim()}
            class="inline-flex items-center gap-1.5 rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {#if submitting}
              <svg class="animate-spin" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <line x1="12" y1="2" x2="12" y2="6" />
                <line x1="12" y1="18" x2="12" y2="22" />
                <line x1="4.93" y1="4.93" x2="7.76" y2="7.76" />
                <line x1="16.24" y1="16.24" x2="19.07" y2="19.07" />
                <line x1="2" y1="12" x2="6" y2="12" />
                <line x1="18" y1="12" x2="22" y2="12" />
                <line x1="4.93" y1="19.07" x2="7.76" y2="16.24" />
                <line x1="16.24" y1="7.76" x2="19.07" y2="4.93" />
              </svg>
              Wird angelegt …
            {:else}
              Request anlegen
            {/if}
          </button>
          <a href="/admin" class="text-sm text-slate-500 hover:text-slate-900">Abbrechen</a>
        </div>
      </form>
    </div>

    <!-- Flow explanation -->
    <div class="mt-6 grid gap-3 sm:grid-cols-3">
      {#each [
        { n: 1, t: 'Link erzeugen', d: 'Der AES-256-Schlüssel wird in deinem Browser erstellt.' },
        { n: 2, t: 'Link teilen', d: 'Der Kunde öffnet ihn und gibt das Geheimnis ein.' },
        { n: 3, t: 'Einmalig abrufen', d: 'Du entschlüsselst lokal. Der Server löscht den Chiffretext.' }
      ] as step (step.n)}
        <div class="rounded-lg border border-slate-200 bg-white p-3">
          <div class="grid h-6 w-6 place-items-center rounded-full bg-slate-900 text-xs font-semibold text-white">
            {step.n}
          </div>
          <div class="mt-2 text-sm font-medium text-slate-900">{step.t}</div>
          <div class="mt-0.5 text-xs text-slate-600">{step.d}</div>
        </div>
      {/each}
    </div>
  {/if}
</div>
