// Zero-knowledge client-side crypto. All encryption happens in the browser; the
// server never sees the AES key or plaintext. AES-256-GCM via Web Crypto API.

const AES_KEY_BITS = 256;
const IV_BYTES = 12;

function b64urlEncode(bytes: Uint8Array): string {
  let bin = '';
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

// Backed by an explicit ArrayBuffer so the result satisfies WebCrypto's
// BufferSource type (TS 5.7+ makes plain Uint8Array generic over
// ArrayBufferLike, which doesn't include the concrete ArrayBuffer required).
function b64urlDecode(s: string): Uint8Array<ArrayBuffer> {
  const pad = s.length % 4 === 0 ? 0 : 4 - (s.length % 4);
  const norm = s.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat(pad);
  const bin = atob(norm);
  const out = new Uint8Array(new ArrayBuffer(bin.length));
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

export async function generateKey(): Promise<CryptoKey> {
  return crypto.subtle.generateKey(
    { name: 'AES-GCM', length: AES_KEY_BITS },
    true,
    ['encrypt', 'decrypt']
  );
}

export async function exportKeyB64url(key: CryptoKey): Promise<string> {
  const raw = await crypto.subtle.exportKey('raw', key);
  return b64urlEncode(new Uint8Array(raw));
}

export async function importKeyB64url(b64: string): Promise<CryptoKey> {
  const raw = b64urlDecode(b64);
  if (raw.byteLength !== AES_KEY_BITS / 8) {
    throw new Error('invalid key length');
  }
  return crypto.subtle.importKey('raw', raw, { name: 'AES-GCM' }, true, ['encrypt', 'decrypt']);
}

export interface EncryptedPayload {
  ciphertextB64: string;
  ivB64: string;
}

export async function encrypt(plaintext: string, key: CryptoKey): Promise<EncryptedPayload> {
  const iv = crypto.getRandomValues(new Uint8Array(IV_BYTES));
  const enc = new TextEncoder().encode(plaintext);
  const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, enc);
  return { ciphertextB64: b64urlEncode(new Uint8Array(ct)), ivB64: b64urlEncode(iv) };
}

export async function decrypt(
  ciphertextB64: string,
  ivB64: string,
  key: CryptoKey
): Promise<string> {
  const ct = b64urlDecode(ciphertextB64);
  const iv = b64urlDecode(ivB64);
  const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, key, ct);
  return new TextDecoder().decode(pt);
}
