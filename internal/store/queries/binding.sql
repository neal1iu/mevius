-- binding queries
-- name: ListBindingsBySlot :many
SELECT * FROM binding WHERE slot_id = ? ORDER BY created_at DESC;

-- name: FanOutBindingsByAccountExternal :many
SELECT * FROM binding WHERE account_id = ? AND external_id = ?;

-- name: GetBinding :one
SELECT * FROM binding WHERE id = ?;

-- name: InsertBinding :exec
INSERT INTO binding (id, slot_id, account_id, provider, external_id, cached_meta_json, sync_status, last_synced_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateBindingSyncStatus :exec
UPDATE binding SET sync_status = ?, cached_meta_json = ?, last_synced_at = ? WHERE id = ?;

-- name: DeleteBinding :exec
DELETE FROM binding WHERE id = ?;