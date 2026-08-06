-- Server-side OIDC token vault for vLogBin web sessions.

-- name: CreateAuthVault :one
INSERT INTO auth_session_vault (
    id, user_sub, email, name, roles, workspace_id, env,
    access_token, refresh_token, token_exp, expires_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetAuthVault :one
SELECT *
FROM auth_session_vault
WHERE id = $1 AND expires_at > now();

-- name: DeleteAuthVault :exec
DELETE FROM auth_session_vault WHERE id = $1;

-- name: DeleteExpiredAuthVaults :exec
DELETE FROM auth_session_vault WHERE expires_at <= now();
