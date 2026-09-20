-- name: GetOAuthClientConfiguration :one
SELECT * FROM oauth_client_configuration WHERE provider_id = ?;

-- name: ListOAuthClientConfigurations :many
SELECT * FROM oauth_client_configuration ORDER BY provider_id;

-- name: UpsertOAuthClientConfiguration :exec
INSERT INTO oauth_client_configuration (
    provider_id, client_id, encrypted_client_secret, authorization_url,
    token_url, scopes_json, pkce, redirect_base_url, provider_config_json,
    created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(provider_id) DO UPDATE SET
    client_id = excluded.client_id,
    encrypted_client_secret = excluded.encrypted_client_secret,
    authorization_url = excluded.authorization_url,
    token_url = excluded.token_url,
    scopes_json = excluded.scopes_json,
    pkce = excluded.pkce,
    redirect_base_url = excluded.redirect_base_url,
    provider_config_json = excluded.provider_config_json,
    updated_at = excluded.updated_at;

-- name: DeleteOAuthClientConfiguration :execrows
DELETE FROM oauth_client_configuration WHERE provider_id = ?;
