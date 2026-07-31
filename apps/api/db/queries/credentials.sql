-- name: CreateCredential :one
INSERT INTO credentials (provider_id, environment_id, name, key_prefix, key_hash, scopes, allowed_cidrs, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ResolveCredentialByKeyHash :one
SELECT c.id AS credential_id,
       c.provider_id,
       c.environment_id,
       c.name,
       c.key_prefix,
       c.scopes,
       c.allowed_cidrs,
       c.expires_at,
       c.revoked_at,
       p.slug AS provider_slug,
       p.lifecycle_state,
       e.kind AS environment_kind,
       e.issuer
FROM credentials c
JOIN providers p ON p.id = c.provider_id
JOIN environments e ON e.id = c.environment_id
WHERE c.key_hash = $1;

-- name: ListCredentialsByEnvironment :many
SELECT * FROM credentials
WHERE provider_id = $1 AND environment_id = $2
ORDER BY created_at DESC;

-- name: GetCredentialByID :one
SELECT * FROM credentials WHERE id = $1;

-- name: RevokeCredential :one
UPDATE credentials SET revoked_at = now() WHERE id = $1 RETURNING *;

-- name: UpdateCredentialScopes :one
UPDATE credentials SET scopes = $2 WHERE id = $1 AND revoked_at IS NULL RETURNING *;

-- name: TouchCredentialLastUsed :exec
UPDATE credentials SET last_used_at = now() WHERE id = $1;
