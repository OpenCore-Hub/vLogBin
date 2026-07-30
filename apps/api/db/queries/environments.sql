-- name: CreateEnvironment :one
INSERT INTO environments (provider_id, kind, issuer)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetEnvironmentByID :one
SELECT * FROM environments WHERE id = $1;

-- name: ListEnvironmentsByProvider :many
SELECT * FROM environments WHERE provider_id = $1 ORDER BY created_at;
