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

-- name: GetCellByID :one
SELECT * FROM cells WHERE id = $1;

-- name: GetCellByCode :one
SELECT * FROM cells WHERE code = $1;

-- name: GetSharedCellByRegion :one
SELECT * FROM cells
WHERE region_id = $1 AND cell_type = 'shared'
ORDER BY created_at
LIMIT 1;

-- name: CreateCell :one
INSERT INTO cells (region_id, code, cell_type, status, capacity_limits)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateCellStatus :one
UPDATE cells SET status = $2 WHERE id = $1
RETURNING *;

-- name: GetCellByProviderID :one
SELECT c.* FROM cells c
JOIN providers p ON p.cell_id = c.id
WHERE p.id = $1;

-- name: ListCellsByRegion :many
SELECT * FROM cells
WHERE region_id = $1 AND status = 'active'
ORDER BY code;
