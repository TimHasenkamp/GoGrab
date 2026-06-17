// Client-side WebAuthn ceremonies with PRF extension. Handles:
//   - base64url ↔ ArrayBuffer marshalling for the navigator.credentials API
//   - adding the `prf` extension hint with the operator's salt
//   - returning the PRF output (32 bytes) alongside the credential response
//
// The PRF output is the secret material used to derive the wrap key for the
// operator's master KEK. The server never sees PRF outputs.

// Return type pinned to Uint8Array<ArrayBuffer> (not the default
// Uint8Array<ArrayBufferLike> in TS ≥ 5.7) so the bytes are accepted directly
// by WebCrypto APIs that require a concrete ArrayBuffer-backed view.
export function b64urlToBytes(b64: string): Uint8Array<ArrayBuffer> {
  const pad = b64.length % 4 === 0 ? 0 : 4 - (b64.length % 4);
  const norm = b64.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat(pad);
  const bin = atob(norm);
  const out = new Uint8Array(new ArrayBuffer(bin.length));
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

export function bytesToB64url(bytes: Uint8Array): string {
  let bin = '';
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

function bufToB64url(buf: ArrayBuffer): string {
  return bytesToB64url(new Uint8Array(buf));
}

// Decode go-webauthn's PublicKeyCredentialCreationOptions JSON shape into the
// browser-native form (ArrayBuffers for the binary fields).
function decodeCreationOptions(json: any): PublicKeyCredentialCreationOptions {
  const o = json.publicKey ?? json;
  const ret: PublicKeyCredentialCreationOptions = {
    rp: o.rp,
    user: {
      ...o.user,
      id: b64urlToBytes(o.user.id)
    },
    challenge: b64urlToBytes(o.challenge),
    pubKeyCredParams: o.pubKeyCredParams,
    timeout: o.timeout,
    excludeCredentials: (o.excludeCredentials ?? []).map((c: any) => ({
      ...c,
      id: b64urlToBytes(c.id)
    })),
    authenticatorSelection: o.authenticatorSelection,
    attestation: o.attestation,
    extensions: o.extensions
  };
  return ret;
}

function decodeAssertionOptions(json: any): PublicKeyCredentialRequestOptions {
  const o = json.publicKey ?? json;
  return {
    challenge: b64urlToBytes(o.challenge),
    timeout: o.timeout,
    rpId: o.rpId,
    allowCredentials: (o.allowCredentials ?? []).map((c: any) => ({
      ...c,
      id: b64urlToBytes(c.id)
    })),
    userVerification: o.userVerification,
    extensions: o.extensions
  };
}

// Serialize an attestation (register) response back into the JSON shape
// go-webauthn expects on POST /register/finish.
function encodeAttestation(cred: PublicKeyCredential): unknown {
  const r = cred.response as AuthenticatorAttestationResponse;
  return {
    id: cred.id,
    rawId: bufToB64url(cred.rawId),
    type: cred.type,
    response: {
      clientDataJSON: bufToB64url(r.clientDataJSON),
      attestationObject: bufToB64url(r.attestationObject),
      transports: typeof r.getTransports === 'function' ? r.getTransports() : []
    },
    clientExtensionResults: cred.getClientExtensionResults()
  };
}

// Same for assertion (login).
function encodeAssertion(cred: PublicKeyCredential): unknown {
  const r = cred.response as AuthenticatorAssertionResponse;
  return {
    id: cred.id,
    rawId: bufToB64url(cred.rawId),
    type: cred.type,
    response: {
      clientDataJSON: bufToB64url(r.clientDataJSON),
      authenticatorData: bufToB64url(r.authenticatorData),
      signature: bufToB64url(r.signature),
      userHandle: r.userHandle ? bufToB64url(r.userHandle) : null
    },
    clientExtensionResults: cred.getClientExtensionResults()
  };
}

export interface RegisterResult {
  response: unknown;
  prfOutput: Uint8Array<ArrayBuffer> | null; // null when PRF not evaluated during create()
}

export interface AuthResult {
  response: unknown;
  prfOutput: Uint8Array<ArrayBuffer>; // always non-null; throws if PRF missing
}

/** @deprecated use RegisterResult or AuthResult */
export type CeremonyResult = AuthResult;

function extractPRFOutput(cred: PublicKeyCredential): Uint8Array<ArrayBuffer> | null {
  const ext = cred.getClientExtensionResults() as Record<string, unknown>;
  const prf = (ext.prf ?? {}) as { enabled?: boolean; results?: { first?: ArrayBuffer } };
  const first = prf.results?.first;
  if (!first) {
    // null = PRF supported but not evaluated in this ceremony (create() on some Linux+browser combos).
    // Callers that require PRF (authenticate) must throw on null.
    return null;
  }
  return new Uint8Array(first) as Uint8Array<ArrayBuffer>;
}

/** Drive a registration ceremony. Adds `prf` extension with `salt` as the
 * eval input. Returns the JSON response for the server + the PRF output for
 * deriving the wrap key client-side. */
export async function register(salt: Uint8Array<ArrayBuffer>, optionsJSON: unknown): Promise<RegisterResult> {
  const opts = decodeCreationOptions(optionsJSON);
  // Merge in the PRF extension; the server doesn't dictate this since it
  // doesn't need to know the salt (it just sends it as a hint).
  opts.extensions = {
    ...(opts.extensions ?? {}),
    prf: { eval: { first: salt } }
  } as AuthenticationExtensionsClientInputs;

  const cred = (await navigator.credentials.create({ publicKey: opts })) as PublicKeyCredential | null;
  if (!cred) throw new Error('Registrierung abgebrochen');
  return { response: encodeAttestation(cred), prfOutput: extractPRFOutput(cred) };
}

/** Drive an authentication ceremony with PRF extension. */
export async function authenticate(salt: Uint8Array<ArrayBuffer>, optionsJSON: unknown): Promise<AuthResult> {
  const opts = decodeAssertionOptions(optionsJSON);
  opts.extensions = {
    ...(opts.extensions ?? {}),
    prf: { eval: { first: salt } }
  } as AuthenticationExtensionsClientInputs;

  const cred = (await navigator.credentials.get({ publicKey: opts })) as PublicKeyCredential | null;
  if (!cred) throw new Error('Authentifizierung abgebrochen');
  const prfOutput = extractPRFOutput(cred);
  if (!prfOutput) {
    throw new Error(
      'PRF-Auswertung fehlgeschlagen. Stelle sicher, dass dein Authenticator hmac-secret unterstützt (YubiKey Security Key / YubiKey 5 mit Firmware 5.2+) und dass du deinen PIN eingegeben hast.'
    );
  }
  return { response: encodeAssertion(cred), prfOutput };
}
