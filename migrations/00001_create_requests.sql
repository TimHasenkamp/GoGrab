-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

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

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS requests;
-- +goose StatementEnd
