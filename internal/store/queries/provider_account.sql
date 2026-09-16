-- provider_account queries
-- name: ListProviderAccounts :many
SELECT * FROM provider_account ORDER BY created_at DESC;

-- name: GetProviderAccount :one
SELECT * FROM provider_account WHERE id = ?;

-- name: InsertProviderAccount :exec
INSERT INTO provider_account (id, provider, label, encrypted_token, meta_json, created_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: DeleteProviderAccount :exec
DELETE FROM provider_account WHERE id = ?;