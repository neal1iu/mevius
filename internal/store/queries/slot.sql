-- slot queries
-- name: ListSlotsByProject :many
SELECT * FROM slot WHERE project_id = ? ORDER BY created_at DESC;

-- name: GetSlot :one
SELECT * FROM slot WHERE id = ?;

-- name: InsertSlot :exec
INSERT INTO slot (id, project_id, role, name, config_json, created_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: UpdateSlotConfig :exec
UPDATE slot SET config_json = ? WHERE id = ?;

-- name: DeleteSlot :exec
DELETE FROM slot WHERE id = ?;