-- name: CreateCellFailover :one
INSERT INTO cell_failovers (provider_id, from_cell_id, to_cell_id, reason, initiated_by, fencing_token)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetCellFailoverByID :one
SELECT * FROM cell_failovers WHERE id = $1;

-- name: ListCellFailoversByProvider :many
SELECT * FROM cell_failovers
WHERE provider_id = $1
ORDER BY started_at DESC
LIMIT $2;

-- name: UpdateCellFailoverStatus :one
UPDATE cell_failovers
SET status = $2, completed_at = CASE WHEN $2 IN ('completed', 'aborted') THEN now() ELSE completed_at END
WHERE id = $1
RETURNING *;

-- name: UpdateCellFailoverReplay :exec
UPDATE cell_failovers
SET replayed_usage = $2, replayed_outbox = $3
WHERE id = $1;

-- name: GetActiveCellFailover :one
SELECT * FROM cell_failovers
WHERE provider_id = $1 AND status NOT IN ('completed', 'aborted')
ORDER BY started_at DESC
LIMIT 1;
