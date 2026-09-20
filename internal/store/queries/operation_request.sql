-- name: InsertOperationRequest :exec
INSERT INTO operation_request (
    id, idempotency_key, operation_type, request_hash, status,
    resource_instance_id, response_json, error_json, created_at, updated_at, expires_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetOperationRequestByKey :one
SELECT * FROM operation_request WHERE idempotency_key = ?;

-- name: GetOperationRequest :one
SELECT * FROM operation_request WHERE id = ?;

-- name: UpdateOperationRequest :exec
UPDATE operation_request
SET status = ?, resource_instance_id = ?, response_json = ?, error_json = ?,
    updated_at = ?, expires_at = ?
WHERE id = ?;

-- name: DeleteExpiredOperationRequests :exec
DELETE FROM operation_request
WHERE expires_at IS NOT NULL AND expires_at < ? AND status <> 'unknown';
