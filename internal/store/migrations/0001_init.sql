-- +goose Up
-- +goose StatementBegin

CREATE TABLE provider_account (
    id            TEXT PRIMARY KEY,
    provider      TEXT NOT NULL CHECK(provider IN ('cloudflare', 'github', 'vercel')),
    label         TEXT NOT NULL,
    encrypted_token TEXT NOT NULL,
    meta_json     TEXT NOT NULL DEFAULT '{}',
    created_at    TEXT NOT NULL
);

CREATE TABLE project (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL
);

CREATE TABLE slot (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
    type        TEXT NOT NULL CHECK(type IN ('repo', 'compute', 'static-site', 'dns-domain')),
    name        TEXT NOT NULL,
    config_json TEXT NOT NULL DEFAULT '{}',
    created_at  TEXT NOT NULL
);

CREATE TABLE binding (
    id              TEXT PRIMARY KEY,
    slot_id         TEXT NOT NULL REFERENCES slot(id) ON DELETE CASCADE,
    account_id      TEXT NOT NULL REFERENCES provider_account(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL,
    external_id     TEXT NOT NULL,
    cached_meta_json TEXT NOT NULL DEFAULT '{}',
    sync_status     TEXT NOT NULL DEFAULT 'never' CHECK(sync_status IN ('ok', 'error', 'auth_error', 'orphaned', 'never')),
    last_synced_at  TEXT,
    created_at      TEXT NOT NULL,
    UNIQUE(slot_id, account_id, external_id)
);

CREATE INDEX idx_binding_account_external ON binding(account_id, external_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_binding_account_external;
DROP TABLE IF EXISTS binding;
DROP TABLE IF EXISTS slot;
DROP TABLE IF EXISTS project;
DROP TABLE IF EXISTS provider_account;

-- +goose StatementEnd