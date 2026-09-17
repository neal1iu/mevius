-- +goose Up
-- +goose StatementBegin

CREATE TABLE provider_connection (
    id            TEXT PRIMARY KEY,
    provider      TEXT NOT NULL CHECK(provider IN ('cloudflare', 'github', 'vercel')),
    label         TEXT NOT NULL,
    endpoint      TEXT NOT NULL DEFAULT '',
    config_json   TEXT NOT NULL DEFAULT '{}',
    encrypted_credential TEXT NOT NULL DEFAULT '',
    remote_identity_json TEXT NOT NULL DEFAULT '{}',
    created_at    TEXT NOT NULL
);

INSERT INTO provider_connection (id, provider, label, endpoint, config_json, encrypted_credential, remote_identity_json, created_at)
SELECT id, provider, label, '', '{}', encrypted_token, meta_json, created_at FROM provider_account;

DROP TABLE provider_account;

CREATE TABLE binding_new (
    id              TEXT PRIMARY KEY,
    slot_id         TEXT NOT NULL REFERENCES slot(id) ON DELETE CASCADE,
    connection_id   TEXT NOT NULL REFERENCES provider_connection(id) ON DELETE CASCADE,
    product         TEXT NOT NULL DEFAULT '',
    external_id     TEXT NOT NULL,
    cached_meta_json TEXT NOT NULL DEFAULT '{}',
    sync_status     TEXT NOT NULL DEFAULT 'never' CHECK(sync_status IN ('ok', 'error', 'auth_error', 'orphaned', 'never')),
    last_synced_at  TEXT,
    created_at      TEXT NOT NULL,
    UNIQUE(slot_id)
);

INSERT INTO binding_new (id, slot_id, connection_id, product, external_id, cached_meta_json, sync_status, last_synced_at, created_at)
SELECT id, slot_id, account_id, '', external_id, cached_meta_json, sync_status, last_synced_at, created_at FROM binding;

DROP TABLE binding;
ALTER TABLE binding_new RENAME TO binding;

CREATE TABLE slot_new (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
    role        TEXT NOT NULL,
    name        TEXT NOT NULL,
    config_json TEXT NOT NULL DEFAULT '{}',
    created_at  TEXT NOT NULL
);

INSERT INTO slot_new (id, project_id, role, name, config_json, created_at)
SELECT id, project_id, type, name, config_json, created_at FROM slot;

DROP TABLE slot;
ALTER TABLE slot_new RENAME TO slot;

DROP INDEX IF EXISTS idx_binding_account_external;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

CREATE TABLE slot_old (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
    type        TEXT NOT NULL,
    name        TEXT NOT NULL,
    config_json TEXT NOT NULL DEFAULT '{}',
    created_at  TEXT NOT NULL
);

INSERT INTO slot_old SELECT id, project_id, role, name, config_json, created_at FROM slot;
DROP TABLE slot;
ALTER TABLE slot_old RENAME TO slot;

CREATE TABLE binding_old (
    id              TEXT PRIMARY KEY,
    slot_id         TEXT NOT NULL REFERENCES slot(id) ON DELETE CASCADE,
    account_id      TEXT NOT NULL,
    provider        TEXT NOT NULL,
    external_id     TEXT NOT NULL,
    cached_meta_json TEXT NOT NULL DEFAULT '{}',
    sync_status     TEXT NOT NULL DEFAULT 'never',
    last_synced_at  TEXT,
    created_at      TEXT NOT NULL,
    UNIQUE(slot_id, account_id, external_id)
);

INSERT INTO binding_old (id, slot_id, account_id, provider, external_id, cached_meta_json, sync_status, last_synced_at, created_at)
SELECT id, slot_id, connection_id, '', external_id, cached_meta_json, sync_status, last_synced_at, created_at FROM binding;
DROP TABLE binding;
ALTER TABLE binding_old RENAME TO binding;

CREATE INDEX idx_binding_account_external ON binding(account_id, external_id);

CREATE TABLE provider_account (
    id            TEXT PRIMARY KEY,
    provider      TEXT NOT NULL,
    label         TEXT NOT NULL,
    encrypted_token TEXT NOT NULL,
    meta_json     TEXT NOT NULL DEFAULT '{}',
    created_at    TEXT NOT NULL
);

INSERT INTO provider_account (id, provider, label, encrypted_token, meta_json, created_at)
SELECT id, provider, label, encrypted_credential, remote_identity_json, created_at FROM provider_connection;

DROP TABLE provider_connection;

-- +goose StatementEnd