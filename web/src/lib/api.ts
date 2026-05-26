export interface ApiError {
  error: string;
  message: string;
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
    credentials: 'same-origin'
  });
  if (!res.ok) {
    let msg: ApiError = { error: 'http_' + res.status, message: res.statusText };
    try {
      const parsed = (await res.json()) as Partial<ApiError>;
      if (parsed.error) msg = { error: parsed.error, message: parsed.message ?? '' };
    } catch {
      // body was not json; keep default
    }
    throw msg;
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

// --- Admin ---
export interface AdminRequestSummary {
  id: string;
  token: string;
  description: string;
  created_at: string;
  expires_at: string;
  submitted_at: string | null;
  retrieved_at: string | null;
  status: 'pending' | 'submitted' | 'retrieved' | 'expired';
}

export interface CreateRequestResponse {
  request_id: string;
  token: string;
}

export interface RetrieveResponse {
  ciphertext_b64: string;
  iv_b64: string;
}

export const adminApi = {
  list: () => call<AdminRequestSummary[]>('GET', '/api/admin/requests'),
  get: (id: string) => call<AdminRequestSummary>('GET', `/api/admin/requests/${encodeURIComponent(id)}`),
  create: (description: string, expires_in_hours: number) =>
    call<CreateRequestResponse>('POST', '/api/admin/requests', { description, expires_in_hours }),
  retrieve: (id: string) =>
    call<RetrieveResponse>('POST', `/api/admin/requests/${encodeURIComponent(id)}/retrieve`),
  remove: (id: string) =>
    call<void>('DELETE', `/api/admin/requests/${encodeURIComponent(id)}`)
};

// --- Public ---
export interface PublicMeta {
  description: string;
  expires_at: string;
  status: 'pending' | 'submitted' | 'retrieved' | 'expired';
}

export const publicApi = {
  meta: (token: string) =>
    call<PublicMeta>('GET', `/api/requests/${encodeURIComponent(token)}/meta`),
  submit: (token: string, ciphertext_b64: string, iv_b64: string) =>
    call<{ ok: true }>('POST', `/api/requests/${encodeURIComponent(token)}/submit`, {
      ciphertext_b64,
      iv_b64
    })
};

// --- WebAuthn auth / unlock ---
export interface AuthStatus {
  has_credentials: boolean;
  prf_salt_b64: string;
  username: string;
}

export interface RegisterBeginResponse {
  options: unknown;
  session_token: string;
  prf_salt_b64: string;
}

export interface LoginBeginResponse {
  options: unknown;
  session_token: string;
  prf_salt_b64: string;
}

export interface LoginFinishResponse {
  credential_id_b64: string;
  wrapped_master_b64: string;
  wrap_iv_b64: string;
}

export interface CredentialSummary {
  id: string;
  label: string;
  transports: string[];
  created_at: string;
  last_used_at: string | null;
}

export const authApi = {
  status: () => call<AuthStatus>('GET', '/api/admin/auth/status'),
  registerBegin: () =>
    call<RegisterBeginResponse>('POST', '/api/admin/auth/register/begin', {}),
  registerFinish: (body: {
    credential_response: unknown;
    session_token: string;
    label: string;
    wrapped_master_b64: string;
    wrap_iv_b64: string;
  }) => call<CredentialSummary>('POST', '/api/admin/auth/register/finish', body),
  loginBegin: () => call<LoginBeginResponse>('POST', '/api/admin/auth/login/begin', {}),
  loginFinish: (body: { credential_response: unknown; session_token: string }) =>
    call<LoginFinishResponse>('POST', '/api/admin/auth/login/finish', body),
  listCredentials: () =>
    call<CredentialSummary[]>('GET', '/api/admin/auth/credentials'),
  deleteCredential: (id: string) =>
    call<void>('DELETE', `/api/admin/auth/credentials/${encodeURIComponent(id)}`)
};
