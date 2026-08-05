-- name: GetIdempotencyKey :one
SELECT * FROM idempotency_keys
WHERE scope = $1 AND key_hash = $2 AND method = $3 AND path = $4;

-- name: InsertIdempotencyKey :one
INSERT INTO idempotency_keys (scope, key_hash, method, path, request_id, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (scope, key_hash, method, path) DO NOTHING
RETURNING *;

-- name: CompleteIdempotencyKey :execrows
UPDATE idempotency_keys
SET status = 'completed', response_status = $2, content_type = $3, response_body = $4
WHERE id = $1 AND status = 'in_progress';

-- name: DeleteIdempotencyKey :execrows
DELETE FROM idempotency_keys WHERE id = $1;

-- name: DeleteExpiredIdempotencyKeys :execrows
DELETE FROM idempotency_keys WHERE expires_at < $1;
