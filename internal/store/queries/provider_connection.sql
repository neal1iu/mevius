-- name: InsertProviderConnection :exec
INSERT INTO provider_connection (
    id, provider_id, label, endpoint, scope_type, scope_id, scope_label, auth_method,
    config_json, encrypted_credential, remote_identity_json, permissions_json,
    permissions_checked_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListProviderConnections :many
SELECT * FROM provider_connection ORDER BY created_at DESC;

-- name: GetProviderConnection :one
SELECT * FROM provider_connection WHERE id = ?;

-- name: GetProviderConnectionCredential :one
SELECT encrypted_credential FROM provider_connection WHERE id = ?;

-- name: UpdateProviderConnectionCredential :exec
UPDATE provider_connection
SET encrypted_credential = ?, remote_identity_json = ?, permissions_json = ?,
    permissions_checked_at = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteProviderConnection :execrows
DELETE FROM provider_connection WHERE id = ?;
