-- name: InsertOutboxEvent :one
INSERT INTO outbox_events (provider_id, environment_id, aggregate_type, aggregate_id, event_type, payload, payload_hash, transaction_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: InsertOutboxEventIdempotent :exec
INSERT INTO outbox_events (provider_id, environment_id, aggregate_type, aggregate_id, event_type, payload, payload_hash, transaction_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (provider_id, environment_id, transaction_id) DO NOTHING;

-- name: ListOutboxEventsByTenant :many
SELECT * FROM outbox_events
WHERE provider_id = $1 AND environment_id = $2
ORDER BY created_at DESC
LIMIT $3;

-- name: StreamOutboxEvents :many
-- Cursor-based forward pagination for the Enterprise Event Stream API.
-- The cursor is the last event ID the consumer processed. Uses tuple
-- comparison (created_at, id) to handle same-timestamp events within a
-- single transaction correctly. Optional filters by event_type and
-- aggregate_type allow consumers to subscribe to specific event categories.
-- Pass uuid.Nil (all-zeros) as the cursor to start from the beginning.
-- Pass empty strings for event_type/aggregate_type to skip filtering.
SELECT oe.* FROM outbox_events oe
WHERE oe.provider_id = $1 AND oe.environment_id = $2
    AND ($3::uuid = '00000000-0000-0000-0000-000000000000' OR (oe.created_at, oe.id) > (
        SELECT oe2.created_at, oe2.id FROM outbox_events oe2 WHERE oe2.id = $3
    ))
    AND ($4::text = '' OR oe.event_type = $4)
    AND ($5::text = '' OR oe.aggregate_type = $5)
ORDER BY oe.created_at ASC, oe.id ASC
LIMIT $6;

-- name: ClaimDueOutboxEvents :many
-- Atomically claims and leases a batch: sets next_attempt_at 120s ahead so
-- concurrent relay instances skip these rows (SKIP LOCKED + lease). The
-- caller delivers events OUTSIDE this transaction and then marks each
-- event's outcome (published / retry / dead-letter) in a separate short
-- transaction. If the relay crashes, the 120s lease expires and another
-- instance re-claims — at-least-once delivery.
UPDATE outbox_events
SET next_attempt_at = now() + interval '120 seconds'
WHERE id IN (
    SELECT id FROM outbox_events
    WHERE (status = 'pending' AND (next_attempt_at IS NULL OR next_attempt_at <= now()))
       OR (status = 'failed' AND next_attempt_at IS NOT NULL AND next_attempt_at <= now())
    ORDER BY created_at
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: MarkOutboxEventPublished :exec
UPDATE outbox_events SET status = 'published', published_at = now(), next_attempt_at = NULL WHERE id = $1;

-- name: MarkOutboxEventFailed :exec
UPDATE outbox_events SET status = 'failed', attempts = attempts + 1 WHERE id = $1;

-- name: MarkOutboxEventRetry :exec
UPDATE outbox_events SET status = 'failed', attempts = attempts + 1, next_attempt_at = $2 WHERE id = $1;

-- name: MarkOutboxEventDeadLetter :exec
UPDATE outbox_events SET status = 'failed', attempts = attempts + 1, next_attempt_at = NULL WHERE id = $1;
