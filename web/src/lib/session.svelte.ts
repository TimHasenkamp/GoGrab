// In-memory session state for the admin SPA: holds the unlocked Master-KEK
// as a non-extractable CryptoKey. Lives only in the current tab and is gone
// the moment the tab closes (or the user explicitly locks).
//
// IMPORTANT: never persist the master key to localStorage / sessionStorage /
// IndexedDB. It exists only in JS heap.

import { b64urlToBytes, bytesToB64url } from './webauthn';

// AES-256-GCM wrap key derived from a WebAuthn PRF output (32 bytes).
async function deriveWrapKey(prfOutput: Uint8Array): Promise<CryptoKey> {
  // PRF outputs are uniformly random secret material. Per RFC 5869 we could
  // run HKDF, but the input is already 256 bits of secret entropy from a
  // CSPRNG path inside the authenticator, so we can import directly.
  // We DO domain-separate via a label so the same PRF output cannot be reused
  // for a different purpose in a future protocol version.
  const baseKey = await crypto.subtle.importKey(
    'raw',
    prfOutput,
    'HKDF',
    false,
    ['deriveKey']
  );
  const label = new TextEncoder().encode('gograb.wrap.v1');
  return crypto.subtle.deriveKey(
    { name: 'HKDF', hash: 'SHA-256', salt: new Uint8Array(0), info: label },
    baseKey,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt', 'decrypt', 'wrapKey', 'unwrapKey']
  );
}

class Session {
  username = $state<string | null>(null);
  hasCredentials = $state<boolean | null>(null);
  prfSaltB64 = $state<string | null>(null);
  /** The unwrapped Master-KEK. Non-extractable; only usable via subtle API. */
  masterKek = $state<CryptoKey | null>(null);
  unlockingCredentialIdB64 = $state<string | null>(null);

  get isUnlocked(): boolean {
    return this.masterKek !== null;
  }

  get prfSalt(): Uint8Array | null {
    return this.prfSaltB64 ? b64urlToBytes(this.prfSaltB64) : null;
  }

  /** Called after WebAuthn registration: derive wrap key from PRF, generate
   * a fresh Master-KEK, return the wrapped master + IV for the server. */
  async createMasterAndWrap(prfOutput: Uint8Array): Promise<{
    wrappedMasterB64: string;
    wrapIvB64: string;
  }> {
    const wrapKey = await deriveWrapKey(prfOutput);
    const masterRaw = crypto.getRandomValues(new Uint8Array(32));
    const iv = crypto.getRandomValues(new Uint8Array(12));
    const wrapped = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, wrapKey, masterRaw);

    // Keep the master key in memory as non-extractable.
    this.masterKek = await crypto.subtle.importKey(
      'raw',
      masterRaw,
      { name: 'AES-GCM' },
      false,
      ['encrypt', 'decrypt']
    );

    return {
      wrappedMasterB64: bytesToB64url(new Uint8Array(wrapped)),
      wrapIvB64: bytesToB64url(iv)
    };
  }

  /** Called when registering a BACKUP credential while already unlocked: wrap
   * the existing in-memory master KEK with the new credential's PRF output. */
  async wrapExistingMaster(prfOutput: Uint8Array): Promise<{
    wrappedMasterB64: string;
    wrapIvB64: string;
  }> {
    if (!this.masterKek) throw new Error('Session ist nicht entsperrt');
    // We need the raw bytes for re-wrapping. To allow this we re-import
    // ourselves as extractable in a side-channel-free way: not possible with
    // the current non-extractable key. Instead, we use AES-KW via the
    // subtle.wrapKey API which works on non-extractable keys IF the wrap is
    // an AES key. Master is AES-GCM and target wrap is also AES-GCM. Use
    // wrapKey directly.
    const wrapKey = await deriveWrapKey(prfOutput);
    const iv = crypto.getRandomValues(new Uint8Array(12));
    const wrapped = await crypto.subtle.wrapKey(
      'raw',
      this.masterKek,
      wrapKey,
      { name: 'AES-GCM', iv }
    );
    return {
      wrappedMasterB64: bytesToB64url(new Uint8Array(wrapped)),
      wrapIvB64: bytesToB64url(iv)
    };
  }

  /** Called after WebAuthn login: derive wrap key, unwrap server-returned
   * wrapped master, store master in memory as non-extractable AES-GCM key. */
  async unlock(
    prfOutput: Uint8Array,
    wrappedMasterB64: string,
    wrapIvB64: string,
    credentialIdB64: string
  ): Promise<void> {
    const wrapKey = await deriveWrapKey(prfOutput);
    const wrapped = b64urlToBytes(wrappedMasterB64);
    const iv = b64urlToBytes(wrapIvB64);
    try {
      this.masterKek = await crypto.subtle.unwrapKey(
        'raw',
        wrapped,
        wrapKey,
        { name: 'AES-GCM', iv },
        { name: 'AES-GCM', length: 256 },
        false,
        ['encrypt', 'decrypt']
      );
    } catch {
      throw new Error(
        'Konnte den Master-Schlüssel nicht entwrappen — falscher YubiKey oder beschädigte Daten.'
      );
    }
    this.unlockingCredentialIdB64 = credentialIdB64;
  }

  lock(): void {
    this.masterKek = null;
    this.unlockingCredentialIdB64 = null;
  }

  /** Wrap a freshly-generated per-request AES key with the master KEK. */
  async wrapRequestKey(requestKey: CryptoKey): Promise<{
    wrappedKeyB64: string;
    wrapIvB64: string;
  }> {
    if (!this.masterKek) throw new Error('Session ist nicht entsperrt');
    const iv = crypto.getRandomValues(new Uint8Array(12));
    const wrapped = await crypto.subtle.wrapKey(
      'raw',
      requestKey,
      this.masterKek,
      { name: 'AES-GCM', iv }
    );
    return {
      wrappedKeyB64: bytesToB64url(new Uint8Array(wrapped)),
      wrapIvB64: bytesToB64url(iv)
    };
  }

  /** Inverse of wrapRequestKey: unwrap a stored per-request key. */
  async unwrapRequestKey(wrappedKeyB64: string, wrapIvB64: string): Promise<CryptoKey> {
    if (!this.masterKek) throw new Error('Session ist nicht entsperrt');
    const wrapped = b64urlToBytes(wrappedKeyB64);
    const iv = b64urlToBytes(wrapIvB64);
    return crypto.subtle.unwrapKey(
      'raw',
      wrapped,
      this.masterKek,
      { name: 'AES-GCM', iv },
      { name: 'AES-GCM', length: 256 },
      true, // exportable so we can stuff it into the URL fragment for the customer
      ['encrypt', 'decrypt']
    );
  }
}

export const session = new Session();
