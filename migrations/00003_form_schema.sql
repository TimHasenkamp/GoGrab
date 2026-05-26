-- +goose Up
-- +goose StatementBegin

-- form_schema: list of fields the customer fills in. Public — labels and
-- types are by definition visible to the customer (they fill the form).
-- The VALUES stay end-to-end encrypted as before, only the structure is
-- stored in plaintext on the server.
--
-- Each entry: {"id": string, "label": string, "type": "text"|"password"|"textarea", "placeholder"?: string}
-- Default value preserves the v2 single-textarea behavior for any
-- pre-existing rows.
ALTER TABLE requests
  ADD COLUMN form_schema JSONB NOT NULL
    DEFAULT '[{"id":"secret","label":"Geheimnis","type":"textarea"}]'::jsonb;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE requests DROP COLUMN form_schema;
-- +goose StatementEnd
