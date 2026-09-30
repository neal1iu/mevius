-- +goose Up
CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO schema_meta VALUES ('schema_epoch','6');
CREATE TABLE provider_instance (
 id TEXT PRIMARY KEY, provider_id TEXT NOT NULL, instance_key TEXT NOT NULL CHECK(length(instance_key)>0),
 endpoint_version INTEGER NOT NULL CHECK(endpoint_version>0), endpoint_json TEXT NOT NULL CHECK(json_valid(endpoint_json)),
 created_at INTEGER NOT NULL CHECK(created_at>=0), UNIQUE(provider_id,instance_key)
);
CREATE TABLE provider_connection (
 id TEXT PRIMARY KEY, provider_instance_id TEXT NOT NULL REFERENCES provider_instance(id), label TEXT NOT NULL,
 auth_scheme TEXT NOT NULL CHECK(auth_scheme IN ('token','oauth')), encrypted_credential TEXT NOT NULL,
 credential_revision INTEGER NOT NULL CHECK(credential_revision>0), principal_version INTEGER NOT NULL CHECK(principal_version>0),
 principal_json TEXT NOT NULL CHECK(json_valid(principal_json)), authorization_state TEXT NOT NULL CHECK(authorization_state IN ('valid','invalid','unknown')),
 created_at INTEGER NOT NULL CHECK(created_at>=0), updated_at INTEGER NOT NULL CHECK(updated_at>=0)
);
CREATE TABLE provider_scope (
 id TEXT PRIMARY KEY, provider_instance_id TEXT NOT NULL REFERENCES provider_instance(id), scope_type TEXT NOT NULL,
 scope_key TEXT NOT NULL CHECK(json_valid(scope_key) AND json_type(scope_key)='array'), locator_version INTEGER NOT NULL CHECK(locator_version>0),
 locator_json TEXT NOT NULL CHECK(json_valid(locator_json)), label TEXT NOT NULL,
 parent_scope_id TEXT REFERENCES provider_scope(id) ON DELETE SET NULL, UNIQUE(provider_instance_id,scope_type,scope_key),
 UNIQUE(id,provider_instance_id)
);
-- +goose StatementBegin
CREATE TRIGGER scope_parent_instance_insert BEFORE INSERT ON provider_scope WHEN NEW.parent_scope_id IS NOT NULL AND
 (SELECT provider_instance_id FROM provider_scope WHERE id=NEW.parent_scope_id)<>NEW.provider_instance_id
 BEGIN SELECT RAISE(ABORT,'parent scope instance mismatch'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER scope_parent_instance_update BEFORE UPDATE ON provider_scope WHEN NEW.parent_scope_id IS NOT NULL AND
 (SELECT provider_instance_id FROM provider_scope WHERE id=NEW.parent_scope_id)<>NEW.provider_instance_id
 BEGIN SELECT RAISE(ABORT,'parent scope instance mismatch'); END;
-- +goose StatementEnd
CREATE TABLE connection_scope (
 id TEXT PRIMARY KEY, connection_id TEXT NOT NULL REFERENCES provider_connection(id) ON DELETE CASCADE,
 scope_id TEXT NOT NULL REFERENCES provider_scope(id), context_version INTEGER NOT NULL CHECK(context_version>0),
 context_json TEXT NOT NULL CHECK(json_valid(context_json)), validation_state TEXT NOT NULL CHECK(validation_state IN ('valid','invalid','unknown')),
 validated_at INTEGER CHECK(validated_at>=0), UNIQUE(connection_id,scope_id)
);
-- +goose StatementBegin
CREATE TRIGGER connection_scope_instance_insert BEFORE INSERT ON connection_scope WHEN
 (SELECT provider_instance_id FROM provider_connection WHERE id=NEW.connection_id) <> (SELECT provider_instance_id FROM provider_scope WHERE id=NEW.scope_id)
 BEGIN SELECT RAISE(ABORT,'connection and scope instance mismatch'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER connection_scope_instance_update BEFORE UPDATE ON connection_scope WHEN
 (SELECT provider_instance_id FROM provider_connection WHERE id=NEW.connection_id) <> (SELECT provider_instance_id FROM provider_scope WHERE id=NEW.scope_id)
 BEGIN SELECT RAISE(ABORT,'connection and scope instance mismatch'); END;
-- +goose StatementEnd
CREATE TABLE resource_instance (
 id TEXT PRIMARY KEY, provider_instance_id TEXT NOT NULL REFERENCES provider_instance(id), resource_type_id TEXT NOT NULL,
 identity_key TEXT NOT NULL CHECK(json_valid(identity_key) AND json_type(identity_key)='array'), identity_version INTEGER NOT NULL CHECK(identity_version>0),
 scope_id TEXT NOT NULL, locator_version INTEGER NOT NULL CHECK(locator_version>0), locator_json TEXT NOT NULL CHECK(json_valid(locator_json)),
 display_name TEXT NOT NULL, origin TEXT NOT NULL CHECK(origin IN ('external','mevius','unknown')),
 delete_protection INTEGER NOT NULL CHECK(delete_protection IN (0,1)), created_at INTEGER NOT NULL CHECK(created_at>=0), updated_at INTEGER NOT NULL CHECK(updated_at>=0),
 FOREIGN KEY(scope_id,provider_instance_id) REFERENCES provider_scope(id,provider_instance_id),
 UNIQUE(provider_instance_id,resource_type_id,identity_key)
);
CREATE TABLE resource_access (
 id TEXT PRIMARY KEY, resource_id TEXT NOT NULL REFERENCES resource_instance(id) ON DELETE CASCADE,
 connection_scope_id TEXT NOT NULL REFERENCES connection_scope(id) ON DELETE CASCADE,
 credential_revision INTEGER NOT NULL CHECK(credential_revision>0), observation_version INTEGER NOT NULL CHECK(observation_version>0),
 observation_json TEXT NOT NULL CHECK(json_valid(observation_json)), capabilities_json TEXT NOT NULL CHECK(json_valid(capabilities_json)),
 last_attempt_at INTEGER NOT NULL CHECK(last_attempt_at>=0), last_success_at INTEGER CHECK(last_success_at>=0),
 error_code TEXT NOT NULL DEFAULT '', UNIQUE(resource_id,connection_scope_id)
);
-- +goose StatementBegin
CREATE TRIGGER resource_access_instance_insert BEFORE INSERT ON resource_access WHEN
 (SELECT provider_instance_id FROM resource_instance WHERE id=NEW.resource_id) <> (SELECT s.provider_instance_id FROM provider_scope s JOIN connection_scope c ON c.scope_id=s.id WHERE c.id=NEW.connection_scope_id)
 BEGIN SELECT RAISE(ABORT,'resource and access instance mismatch'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER resource_access_instance_update BEFORE UPDATE ON resource_access WHEN
 (SELECT provider_instance_id FROM resource_instance WHERE id=NEW.resource_id) <> (SELECT s.provider_instance_id FROM provider_scope s JOIN connection_scope c ON c.scope_id=s.id WHERE c.id=NEW.connection_scope_id)
 BEGIN SELECT RAISE(ABORT,'resource and access instance mismatch'); END;
-- +goose StatementEnd
CREATE TABLE resource_reference (
 id TEXT PRIMARY KEY, provider_id TEXT NOT NULL, provider_instance_id TEXT REFERENCES provider_instance(id), resource_type_id TEXT NOT NULL,
 reference_version INTEGER NOT NULL CHECK(reference_version>0), reference_json TEXT NOT NULL CHECK(json_valid(reference_json)),
 identity_key TEXT CHECK(identity_key IS NULL OR (json_valid(identity_key) AND json_type(identity_key)='array')),
 resolved_resource_id TEXT REFERENCES resource_instance(id) ON DELETE SET NULL, created_at INTEGER NOT NULL CHECK(created_at>=0)
);
CREATE TABLE resource_relation (
 id TEXT PRIMARY KEY, from_resource_id TEXT NOT NULL REFERENCES resource_instance(id) ON DELETE CASCADE,
 reference_id TEXT NOT NULL REFERENCES resource_reference(id), relation_type TEXT NOT NULL,
 origin TEXT NOT NULL CHECK(origin IN ('adapter','user')), observed_access_id TEXT REFERENCES resource_access(id) ON DELETE SET NULL,
 attributes_version INTEGER NOT NULL CHECK(attributes_version>0), attributes_json TEXT NOT NULL CHECK(json_valid(attributes_json)),
 blocks_deletion INTEGER NOT NULL CHECK(blocks_deletion IN (0,1)), created_at INTEGER NOT NULL CHECK(created_at>=0),
 UNIQUE(from_resource_id,reference_id,relation_type,origin)
);
CREATE TABLE project (id TEXT PRIMARY KEY,name TEXT NOT NULL UNIQUE,description TEXT NOT NULL DEFAULT '',created_at INTEGER NOT NULL CHECK(created_at>=0),updated_at INTEGER NOT NULL CHECK(updated_at>=0));
CREATE TABLE project_resource (
 id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
 resource_id TEXT NOT NULL REFERENCES resource_instance(id) ON DELETE RESTRICT, alias TEXT NOT NULL CHECK(length(alias)>0),
 role TEXT NOT NULL CHECK(length(role)>0),purpose TEXT NOT NULL DEFAULT '',environment TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL CHECK(created_at>=0),updated_at INTEGER NOT NULL CHECK(updated_at>=0),UNIQUE(project_id,alias)
);
CREATE TABLE operation_request (
 id TEXT PRIMARY KEY, idempotency_key TEXT NOT NULL UNIQUE CHECK(length(idempotency_key)>0), request_fingerprint TEXT NOT NULL,
 action_id TEXT NOT NULL,resource_type_id TEXT NOT NULL,target_kind TEXT NOT NULL CHECK(target_kind IN ('scope','resource')),
 target_id TEXT NOT NULL,access_id TEXT NOT NULL,connection_id TEXT NOT NULL,credential_revision INTEGER NOT NULL CHECK(credential_revision>0),
 snapshot_version INTEGER NOT NULL CHECK(snapshot_version>0), snapshot_json TEXT NOT NULL CHECK(json_valid(snapshot_json)),
 status TEXT NOT NULL CHECK(status IN ('in_progress','succeeded','failed','unknown')),
 remote_operation_id TEXT NOT NULL DEFAULT '', result_version INTEGER NOT NULL CHECK(result_version>0),result_json TEXT NOT NULL CHECK(json_valid(result_json)),
 error_code TEXT NOT NULL DEFAULT '',result_resource_id TEXT NOT NULL DEFAULT '',created_at INTEGER NOT NULL CHECK(created_at>=0),updated_at INTEGER NOT NULL CHECK(updated_at>=0)
);
CREATE INDEX operation_active_connection ON operation_request(connection_id,status);
CREATE INDEX project_resource_resource ON project_resource(resource_id);
CREATE INDEX relation_reference ON resource_relation(reference_id);
CREATE INDEX resource_access_binding ON resource_access(connection_scope_id);
CREATE INDEX resource_relation_access ON resource_relation(observed_access_id);
CREATE INDEX resource_reference_resolved ON resource_reference(resolved_resource_id);
CREATE INDEX resource_reference_identity ON resource_reference(provider_instance_id,resource_type_id,identity_key);
CREATE INDEX provider_scope_parent ON provider_scope(parent_scope_id);
CREATE INDEX resource_instance_scope ON resource_instance(scope_id);
CREATE TABLE oauth_client_configuration (
 id TEXT PRIMARY KEY,provider_id TEXT NOT NULL UNIQUE,client_id TEXT NOT NULL,encrypted_client_secret TEXT NOT NULL,
 authorization_url TEXT NOT NULL,token_url TEXT NOT NULL,scopes_json TEXT NOT NULL CHECK(json_valid(scopes_json)),pkce INTEGER NOT NULL CHECK(pkce IN (0,1)),
 redirect_base_url TEXT NOT NULL,created_at INTEGER NOT NULL CHECK(created_at>=0),updated_at INTEGER NOT NULL CHECK(updated_at>=0)
);
CREATE TABLE oauth_authorization_session (
 id TEXT PRIMARY KEY,state TEXT NOT NULL UNIQUE,provider_instance_id TEXT NOT NULL REFERENCES provider_instance(id),redirect_uri TEXT NOT NULL,
 encrypted_code_verifier TEXT NOT NULL,status TEXT NOT NULL CHECK(status IN ('pending','authorized','consumed','failed')),
 encrypted_credential TEXT NOT NULL DEFAULT '',error_code TEXT NOT NULL DEFAULT '',created_at INTEGER NOT NULL CHECK(created_at>=0),updated_at INTEGER NOT NULL CHECK(updated_at>=0),expires_at INTEGER NOT NULL CHECK(expires_at>=0)
);
-- +goose Down
-- The release baseline is irreversible. Reset is an explicit administrative action.
