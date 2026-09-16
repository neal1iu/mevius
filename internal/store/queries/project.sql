-- project queries
-- name: ListProjects :many
SELECT * FROM project ORDER BY created_at DESC;

-- name: GetProject :one
SELECT * FROM project WHERE id = ?;

-- name: InsertProject :exec
INSERT INTO project (id, name, description, created_at, updated_at)
VALUES (?, ?, ?, ?, ?);

-- name: DeleteProject :exec
DELETE FROM project WHERE id = ?;