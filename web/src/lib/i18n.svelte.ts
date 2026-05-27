// Minimal i18n for the customer-facing /r/{token} page.
// Two locales: de (default), en. Language detection precedence:
//   1. ?lang=… URL search param (one-shot override, no persistence)
//   2. navigator.language
//   3. fallback to 'de'

export type Locale = 'de' | 'en';

const messages = {
  de: {
    'page.title': '{brand} — Geheimnis übermitteln',
    'loading': 'Lade …',
    'card.unavailable.title': 'Nicht verfügbar',
    'card.unavailable.fallback': 'Anfrage nicht gefunden',
    'card.no_data': 'Keine Daten.',
    'card.missing_key.title': 'Schlüssel fehlt',
    'card.missing_key.text':
      'Dieser Link enthält keinen Verschlüsselungs-Schlüssel. Bitte den Absender, den Link erneut zu schicken.',
    'card.bad_key.text': 'Der Schlüssel in diesem Link ist ungültig.',
    'card.expired.title': 'Abgelaufen',
    'card.expired.text':
      'Diese Anfrage ist abgelaufen und akzeptiert keine Einreichungen mehr.',
    'card.already.title': 'Bereits eingereicht',
    'card.already.text': 'Für diese Anfrage wurde bereits ein Geheimnis eingereicht.',
    'card.done.title': 'Übermittelt',
    'card.done.text':
      'Deine Eingaben wurden in deinem Browser verschlüsselt und sicher gesendet. Du kannst dieses Tab jetzt schließen.',
    'card.done.pw_hint':
      'Vergiss nicht: generierte Passwörter brauchst du selbst (in der Zwischenablage oder gespeichert). Der Empfänger sieht sie nach Abruf nicht erneut.',
    'pw.placeholder': 'eigenes Passwort oder generieren …',
    'pw.show': 'Anzeigen',
    'pw.hide': 'Verbergen',
    'pw.generate': '↻ Generieren',
    'pw.copy': '📋 Kopieren',
    'pw.adjust': 'Anpassen',
    'pw.length': 'Länge',
    'pw.symbols': 'Sonderzeichen',
    'pw.on': 'an',
    'pw.off': 'aus',
    'pw.strength.weak': 'schwach',
    'pw.strength.ok': 'ok',
    'pw.strength.strong': 'stark',
    'pw.strength.very_strong': 'sehr stark',
    'pw.bits': '{n} bits',
    'pw.confirm_saved': 'Ich habe „{label}" kopiert / gespeichert.',
    'warn.save_passwords':
      'Passwörter speichern. Nach dem Absenden ruft der Empfänger sie einmalig ab — danach sind sie auf dem Server gelöscht.',
    'submit.button.idle': 'Sicher übermitteln',
    'submit.button.busy': 'Verschlüssele & sende …',
    'submit.note':
      'Deine Eingaben werden direkt in deinem Browser verschlüsselt, bevor sie gesendet werden. Der Server kann den Inhalt nicht lesen.',
    'confirm.title': 'Bestätigen',
    'confirm.lead': 'Du übermittelst die folgenden Eingaben verschlüsselt an {brand}:',
    'confirm.note':
      'Werte werden in deinem Browser verschlüsselt, bevor sie gesendet werden — niemand außer {brand} kann sie lesen.',
    'confirm.back': 'Zurück, ich will noch was ändern',
    'confirm.send': 'An {brand} senden',
    'confirm.sending': 'Verschlüssele & sende …',
    'footer.unbranded': 'Powered by GoGrab — Ende-zu-Ende verschlüsselte Geheimnis-Übermittlung.',
    'footer.branded':
      'Sicher übermittelt mit GoGrab für {brand} — Ende-zu-Ende verschlüsselt.',
    'error.submit_failed': 'Senden fehlgeschlagen',
    'lang.switch': 'EN'
  },
  en: {
    'page.title': '{brand} — submit secret',
    'loading': 'Loading …',
    'card.unavailable.title': 'Unavailable',
    'card.unavailable.fallback': 'Request not found',
    'card.no_data': 'No data.',
    'card.missing_key.title': 'Missing key',
    'card.missing_key.text':
      "This link is missing its decryption key. Ask the sender to resend it.",
    'card.bad_key.text': 'The key in this link is malformed.',
    'card.expired.title': 'Expired',
    'card.expired.text': 'This request has expired and no longer accepts submissions.',
    'card.already.title': 'Already submitted',
    'card.already.text': 'A secret has already been submitted for this request.',
    'card.done.title': 'Submitted',
    'card.done.text':
      'Your input was encrypted in your browser and sent securely. You can close this tab now.',
    'card.done.pw_hint':
      "Don't forget: any generated passwords are yours to keep (clipboard or password manager). The recipient won't see them again after retrieval.",
    'pw.placeholder': 'your own password or generate one …',
    'pw.show': 'Show',
    'pw.hide': 'Hide',
    'pw.generate': '↻ Generate',
    'pw.copy': '📋 Copy',
    'pw.adjust': 'Adjust',
    'pw.length': 'Length',
    'pw.symbols': 'Symbols',
    'pw.on': 'on',
    'pw.off': 'off',
    'pw.strength.weak': 'weak',
    'pw.strength.ok': 'ok',
    'pw.strength.strong': 'strong',
    'pw.strength.very_strong': 'very strong',
    'pw.bits': '{n} bits',
    'pw.confirm_saved': 'I have copied / saved “{label}”.',
    'warn.save_passwords':
      'Save passwords now. The recipient retrieves them once — after that they are deleted from the server.',
    'submit.button.idle': 'Submit securely',
    'submit.button.busy': 'Encrypting & sending …',
    'submit.note':
      'Your input is encrypted in your browser before being sent. The server cannot read it.',
    'confirm.title': 'Confirm',
    'confirm.lead': "You're about to send the following values, encrypted, to {brand}:",
    'confirm.note':
      "Values are encrypted in your browser before sending — nobody but {brand} can read them.",
    'confirm.back': 'Back, I want to change something',
    'confirm.send': 'Send to {brand}',
    'confirm.sending': 'Encrypting & sending …',
    'footer.unbranded': 'Powered by GoGrab — end-to-end encrypted secret submission.',
    'footer.branded': 'Securely submitted via GoGrab for {brand} — end-to-end encrypted.',
    'error.submit_failed': 'Send failed',
    'lang.switch': 'DE'
  }
} as const;

type MessageKey = keyof typeof messages.de;

function detectLocale(): Locale {
  if (typeof window === 'undefined') return 'de';
  const urlOverride = new URLSearchParams(window.location.search).get('lang');
  if (urlOverride === 'en' || urlOverride === 'de') return urlOverride;
  const nav = navigator.language?.toLowerCase() ?? '';
  if (nav.startsWith('en')) return 'en';
  return 'de';
}

class I18n {
  locale = $state<Locale>('de');

  init() {
    this.locale = detectLocale();
  }

  toggle() {
    this.locale = this.locale === 'de' ? 'en' : 'de';
  }

  t(key: MessageKey, params?: Record<string, string | number>): string {
    const dict = messages[this.locale];
    let s: string = dict[key];
    if (!s) return key;
    if (params) {
      for (const [k, v] of Object.entries(params)) {
        s = s.replaceAll('{' + k + '}', String(v));
      }
    }
    return s;
  }
}

export const i18n = new I18n();
