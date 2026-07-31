-- name: CreateAuthConfig :one
INSERT INTO provider_auth_configs (
    provider_id, environment_id,
    zitadel_project_id, zitadel_app_id, zitadel_client_id, zitadel_client_secret
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetAuthConfigByTenant :one
SELECT * FROM provider_auth_configs
WHERE provider_id = $1 AND environment_id = $2;

-- name: DeleteAuthConfig :exec
DELETE FROM provider_auth_configs
WHERE provider_id = $1 AND environment_id = $2;
