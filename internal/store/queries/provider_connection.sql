-- provider_connection queries
-- name: ListProviderConnections :many
SELECT * FROM provider_connection ORDER BY created_at DESC;

-- name: GetProviderConnection :one
SELECT * FROM provider_connection WHERE id = ?;

-- name: InsertProviderConnection :exec
INSERT INTO provider_connection (id, provider, label, endpoint, config_json, encrypted_credential, remote_identity_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: DeleteProviderConnection :exec
DELETE FROM provider_connection WHERE id = ?;