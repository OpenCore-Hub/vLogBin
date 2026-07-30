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
