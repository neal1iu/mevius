-- name: InsertProjectResource :exec
INSERT INTO project_resource (
    id, project_id, resource_instance_id, alias, purpose, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetProjectResource :one
SELECT * FROM project_resource WHERE id = ?;

-- name: ListProjectResources :many
SELECT * FROM project_resource WHERE project_id = ? ORDER BY created_at ASC;

-- name: CountProjectResourcesByInstance :one
SELECT COUNT(*) FROM project_resource WHERE resource_instance_id = ?;

-- name: UpdateProjectResource :exec
UPDATE project_resource SET alias = ?, purpose = ?, updated_at = ? WHERE id = ?;

-- name: DeleteProjectResource :execrows
DELETE FROM project_resource WHERE id = ? AND project_id = ?;
