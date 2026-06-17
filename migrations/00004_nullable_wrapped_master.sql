-- +goose Up
-- +goose StatementBegin

-- Some authenticator+browser combinations on Linux only evaluate hmac-secret
-- (WebAuthn PRF) during authenticatorGetAssertion, not authenticatorMakeCredential.
-- Making wrapped_master nullable lets signup save the credential first, then
-- collect PRF via a second assertion and call /signup/set-master to finalize.
ALTER TABLE webauthn_credentials
    ALTER COLUMN wrapped_master DROP NOT NULL,
    ALTER COLUMN wrap_iv DROP NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

UPDATE webauthn_credentials SET wrapped_master = '\x'::bytea WHERE wrapped_master IS NULL;
UPDATE webauthn_credentials SET wrap_iv       = '\x'::bytea WHERE wrap_iv IS NULL;
ALTER TABLE webauthn_credentials
    ALTER COLUMN wrapped_master SET NOT NULL,
    ALTER COLUMN wrap_iv SET NOT NULL;

-- +goose StatementEnd
