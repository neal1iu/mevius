-- +goose Up
-- +goose StatementBegin

ALTER TABLE provider_connection
    ADD COLUMN auth_method TEXT NOT NULL DEFAULT 'token'
    CHECK(auth_method IN ('token', 'oauth'));

CREATE TABLE oauth_authorization_session (
    id TEXT PRIMARY KEY,
    state TEXT NOT NULL UNIQUE,
    provider_id TEXT NOT NULL,
    endpoint TEXT NOT NULL DEFAULT '',
    redirect_uri TEXT NOT NULL,
    encrypted_code_verifier TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK(status IN ('pending', 'authorized', 'consumed', 'failed')),
    encrypted_credential TEXT NOT NULL DEFAULT '',
    token_meta_json TEXT NOT NULL DEFAULT '{}',
    remote_identity_json TEXT NOT NULL DEFAULT '{}',
    scopes_json TEXT NOT NULL DEFAULT '[]',
    permissions_json TEXT NOT NULL DEFAULT '{}',
    error_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    consumed_at TEXT
);

CREATE INDEX idx_oauth_authorization_session_expiry
    ON oauth_authorization_session(expires_at);

UPDATE schema_meta SET value = '3' WHERE key = 'schema_epoch';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS oauth_authorization_session;
ALTER TABLE provider_connection DROP COLUMN auth_method;
UPDATE schema_meta SET value = '2' WHERE key = 'schema_epoch';
-- +goose StatementEnd
