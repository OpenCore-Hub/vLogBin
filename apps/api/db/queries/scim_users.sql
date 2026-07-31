-- name: CreateSCIMUser :one
INSERT INTO scim_users (provider_id, environment_id, external_id, display_name, email, active, customer_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetSCIMUserByID :one
SELECT * FROM scim_users WHERE id = $1;

-- name: GetSCIMUserByExternalID :one
SELECT * FROM scim_users
WHERE provider_id = $1 AND environment_id = $2 AND external_id = $3;

-- name: ListSCIMUsers :many
-- activeFilter: 0 = inactive only, 1 = active only, 2 = all.
SELECT * FROM scim_users
WHERE provider_id = $1 AND environment_id = $2
    AND ($3::int = 2 OR active = ($3::int = 1))
ORDER BY created_at ASC
LIMIT $4;

-- name: CountSCIMUsers :one
SELECT COUNT(*) FROM scim_users
WHERE provider_id = $1 AND environment_id = $2;

-- name: UpdateSCIMUser :one
UPDATE scim_users
SET display_name = $2, email = $3, active = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetSCIMUserCustomer :exec
UPDATE scim_users SET customer_id = $2, updated_at = now() WHERE id = $1;

-- name: DeleteSCIMUser :execrows
DELETE FROM scim_users
WHERE provider_id = $1 AND environment_id = $2 AND id = $3;
