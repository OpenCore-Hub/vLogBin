-- name: InsertOutboxEvent :one
INSERT INTO outbox_events (provider_id, environment_id, aggregate_type, aggregate_id, event_type, payload, payload_hash, transaction_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListOutboxEventsByTenant :many
SELECT * FROM outbox_events
WHERE provider_id = $1 AND environment_id = $2
ORDER BY created_at DESC
LIMIT $3;

-- name: ClaimPendingOutboxEvents :many
SELECT * FROM outbox_events
WHERE status = 'pending'
ORDER BY created_at
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxEventPublished :exec
UPDATE outbox_events SET status = 'published', published_at = now() WHERE id = $1;

-- name: MarkOutboxEventFailed :exec
UPDATE outbox_events SET status = 'failed', attempts = attempts + 1 WHERE id = $1;
