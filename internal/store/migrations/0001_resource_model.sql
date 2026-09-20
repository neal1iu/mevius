-- +goose Up
-- +goose StatementBegin

CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO schema_meta (key, value) VALUES ('schema_epoch', '2');

CREATE TABLE project (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE provider_connection (
    id TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL,
    label TEXT NOT NULL,
    endpoint TEXT NOT NULL DEFAULT '',
    scope_type TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    scope_label TEXT NOT NULL,
    config_json TEXT NOT NULL DEFAULT '{}',
    encrypted_credential TEXT NOT NULL,
    remote_identity_json TEXT NOT NULL DEFAULT '{}',
    permissions_json TEXT NOT NULL DEFAULT '{}',
    permissions_checked_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(provider_id, endpoint, scope_type, scope_id)
);

CREATE TABLE resource_instance (
    id TEXT PRIMARY KEY,
    connection_id TEXT NOT NULL REFERENCES provider_connection(id) ON DELETE RESTRICT,
    provider_product_id TEXT NOT NULL,
    resource_kind TEXT NOT NULL CHECK(resource_kind IN ('git_repo', 'ci_pipeline', 'page', 'serverless_service', 'dns_zone')),
    external_id TEXT NOT NULL,
    external_url TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL,
    lifecycle_mode TEXT NOT NULL CHECK(lifecycle_mode IN ('managed', 'imported')),
    spec_json TEXT NOT NULL DEFAULT '{}',
    provider_config_json TEXT NOT NULL DEFAULT '{"version":1}',
    cached_meta_json TEXT NOT NULL DEFAULT '{}',
    sync_status TEXT NOT NULL DEFAULT 'never' CHECK(sync_status IN ('ok', 'error', 'auth_error', 'orphaned', 'never')),
    capability_state_json TEXT NOT NULL DEFAULT '{}',
    last_synced_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(connection_id, provider_product_id, external_id)
);

CREATE INDEX idx_resource_instance_connection ON resource_instance(connection_id);
CREATE INDEX idx_resource_instance_product ON resource_instance(provider_product_id);
CREATE INDEX idx_resource_instance_kind ON resource_instance(resource_kind);

CREATE TABLE project_resource (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
    resource_instance_id TEXT NOT NULL REFERENCES resource_instance(id) ON DELETE RESTRICT,
    alias TEXT NOT NULL,
    purpose TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(project_id, alias),
    UNIQUE(project_id, resource_instance_id)
);
CREATE INDEX idx_project_resource_instance ON project_resource(resource_instance_id);

CREATE TABLE resource_relation (
    id TEXT PRIMARY KEY,
    from_resource_instance_id TEXT NOT NULL REFERENCES resource_instance(id) ON DELETE CASCADE,
    to_resource_instance_id TEXT NOT NULL REFERENCES resource_instance(id) ON DELETE RESTRICT,
    relation_type TEXT NOT NULL CHECK(relation_type IN ('source_repo', 'deploys_to')),
    origin TEXT NOT NULL CHECK(origin IN ('system', 'user')),
    config_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    CHECK(from_resource_instance_id <> to_resource_instance_id),
    UNIQUE(from_resource_instance_id, to_resource_instance_id, relation_type)
);
CREATE UNIQUE INDEX idx_resource_relation_single_source
    ON resource_relation(from_resource_instance_id) WHERE relation_type = 'source_repo';
CREATE INDEX idx_resource_relation_to ON resource_relation(to_resource_instance_id);

CREATE TABLE operation_request (
    id TEXT PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE,
    operation_type TEXT NOT NULL CHECK(operation_type IN ('create_resource', 'delete_resource')),
    request_hash TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('in_progress', 'succeeded', 'failed', 'unknown')),
    resource_instance_id TEXT REFERENCES resource_instance(id) ON DELETE SET NULL,
    response_json TEXT NOT NULL DEFAULT '{}',
    error_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    expires_at TEXT
);
CREATE INDEX idx_operation_request_expiry ON operation_request(expires_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS operation_request;
DROP TABLE IF EXISTS resource_relation;
DROP TABLE IF EXISTS project_resource;
DROP TABLE IF EXISTS resource_instance;
DROP TABLE IF EXISTS provider_connection;
DROP TABLE IF EXISTS project;
DROP TABLE IF EXISTS schema_meta;
-- +goose StatementEnd
