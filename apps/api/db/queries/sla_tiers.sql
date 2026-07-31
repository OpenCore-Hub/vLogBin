-- name: CreateSLATier :one
INSERT INTO sla_tiers (provider_id, environment_id, code, name, uptime_sla, priority_level, reserved_capacity)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetSLATierByID :one
SELECT * FROM sla_tiers WHERE id = $1;

-- name: GetSLATierByCode :one
SELECT * FROM sla_tiers
WHERE provider_id = $1 AND environment_id = $2 AND code = $3;

-- name: ListSLATiers :many
SELECT * FROM sla_tiers
WHERE provider_id = $1 AND environment_id = $2
ORDER BY priority_level DESC;

-- name: UpdateSLATier :one
UPDATE sla_tiers
SET name = $2, uptime_sla = $3, priority_level = $4, reserved_capacity = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteSLATier :execrows
DELETE FROM sla_tiers
WHERE provider_id = $1 AND environment_id = $2 AND id = $3;
