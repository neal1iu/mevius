-- name: InsertResourceRelation :exec
INSERT INTO resource_relation (
    id, from_resource_instance_id, to_resource_instance_id,
    relation_type, origin, config_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetResourceRelation :one
SELECT * FROM resource_relation WHERE id = ?;

-- name: GetSourceRelation :one
SELECT * FROM resource_relation
WHERE from_resource_instance_id = ? AND relation_type = 'source_repo';

-- name: ListResourceRelations :many
SELECT * FROM resource_relation ORDER BY created_at ASC;

-- name: ListRelationsFrom :many
SELECT * FROM resource_relation WHERE from_resource_instance_id = ? ORDER BY created_at ASC;

-- name: ListRelationsTo :many
SELECT * FROM resource_relation WHERE to_resource_instance_id = ? ORDER BY created_at ASC;

-- name: CountRelationsTo :one
SELECT COUNT(*) FROM resource_relation WHERE to_resource_instance_id = ?;

-- name: DeleteResourceRelation :execrows
DELETE FROM resource_relation WHERE id = ?;
