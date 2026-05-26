-- name: CreateRequest :one
INSERT INTO requests (token, description, created_by, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id, token, description, created_by, created_at, expires_at,
          submitted_at, retrieved_at, ciphertext, iv, status;

-- name: GetRequestByToken :one
SELECT id, token, description, created_by, created_at, expires_at,
       submitted_at, retrieved_at, ciphertext, iv, status
FROM requests
WHERE token = $1;

-- name: GetRequestByID :one
SELECT id, token, description, created_by, created_at, expires_at,
       submitted_at, retrieved_at, ciphertext, iv, status
FROM requests
WHERE id = $1;

-- name: ListRequestsByUser :many
SELECT id, token, description, created_by, created_at, expires_at,
       submitted_at, retrieved_at, ciphertext, iv, status
FROM requests
WHERE created_by = $1
ORDER BY created_at DESC
LIMIT 200;

-- name: SubmitCiphertext :one
UPDATE requests
SET ciphertext = $2,
    iv = $3,
    submitted_at = now(),
    status = 'submitted'
WHERE token = $1
  AND status = 'pending'
  AND expires_at > now()
RETURNING id, token, description, created_by, created_at, expires_at,
          submitted_at, retrieved_at, ciphertext, iv, status;

-- name: MarkRetrievedAndPurge :one
UPDATE requests
SET retrieved_at = now(),
    status = 'retrieved',
    ciphertext = NULL,
    iv = NULL
WHERE id = $1
  AND status = 'submitted'
RETURNING id, token, description, created_by, created_at, expires_at,
          submitted_at, retrieved_at, status;

-- name: DeleteRequest :exec
DELETE FROM requests WHERE id = $1 AND created_by = $2;

-- name: ExpirePendingRequests :execrows
UPDATE requests
SET status = 'expired'
WHERE status = 'pending'
  AND expires_at <= now();
