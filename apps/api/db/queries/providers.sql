-- name: CreateProvider :one
INSERT INTO providers (slug, name, home_region_id, cell_id, lifecycle_state)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetProviderByID :one
SELECT * FROM providers WHERE id = $1;

-- name: ListProviders :many
SELECT * FROM providers ORDER BY created_at DESC;

-- name: UpdateProviderLifecycle :one
UPDATE providers
SET lifecycle_state = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateProviderCell :exec
UPDATE providers SET cell_id = $2, updated_at = now() WHERE id = $1;

-- name: UpsertProviderCapability :one
INSERT INTO provider_capabilities (provider_id, capability, status, granted_by, reason)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (provider_id, capability) DO UPDATE SET
    status = EXCLUDED.status,
    granted_by = CASE
        WHEN EXCLUDED.status = 'granted' THEN EXCLUDED.granted_by
        ELSE provider_capabilities.granted_by
    END,
    reason = EXCLUDED.reason,
    granted_at = CASE
        WHEN EXCLUDED.status = 'granted' AND provider_capabilities.status != 'granted' THEN now()
        ELSE provider_capabilities.granted_at
    END,
    revoked_at = CASE
        WHEN EXCLUDED.status = 'revoked' AND provider_capabilities.status != 'revoked' THEN now()
        ELSE provider_capabilities.revoked_at
    END,
    updated_at = now()
RETURNING *;

-- name: GetProviderCapability :one
SELECT * FROM provider_capabilities WHERE provider_id = $1 AND capability = $2;

-- name: ListProviderCapabilities :many
SELECT * FROM provider_capabilities WHERE provider_id = $1 ORDER BY capability;
