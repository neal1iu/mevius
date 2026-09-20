-- name: InsertOAuthAuthorizationSession :exec
INSERT INTO oauth_authorization_session (
    id, state, provider_id, endpoint, redirect_uri, encrypted_code_verifier,
    status, encrypted_credential, token_meta_json, remote_identity_json,
    scopes_json, permissions_json, error_json, created_at, updated_at, expires_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetOAuthAuthorizationSession :one
SELECT * FROM oauth_authorization_session WHERE id = ?;

-- name: GetOAuthAuthorizationSessionByState :one
SELECT * FROM oauth_authorization_session WHERE state = ?;

-- name: AuthorizeOAuthSession :execrows
UPDATE oauth_authorization_session
SET status = 'authorized', encrypted_credential = ?, token_meta_json = ?,
    remote_identity_json = ?, scopes_json = ?, permissions_json = ?,
    error_json = '{}', updated_at = ?
WHERE id = ? AND status = 'pending' AND expires_at > ?;

-- name: FailOAuthSession :execrows
UPDATE oauth_authorization_session
SET status = 'failed', error_json = ?, updated_at = ?
WHERE id = ? AND status = 'pending';

-- name: ConsumeOAuthSession :execrows
UPDATE oauth_authorization_session
SET status = 'consumed', consumed_at = ?, updated_at = ?
WHERE id = ? AND status = 'authorized' AND expires_at > ?;

-- name: DeleteExpiredOAuthSessions :exec
DELETE FROM oauth_authorization_session
WHERE expires_at < ?;
