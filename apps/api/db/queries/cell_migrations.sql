-- name: CreateCellMigration :one
INSERT INTO cell_migrations (provider_id, from_cell_id, to_cell_id, reason, initiated_by, scheduled_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetCellMigrationByID :one
SELECT * FROM cell_migrations WHERE id = $1;

-- name: ListCellMigrationsByProvider :many
SELECT * FROM cell_migrations
WHERE provider_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: UpdateCellMigrationStatus :one
UPDATE cell_migrations
SET status = $2,
    started_at = CASE WHEN $2 = 'migrating' THEN now() ELSE started_at END,
    completed_at = CASE WHEN $2 IN ('completed', 'failed', 'cancelled') THEN now() ELSE completed_at END,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetCellMigrationPrecheck :exec
UPDATE cell_migrations
SET precheck_passed = $2, data_integrity_hash = $3, record_count = $4, updated_at = now()
WHERE id = $1;

-- name: SetCellMigrationError :exec
UPDATE cell_migrations
SET error_message = $2, updated_at = now()
WHERE id = $1;

-- name: GetActiveCellMigration :one
SELECT * FROM cell_migrations
WHERE provider_id = $1 AND status NOT IN ('completed', 'failed', 'cancelled')
ORDER BY created_at DESC
LIMIT 1;
