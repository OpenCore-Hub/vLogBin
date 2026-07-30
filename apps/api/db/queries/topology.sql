-- name: InsertCommerceAccount :one
INSERT INTO commerce_accounts (domain, provider_id, environment_id, display_name)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListCommerceAccounts :many
SELECT * FROM commerce_accounts ORDER BY created_at;

-- name: ListRegions :many
SELECT * FROM regions ORDER BY code;

-- name: GetRegionByCode :one
SELECT * FROM regions WHERE code = $1;

-- name: ListCells :many
SELECT * FROM cells ORDER BY code;

-- name: GetSharedCellByRegion :one
SELECT * FROM cells
WHERE region_id = $1 AND cell_type = 'shared'
ORDER BY created_at
LIMIT 1;
