-- +goose Up
-- +goose StatementBegin

CREATE TABLE oauth_client_configuration (
    provider_id TEXT PRIMARY KEY,
    client_id TEXT NOT NULL,
    encrypted_client_secret TEXT NOT NULL,
    authorization_url TEXT NOT NULL,
    token_url TEXT NOT NULL,
    scopes_json TEXT NOT NULL DEFAULT '[]',
    pkce INTEGER NOT NULL DEFAULT 0 CHECK(pkce IN (0, 1)),
    redirect_base_url TEXT NOT NULL,
    provider_config_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

UPDATE schema_meta SET value = '4' WHERE key = 'schema_epoch';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS oauth_client_configuration;
UPDATE schema_meta SET value = '3' WHERE key = 'schema_epoch';
-- +goose StatementEnd
