-- +goose Up
-- +goose StatementBegin

-- Clean cut: drop v1 requests table; v2 introduces an envelope-encryption
-- model where each request's AES key is wrapped with a per-operator master
-- KEK that's only unlocked client-side via a WebAuthn PRF tap.

DROP TABLE IF EXISTS requests;

-- One row per authenticated Authentik user.
CREATE TABLE operators (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username    TEXT UNIQUE NOT NULL,
    email       TEXT NOT NULL DEFAULT '',
    prf_salt    BYTEA NOT NULL,                -- 32 random bytes, used as PRF eval input
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- N credentials per operator (Primary + Backup recommended).
-- wrapped_master holds the operator's master KEK encrypted with the
-- PRF-derived wrap key for THIS specific credential.
CREATE TABLE webauthn_credentials (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operator_id     UUID NOT NULL REFERENCES operators(id) ON DELETE CASCADE,
    credential_id   BYTEA NOT NULL UNIQUE,
    public_key      BYTEA NOT NULL,
    sign_count      BIGINT NOT NULL DEFAULT 0,
    transports      TEXT[] NOT NULL DEFAULT '{}',
    label           TEXT NOT NULL,
    aaguid          BYTEA,
    wrapped_master  BYTEA NOT NULL,
    wrap_iv         BYTEA NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at    TIMESTAMPTZ
);
CREATE INDEX idx_webauthn_creds_op ON webauthn_credentials(operator_id);

-- Requests now carry the operator's wrapped per-request AES key alongside
-- the customer-supplied ciphertext + IV.
CREATE TABLE requests (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token         TEXT UNIQUE NOT NULL,
    description   TEXT NOT NULL,
    operator_id   UUID NOT NULL REFERENCES operators(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ NOT NULL,
    submitted_at  TIMESTAMPTZ,
    retrieved_at  TIMESTAMPTZ,
    ciphertext    BYTEA,
    iv            BYTEA,
    wrapped_key   BYTEA,                          -- set at create, NULLed after retrieve
    wrap_iv       BYTEA,
    status        TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'submitted', 'retrieved', 'expired'))
);
CREATE INDEX idx_requests_token    ON requests(token);
CREATE INDEX idx_requests_operator ON requests(operator_id);
CREATE INDEX idx_requests_status   ON requests(status);

-- Append-only audit trail. Never delete rows; truncate via maintenance job
-- if retention policy ever requires it.
CREATE TABLE audit_log (
    id           BIGSERIAL PRIMARY KEY,
    occurred_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor        TEXT NOT NULL,
    action       TEXT NOT NULL,
    request_id   UUID,
    operator_id  UUID,
    ip           INET,
    user_agent   TEXT,
    metadata     JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX idx_audit_request       ON audit_log(request_id);
CREATE INDEX idx_audit_operator_time ON audit_log(operator_id, occurred_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS requests;
DROP TABLE IF EXISTS webauthn_credentials;
DROP TABLE IF EXISTS operators;

-- Restore v1 requests table for symmetry with 00001 down state.
CREATE TABLE requests (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token         TEXT UNIQUE NOT NULL,
    description   TEXT NOT NULL,
    created_by    TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ NOT NULL,
    submitted_at  TIMESTAMPTZ,
    retrieved_at  TIMESTAMPTZ,
    ciphertext    BYTEA,
    iv            BYTEA,
    status        TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'submitted', 'retrieved', 'expired'))
);
CREATE INDEX idx_requests_token      ON requests(token);
CREATE INDEX idx_requests_created_by ON requests(created_by);
CREATE INDEX idx_requests_status     ON requests(status);
-- +goose StatementEnd
