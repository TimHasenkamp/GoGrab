-- =================== operators ===================

-- name: GetOperatorByUsername :one
SELECT id, username, email, prf_salt, created_at
FROM operators
WHERE username = $1;

-- name: CreateOperator :one
INSERT INTO operators (username, email, prf_salt)
VALUES ($1, $2, $3)
RETURNING id, username, email, prf_salt, created_at;

-- name: UpsertOperator :one
INSERT INTO operators (username, email, prf_salt)
VALUES ($1, $2, $3)
ON CONFLICT (username) DO UPDATE
  SET email = EXCLUDED.email
RETURNING id, username, email, prf_salt, created_at;

-- =================== webauthn credentials ===================

-- name: ListCredentialsByOperator :many
SELECT id, operator_id, credential_id, public_key, sign_count, transports,
       label, aaguid, wrapped_master, wrap_iv, created_at, last_used_at
FROM webauthn_credentials
WHERE operator_id = $1
ORDER BY created_at ASC;

-- name: CountCredentialsByOperator :one
SELECT COUNT(*)::int AS n
FROM webauthn_credentials
WHERE operator_id = $1;

-- name: CreateCredential :one
INSERT INTO webauthn_credentials (
    operator_id, credential_id, public_key, sign_count, transports,
    label, aaguid, wrapped_master, wrap_iv
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, operator_id, credential_id, public_key, sign_count, transports,
          label, aaguid, wrapped_master, wrap_iv, created_at, last_used_at;

-- name: GetCredentialByCredentialID :one
SELECT id, operator_id, credential_id, public_key, sign_count, transports,
       label, aaguid, wrapped_master, wrap_iv, created_at, last_used_at
FROM webauthn_credentials
WHERE credential_id = $1;

-- name: UpdateCredentialAfterUse :exec
UPDATE webauthn_credentials
SET sign_count = $2,
    last_used_at = now()
WHERE id = $1;

-- name: DeleteCredential :exec
DELETE FROM webauthn_credentials
WHERE id = $1 AND operator_id = $2;

-- =================== requests ===================

-- name: CreateRequest :one
INSERT INTO requests (
    token, description, operator_id, expires_at, wrapped_key, wrap_iv, form_schema
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, token, description, operator_id, created_at, expires_at,
          submitted_at, retrieved_at, ciphertext, iv, wrapped_key, wrap_iv,
          status, form_schema;

-- name: GetRequestByToken :one
SELECT id, token, description, operator_id, created_at, expires_at,
       submitted_at, retrieved_at, ciphertext, iv, wrapped_key, wrap_iv,
       status, form_schema
FROM requests
WHERE token = $1;

-- name: GetRequestByID :one
SELECT id, token, description, operator_id, created_at, expires_at,
       submitted_at, retrieved_at, ciphertext, iv, wrapped_key, wrap_iv,
       status, form_schema
FROM requests
WHERE id = $1;

-- name: ListRequestsByOperator :many
SELECT id, token, description, operator_id, created_at, expires_at,
       submitted_at, retrieved_at, ciphertext, iv, wrapped_key, wrap_iv,
       status, form_schema
FROM requests
WHERE operator_id = @operator_id
  AND (@search::text = '' OR description ILIKE '%' || @search || '%')
ORDER BY created_at DESC
LIMIT @lim::int OFFSET @off::int;

-- name: CountRequestsByOperator :one
SELECT COUNT(*)::int AS n
FROM requests
WHERE operator_id = @operator_id
  AND (@search::text = '' OR description ILIKE '%' || @search || '%');

-- name: SubmitCiphertext :one
UPDATE requests
SET ciphertext = $2,
    iv = $3,
    submitted_at = now(),
    status = 'submitted'
WHERE token = $1
  AND status = 'pending'
  AND expires_at > now()
RETURNING id, token, description, operator_id, created_at, expires_at,
          submitted_at, retrieved_at, ciphertext, iv, wrapped_key, wrap_iv,
          status, form_schema;

-- name: MarkRetrievedAndPurge :one
UPDATE requests
SET retrieved_at = now(),
    status = 'retrieved',
    ciphertext = NULL,
    iv = NULL,
    wrapped_key = NULL,
    wrap_iv = NULL
WHERE id = $1
  AND status = 'submitted'
RETURNING id, token, description, operator_id, created_at, expires_at,
          submitted_at, retrieved_at, status;

-- name: DeleteRequest :exec
DELETE FROM requests WHERE id = $1 AND operator_id = $2;

-- name: ExpirePendingRequests :execrows
UPDATE requests
SET status = 'expired'
WHERE status = 'pending'
  AND expires_at <= now();

-- =================== audit log ===================

-- name: InsertAuditLog :exec
INSERT INTO audit_log (actor, action, request_id, operator_id, ip, user_agent, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListAuditByOperator :many
SELECT id, occurred_at, actor, action, request_id, operator_id, ip, user_agent, metadata
FROM audit_log
WHERE operator_id = $1
ORDER BY occurred_at DESC
LIMIT $2;

-- name: PruneAuditOlderThan :execrows
DELETE FROM audit_log WHERE occurred_at < $1;

-- name: CountViewsByRequest :one
SELECT COUNT(*)::int AS n
FROM audit_log
WHERE request_id = $1 AND action = 'request.view';

-- name: ListAuditByRequest :many
SELECT id, occurred_at, actor, action, request_id, operator_id, ip, user_agent, metadata
FROM audit_log
WHERE request_id = $1
ORDER BY occurred_at DESC;
