-- project queries
-- name: ListProjects :many
SELECT * FROM project ORDER BY created_at DESC;

-- name: GetProject :one
SELECT * FROM project WHERE id = ?;

-- name: GetProjectByName :one
SELECT * FROM project WHERE name = ?;

-- name: InsertProject :exec
INSERT INTO project (id, name, description, created_at, updated_at)
VALUES (?, ?, ?, ?, ?);

-- name: UpdateProject :exec
UPDATE project SET name = ?, description = ?, updated_at = ? WHERE id = ?;

-- name: DeleteProject :exec
DELETE FROM project WHERE id = ?;
