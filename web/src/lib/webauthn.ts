// Client-side WebAuthn ceremonies with PRF extension. Handles:
//   - base64url ↔ ArrayBuffer marshalling for the navigator.credentials API
//   - adding the `prf` extension hint with the operator's salt
//   - returning the PRF output (32 bytes) alongside the credential response
//
// The PRF output is the secret material used to derive the wrap key for the
// operator's master KEK. The server never sees PRF outputs.

export function b64urlToBytes(b64: string): Uint8Array {
  const pad = b64.length % 4 === 0 ? 0 : 4 - (b64.length % 4);
  const norm = b64.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat(pad);
  const bin = atob(norm);
  const out = new Uint8Array(bin.length);
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

export interface CeremonyResult {
  response: unknown;          // JSON ready to POST to /finish
  prfOutput: Uint8Array;      // 32-byte PRF eval output
}

function extractPRFOutput(cred: PublicKeyCredential): Uint8Array {
  const ext = cred.getClientExtensionResults() as Record<string, unknown>;
  const prf = (ext.prf ?? {}) as { results?: { first?: ArrayBuffer } };
  const first = prf.results?.first;
  if (!first) {
    throw new Error(
      'Dein Authenticator unterstützt die PRF-Extension nicht. Stelle sicher, dass dein YubiKey die Firmware 5.7+ hat (oder nutze einen anderen kompatiblen Authenticator).'
    );
  }
  return new Uint8Array(first);
}

/** Drive a registration ceremony. Adds `prf` extension with `salt` as the
 * eval input. Returns the JSON response for the server + the PRF output for
 * deriving the wrap key client-side. */
export async function register(salt: Uint8Array, optionsJSON: unknown): Promise<CeremonyResult> {
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
export async function authenticate(salt: Uint8Array, optionsJSON: unknown): Promise<CeremonyResult> {
  const opts = decodeAssertionOptions(optionsJSON);
  opts.extensions = {
    ...(opts.extensions ?? {}),
    prf: { eval: { first: salt } }
  } as AuthenticationExtensionsClientInputs;

  const cred = (await navigator.credentials.get({ publicKey: opts })) as PublicKeyCredential | null;
  if (!cred) throw new Error('Authentifizierung abgebrochen');
  return { response: encodeAssertion(cred), prfOutput: extractPRFOutput(cred) };
}
