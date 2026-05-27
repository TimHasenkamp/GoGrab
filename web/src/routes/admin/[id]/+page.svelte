<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { adminApi, type AdminRequestSummary, type ApiError } from '$lib/api';
  import { decrypt } from '$lib/crypto';
  import { session } from '$lib/session.svelte';
  import { defaultSchema, type FormField } from '$lib/forms';
  import { toast } from '$lib/toast.svelte';
  import { confirmStore } from '$lib/confirm.svelte';
  import Icon from '$lib/Icon.svelte';
  import {
    relativeTime,
    absoluteTime,
    statusLabel,
    statusBadge,
    type Status
  } from '$lib/format';

  const id = $derived($page.params.id ?? '');

  let request = $state<AdminRequestSummary | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let decrypted = $state<string | null>(null);
  let decryptedFields = $state<{ field: FormField; value: string }[] | null>(null);
  let revealed = $state<Record<string, boolean>>({});
  let copiedField = $state<string | null>(null);
  let decrypting = $state(false);
  let decryptError = $state<string | null>(null);

  let shareUrlVisible = $state(false);
  let shareUrlCopied = $state(false);
  const cachedShareUrl = $derived(session.recentShareUrls[id] ?? null);

  async function copyShareUrl() {
    if (!cachedShareUrl) return;
    await navigator.clipboard.writeText(cachedShareUrl);
    shareUrlCopied = true;
    setTimeout(() => (shareUrlCopied = false), 1500);
  }

  function mailtoShare(): string {
    if (!cachedShareUrl || !request) return '#';
    const subject = encodeURIComponent('Sichere Übermittlung — ' + request.description);
    const body = encodeURIComponent(
      `Hallo,\n\nbitte hinterlege das gewünschte Geheimnis sicher über diesen einmaligen Link:\n\n${cachedShareUrl}\n\nDer Inhalt wird in deinem Browser verschlüsselt.\n\nViele Grüße`
    );
    return `mailto:?subject=${subject}&body=${body}`;
  }

  async function load() {
    loading = true;
    error = null;
    try {
      request = await adminApi.get(id);
    } catch (e) {
      error = (e as ApiError).message || 'Konnte Request nicht laden';
    } finally {
      loading = false;
    }
  }

  onMount(load);

  async function reveal() {
    if (decrypting) return;
    if (!session.isUnlocked) {
      decryptError = 'Session ist nicht entsperrt.';
      return;
    }
    decrypting = true;
    decryptError = null;
    decrypted = null;
    decryptedFields = null;
    try {
      const payload = await adminApi.retrieve(id);
      const key = await session.unwrapRequestKey(
        payload.wrapped_key_b64,
        payload.wrap_iv_b64
      );
      const plaintext = await decrypt(payload.ciphertext_b64, payload.iv_b64, key);
      decrypted = plaintext;

      // Try to parse as the JSON object the customer's new flow sends. Fall
      // back to a single-field display for legacy / non-JSON payloads.
      const schema = request?.form_schema ?? defaultSchema();
      let parsed: Record<string, string> | null = null;
      try {
        const obj = JSON.parse(plaintext);
        if (obj && typeof obj === 'object' && !Array.isArray(obj)) {
          parsed = obj as Record<string, string>;
        }
      } catch {
        // not JSON — old single-string format
      }
      if (parsed) {
        decryptedFields = schema.map((f) => ({
          field: f,
          value: typeof parsed![f.id] === 'string' ? parsed![f.id]! : ''
        }));
      } else {
        decryptedFields = [
          { field: { id: 'secret', label: 'Geheimnis', type: 'textarea' }, value: plaintext }
        ];
      }
      await load();
    } catch (e) {
      decryptError =
        (e as ApiError).message || (e as Error).message || 'Entschlüsselung fehlgeschlagen';
    } finally {
      decrypting = false;
    }
  }

  async function copyFieldValue(id: string, value: string) {
    await navigator.clipboard.writeText(value);
    copiedField = id;
    setTimeout(() => {
      if (copiedField === id) copiedField = null;
    }, 1500);
  }

  function toggleReveal(id: string) {
    revealed = { ...revealed, [id]: !revealed[id] };
  }

  async function cancel() {
    const ok = await confirmStore.ask({
      title: 'Request löschen?',
      body: 'Der Link wird ungültig und der Kunde kann nichts mehr einreichen. Diese Aktion lässt sich nicht rückgängig machen.',
      confirmLabel: 'Endgültig löschen',
      destructive: true
    });
    if (!ok) return;
    try {
      await adminApi.remove(id);
      toast.success('Request gelöscht.');
      history.back();
    } catch (e) {
      toast.error((e as ApiError).message || 'Konnte nicht löschen');
    }
  }

  function timelineState(r: AdminRequestSummary): { step: string; state: 'done' | 'current' | 'todo' }[] {
    const out: { step: string; state: 'done' | 'current' | 'todo' }[] = [];
    out.push({ step: 'Angelegt', state: 'done' });
    if (r.status === 'pending') {
      out.push({ step: 'Warte auf Einreichung', state: 'current' });
      out.push({ step: 'Abrufen', state: 'todo' });
    } else if (r.status === 'submitted') {
      out.push({ step: 'Eingereicht', state: 'done' });
      out.push({ step: 'Bereit zum Abruf', state: 'current' });
    } else if (r.status === 'retrieved') {
      out.push({ step: 'Eingereicht', state: 'done' });
      out.push({ step: 'Abgerufen & gelöscht', state: 'done' });
    } else if (r.status === 'expired') {
      out.push({ step: 'Abgelaufen', state: 'current' });
    }
    return out;
  }
</script>

<svelte:head><title>GoGrab — Request</title></svelte:head>

<div class="mx-auto max-w-2xl px-6 py-8">
  <a href="/admin" class="mb-4 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="15 18 9 12 15 6" />
    </svg>
    Zurück zur Liste
  </a>

  {#if loading}
    <div class="h-40 animate-pulse rounded-lg border border-border bg-card"></div>
  {:else if error}
    <div class="rounded-lg border border-danger/30 bg-danger/10 p-4 text-sm text-danger">
      {error}
    </div>
  {:else if request}
    <div class="rounded-xl border border-border bg-card shadow-sm">
      <div class="border-b border-border px-6 py-5">
        <div class="flex items-start justify-between gap-3">
          <h1 class="text-lg font-semibold text-foreground">{request.description}</h1>
          <span class="shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ring-1 {statusBadge[request.status as Status]}">
            {statusLabel[request.status as Status]}
          </span>
        </div>
        <dl class="mt-3 grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
          <dt class="text-muted-foreground">Angelegt</dt>
          <dd class="text-foreground" title={absoluteTime(request.created_at)}>
            {relativeTime(request.created_at)}
          </dd>
          <dt class="text-muted-foreground">Läuft ab</dt>
          <dd class="text-foreground" title={absoluteTime(request.expires_at)}>
            {relativeTime(request.expires_at)}
          </dd>
          {#if request.submitted_at}
            <dt class="text-muted-foreground">Eingereicht</dt>
            <dd class="text-foreground" title={absoluteTime(request.submitted_at)}>
              {relativeTime(request.submitted_at)}
            </dd>
          {/if}
          {#if request.retrieved_at}
            <dt class="text-muted-foreground">Abgerufen</dt>
            <dd class="text-foreground" title={absoluteTime(request.retrieved_at)}>
              {relativeTime(request.retrieved_at)}
            </dd>
          {/if}
          {#if request.view_count != null && request.view_count > 0}
            <dt class="text-muted-foreground">Link geöffnet</dt>
            <dd class="text-foreground">
              {request.view_count}× vom Kunden{#if request.status === 'pending' && !request.submitted_at}<span class="ml-1 text-warning">— aber noch keine Einreichung</span>{/if}
            </dd>
          {/if}
        </dl>
      </div>

      <div class="border-b border-border px-6 py-4">
        <ol class="space-y-2">
          {#each timelineState(request) as t (t.step)}
            <li class="flex items-center gap-3 text-sm">
              {#if t.state === 'done'}
                <span class="grid h-5 w-5 place-items-center rounded-full bg-success text-background">
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12" /></svg>
                </span>
                <span class="text-foreground">{t.step}</span>
              {:else if t.state === 'current'}
                <span class="grid h-5 w-5 place-items-center rounded-full bg-accent">
                  <span class="h-1.5 w-1.5 animate-pulse rounded-full bg-card"></span>
                </span>
                <span class="font-medium text-foreground">{t.step}</span>
              {:else}
                <span class="h-5 w-5 rounded-full border-2 border-dashed border-border-strong"></span>
                <span class="text-muted-foreground/70">{t.step}</span>
              {/if}
            </li>
          {/each}
        </ol>
      </div>

      <div class="p-6">
        {#if request.status === 'pending'}
          <div class="rounded-md border border-border bg-background p-4 text-sm">
            <p class="font-medium text-foreground">Wartet auf den Kunden</p>
            <p class="mt-1 text-muted-foreground">
              Sobald der Kunde einreicht, kannst du das Geheimnis hier mit einem Klick abrufen.
            </p>

            {#if cachedShareUrl}
              <div class="mt-3">
                {#if !shareUrlVisible}
                  <button
                    type="button"
                    onclick={() => (shareUrlVisible = true)}
                    class="inline-flex items-center gap-1.5 rounded-md border border-border-strong bg-card px-3 py-1.5 text-xs font-medium text-foreground hover:bg-background"
                  >
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
                      <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
                    </svg>
                    Share-URL nochmal anzeigen
                  </button>
                {:else}
                  <div class="space-y-2">
                    <textarea
                      readonly
                      rows="3"
                      class="w-full break-all rounded-md border border-border-strong bg-card p-2 font-mono text-xs text-foreground"
                      >{cachedShareUrl}</textarea
                    >
                    <div class="flex flex-wrap gap-2">
                      <button
                        type="button"
                        onclick={copyShareUrl}
                        class="inline-flex items-center gap-1 rounded-md bg-accent px-3 py-1.5 text-xs font-medium text-background hover:bg-accent-hover"
                      >
                        <Icon name={shareUrlCopied ? 'check' : 'copy'} size={12} />
                        <span>{shareUrlCopied ? 'Kopiert' : 'Kopieren'}</span>
                      </button>
                      <a
                        href={mailtoShare()}
                        class="rounded-md border border-border-strong bg-card px-3 py-1.5 text-xs font-medium text-foreground hover:bg-background"
                      >
                        Per Mail senden
                      </a>
                      <button
                        type="button"
                        onclick={() => (shareUrlVisible = false)}
                        class="rounded-md px-3 py-1.5 text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
                      >
                        Verbergen
                      </button>
                    </div>
                    <p class="text-[11px] text-muted-foreground">
                      Hinweis: dieser Link existiert nur im Speicher dieses Tabs. Wenn du das Tab
                      schließt, ist er weg — dann musst du den Request canceln und neu anlegen.
                    </p>
                  </div>
                {/if}
              </div>
            {/if}

            <button
              type="button"
              onclick={cancel}
              class="mt-3 inline-flex items-center gap-1.5 rounded-md border border-danger/30 bg-card px-3 py-1.5 text-xs font-medium text-danger hover:bg-danger/10"
            >
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="3 6 5 6 21 6" />
                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
              </svg>
              Request löschen
            </button>
          </div>
        {:else if request.status === 'submitted'}
          <div>
            <h2 class="text-sm font-semibold text-foreground">Geheimnis abrufen</h2>
            <p class="mt-1 text-xs text-muted-foreground">
              Ein Klick — deine entsperrte Session entwrappt den Schlüssel und entschlüsselt lokal.
              Der Server löscht den Chiffretext nach diesem Aufruf.
            </p>
            {#if decryptError}
              <p class="mt-3 rounded-md border border-danger/30 bg-danger/10 px-3 py-2 text-sm text-danger">{decryptError}</p>
            {/if}
            <button
              type="button"
              onclick={reveal}
              disabled={decrypting || !session.isUnlocked}
              class="mt-3 inline-flex items-center gap-1.5 rounded-md bg-accent px-4 py-2 text-sm font-medium text-background shadow-sm hover:bg-accent-hover disabled:opacity-50"
            >
              {decrypting ? 'Entschlüssele …' : 'Abrufen & entschlüsseln'}
            </button>
          </div>
        {:else if request.status === 'expired'}
          <div class="rounded-md border border-danger/30 bg-danger/10 p-4 text-sm text-danger">
            Dieser Request ist abgelaufen, ohne dass der Kunde eingereicht hat.
          </div>
        {:else if request.status === 'retrieved'}
          <div class="rounded-md border border-border bg-background p-4 text-sm text-foreground">
            Das Geheimnis wurde bereits abgerufen und auf dem Server gelöscht. Es ist nicht erneut abrufbar.
          </div>
        {/if}
      </div>
    </div>

    {#if decryptedFields !== null}
      <div class="mt-4 rounded-xl border border-success/30 bg-card shadow-sm">
        <div class="flex items-center justify-between gap-3 border-b border-success/30 bg-success/10 px-6 py-3">
          <div class="flex items-center gap-2">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" class="text-success">
              <polyline points="20 6 9 17 4 12" />
            </svg>
            <h2 class="text-sm font-semibold text-success">Entschlüsselte Werte</h2>
          </div>
        </div>
        <ul class="divide-y divide-border">
          {#each decryptedFields as { field, value } (field.id)}
            <li class="px-6 py-4">
              <div class="mb-1 flex items-center justify-between gap-2">
                <span class="text-xs font-medium uppercase tracking-wide text-muted-foreground">{field.label}</span>
                <div class="flex items-center gap-1">
                  {#if field.type === 'password'}
                    <button
                      type="button"
                      onclick={() => toggleReveal(field.id)}
                      class="inline-flex items-center rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
                      title={revealed[field.id] ? 'Verbergen' : 'Anzeigen'}
                      aria-label={revealed[field.id] ? 'Verbergen' : 'Anzeigen'}
                    >
                      <Icon name={revealed[field.id] ? 'eye-off' : 'eye'} size={14} />
                    </button>
                  {/if}
                  <button
                    type="button"
                    onclick={() => copyFieldValue(field.id, value)}
                    class="inline-flex items-center gap-1 rounded-md bg-success px-2 py-0.5 text-xs font-medium text-background hover:bg-success"
                  >
                    <Icon name={copiedField === field.id ? 'check' : 'copy'} size={12} />
                    <span>{copiedField === field.id ? 'Kopiert' : 'Kopieren'}</span>
                  </button>
                </div>
              </div>
              {#if field.type === 'password' && !revealed[field.id]}
                <div class="font-mono text-sm tracking-widest text-foreground">
                  {'•'.repeat(Math.min(value.length, 40))}
                </div>
              {:else if field.type === 'textarea'}
                <pre class="whitespace-pre-wrap break-words rounded-md bg-background p-2 font-mono text-sm text-foreground">{value}</pre>
              {:else}
                <div class="select-all break-all rounded-md bg-background p-2 font-mono text-sm text-foreground">{value}</div>
              {/if}
            </li>
          {/each}
        </ul>
        <p class="border-t border-success/30 px-6 py-2 text-xs text-success">
          Kopiere die Werte oder schließe das Tab, wenn du fertig bist. Der Server hat keinen Zugriff mehr darauf.
        </p>
      </div>
    {/if}
  {/if}
</div>
