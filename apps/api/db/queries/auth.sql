-- name: ListAllProviderAuthCiphertexts :many
-- Operator-only view used by the re-encryption worker.
SELECT id, zitadel_client_secret FROM provider_auth_configs
ORDER BY id
LIMIT $1;

-- name: UpdateProviderAuthClientSecret :exec
UPDATE provider_auth_configs SET zitadel_client_secret = $2, updated_at = now()
WHERE id = $1;

-- name: CreateAuthConfig :one
INSERT INTO provider_auth_configs (
    provider_id, environment_id, name,
    zitadel_project_id, zitadel_app_id, zitadel_client_id, zitadel_client_secret,
    redirect_uris
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetAuthConfigByTenant :one
SELECT * FROM provider_auth_configs
WHERE provider_id = $1 AND environment_id = $2;

-- name: ListAuthConfigsByTenant :many
SELECT * FROM provider_auth_configs
WHERE provider_id = $1 AND environment_id = $2
ORDER BY created_at DESC;

-- name: UpdateAuthConfigSecret :one
UPDATE provider_auth_configs
SET zitadel_client_secret = $3, updated_at = now()
WHERE provider_id = $1 AND environment_id = $2
RETURNING *;

-- name: UpdateAuthConfigRedirectURIs :one
UPDATE provider_auth_configs
SET redirect_uris = $3, updated_at = now()
WHERE provider_id = $1 AND environment_id = $2
RETURNING *;

-- name: DeleteAuthConfig :exec
DELETE FROM provider_auth_configs
WHERE provider_id = $1 AND environment_id = $2;
