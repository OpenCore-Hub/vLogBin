-- name: UpsertQuotaLimit :one
INSERT INTO quota_limits (provider_id, environment_id, subscription_id, quota_key, limit_value, period_type)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (subscription_id, quota_key) DO UPDATE
SET limit_value = EXCLUDED.limit_value, period_type = EXCLUDED.period_type, updated_at = now()
RETURNING *;

-- name: GetQuotaLimit :one
SELECT * FROM quota_limits
WHERE subscription_id = $1 AND quota_key = $2;

-- name: ListQuotaLimitsBySubscription :many
SELECT * FROM quota_limits
WHERE provider_id = $1 AND environment_id = $2 AND subscription_id = $3
ORDER BY quota_key;

-- name: DeleteQuotaLimit :execrows
DELETE FROM quota_limits
WHERE provider_id = $1 AND environment_id = $2 AND subscription_id = $3 AND quota_key = $4;

-- name: ReserveQuota :one
-- Atomic check-and-insert: the CTE computes current usage (reserved + committed)
-- and only inserts if usage + amount <= limit. Returns no rows when exceeded
-- (pgx.ErrNoRows → service translates to ErrQuotaExceeded).
WITH current_usage AS (
    SELECT COALESCE(SUM(amount), 0)::bigint AS total
    FROM quota_reservations
    WHERE subscription_id = $1 AND quota_key = $2
        AND status IN ('reserved', 'committed')
),
ql AS (
    SELECT limit_value FROM quota_limits
    WHERE subscription_id = $1 AND quota_key = $2
)
INSERT INTO quota_reservations (
    provider_id, environment_id, subscription_id, quota_key,
    amount, reservation_id, expires_at
)
SELECT $3, $4, $1, $2, $5, $6, $7
FROM current_usage, ql
WHERE current_usage.total + $5 <= ql.limit_value
RETURNING *;

-- name: GetQuotaReservationByID :one
SELECT * FROM quota_reservations WHERE id = $1;

-- name: GetQuotaReservationByTxID :one
SELECT * FROM quota_reservations
WHERE provider_id = $1 AND environment_id = $2 AND reservation_id = $3;

-- name: CommitQuotaReservation :one
UPDATE quota_reservations
SET status = 'committed', committed_at = now()
WHERE id = $1 AND status = 'reserved'
RETURNING *;

-- name: ReleaseQuotaReservation :one
UPDATE quota_reservations
SET status = 'released', released_at = now()
WHERE id = $1 AND status = 'reserved'
RETURNING *;

-- name: GetQuotaUsage :one
SELECT
    COALESCE(SUM(CASE WHEN status = 'committed' THEN amount ELSE 0 END), 0)::bigint AS committed,
    COALESCE(SUM(CASE WHEN status = 'reserved' THEN amount ELSE 0 END), 0)::bigint AS reserved
FROM quota_reservations
WHERE subscription_id = $1 AND quota_key = $2
    AND status IN ('reserved', 'committed');

-- name: ExpireQuotaReservations :execrows
UPDATE quota_reservations
SET status = 'expired'
WHERE status = 'reserved' AND expires_at IS NOT NULL AND expires_at <= now();

-- name: ListQuotaReservationsBySubscription :many
SELECT * FROM quota_reservations
WHERE provider_id = $1 AND environment_id = $2 AND subscription_id = $3
    AND status IN ('reserved', 'committed')
ORDER BY created_at DESC
LIMIT $4;
