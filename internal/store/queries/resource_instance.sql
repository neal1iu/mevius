-- name: InsertResourceInstance :exec
INSERT INTO resource_instance (
    id, connection_id, provider_product_id, resource_kind, external_id,
    external_url, display_name, lifecycle_mode, spec_json, provider_config_json,
    cached_meta_json, sync_status, capability_state_json, last_synced_at,
    created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetResourceInstance :one
SELECT * FROM resource_instance WHERE id = ?;

-- name: GetResourceInstanceByRemote :one
SELECT * FROM resource_instance
WHERE connection_id = ? AND provider_product_id = ? AND external_id = ?;

-- name: ListResourceInstances :many
SELECT * FROM resource_instance ORDER BY created_at DESC;

-- name: ListResourceInstancesByConnection :many
SELECT * FROM resource_instance WHERE connection_id = ? ORDER BY created_at DESC;

-- name: CountResourceInstancesByConnection :one
SELECT COUNT(*) FROM resource_instance WHERE connection_id = ?;

-- name: UpdateResourceInstanceSync :exec
UPDATE resource_instance
SET external_url = ?, display_name = ?, cached_meta_json = ?, sync_status = ?,
    capability_state_json = ?, last_synced_at = ?, updated_at = ?
WHERE id = ?;

-- name: UpdateResourceInstanceLifecycle :exec
UPDATE resource_instance SET lifecycle_mode = ?, updated_at = ? WHERE id = ?;

-- name: DeleteResourceInstance :execrows
DELETE FROM resource_instance WHERE id = ?;
