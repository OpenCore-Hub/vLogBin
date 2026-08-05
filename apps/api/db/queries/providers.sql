-- name: CreateProvider :one
INSERT INTO providers (slug, name, home_region_id, cell_id, lifecycle_state)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CreateRegisteredProvider :one
-- Signup-time provider record (design baseline §2.1): workspace_id maps 1:1
-- to provider_id, so the caller passes the workspace id as the provider id.
-- No region or cell yet; both are assigned by the operator at activation
-- (REGISTERED → TEST_ACTIVE).
INSERT INTO providers (id, slug, name, lifecycle_state)
VALUES ($1, $2, $3, 'REGISTERED')
RETURNING *;

-- name: GetProviderByID :one
SELECT * FROM providers WHERE id = $1;

-- name: ListProviders :many
SELECT * FROM providers ORDER BY created_at DESC;

-- name: UpdateProviderLifecycle :execrows
-- Optimistic concurrency guard (design baseline §2.1): the transition is
-- conditional on the observed source state, so two concurrent transitions
-- cannot silently overwrite each other. A stale caller (whose read happened
-- before another transition committed) matches no row and receives affected=0
-- instead of corrupting state; the service maps that to a lifecycle_conflict.
UPDATE providers
SET lifecycle_state = sqlc.arg(to_state), updated_at = now()
WHERE id = sqlc.arg(id) AND lifecycle_state = sqlc.arg(from_state);

-- name: ActivateProvider :one
-- Operator activation (design baseline §2.1): assigns the home region and
-- shared cell, then moves the provider from REGISTERED to TEST_ACTIVE.
-- The WHERE guard makes activation concurrency-safe: a provider that is not
-- REGISTERED (already activated, suspended, or gone) matches no row and the
-- caller receives ErrNoRows instead of silently corrupting state.
UPDATE providers
SET home_region_id = $2, cell_id = $3, lifecycle_state = $4, updated_at = now()
WHERE id = $1 AND lifecycle_state = 'REGISTERED'
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
