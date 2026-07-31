-- name: CreateCustomDomain :one
INSERT INTO custom_domains (provider_id, environment_id, domain, verification_token)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetCustomDomainByID :one
SELECT * FROM custom_domains WHERE id = $1;

-- name: GetCustomDomainByHostname :one
-- Operator-only lookup for request routing (find which provider owns a hostname).
SELECT * FROM custom_domains WHERE domain = $1 AND status = 'verified';

-- name: ListCustomDomains :many
SELECT * FROM custom_domains
WHERE provider_id = $1 AND environment_id = $2
ORDER BY created_at DESC;

-- name: VerifyCustomDomain :one
UPDATE custom_domains
SET status = 'verified', verified_at = now(), updated_at = now()
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: RevokeCustomDomain :one
UPDATE custom_domains
SET status = 'revoked', revoked_at = now(), updated_at = now()
WHERE id = $1 AND status IN ('pending', 'verified')
RETURNING *;

-- name: DeleteCustomDomain :execrows
DELETE FROM custom_domains
WHERE provider_id = $1 AND environment_id = $2 AND id = $3 AND status != 'verified';
