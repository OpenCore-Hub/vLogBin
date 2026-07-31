-- name: CreateWebhookEndpoint :one
INSERT INTO webhook_endpoints (provider_id, environment_id, url, secret, enabled, events)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListWebhookEndpoints :many
SELECT * FROM webhook_endpoints
WHERE provider_id = $1 AND environment_id = $2
ORDER BY created_at;

-- name: DeleteWebhookEndpoint :exec
DELETE FROM webhook_endpoints
WHERE id = $1 AND provider_id = $2 AND environment_id = $3;

-- name: ListEnabledWebhooksByTenant :many
SELECT * FROM webhook_endpoints
WHERE provider_id = $1 AND environment_id = $2 AND enabled = true;

-- name: FindUndeliveredOutboxEvents :many
SELECT oe.id, oe.provider_id, oe.environment_id, oe.aggregate_type, oe.aggregate_id, oe.event_type, oe.payload, oe.payload_hash, oe.transaction_id, oe.status, oe.attempts, oe.created_at, oe.published_at, oe.next_attempt_at
FROM outbox_events oe
WHERE oe.status = 'published'
  AND NOT EXISTS (SELECT 1 FROM webhook_deliveries wd WHERE wd.outbox_event_id = oe.id)
ORDER BY oe.created_at
LIMIT $1;

-- name: CreateWebhookDelivery :exec
INSERT INTO webhook_deliveries (endpoint_id, outbox_event_id, provider_id, environment_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (endpoint_id, outbox_event_id) DO NOTHING;

-- name: ClaimPendingWebhookDeliveries :many
UPDATE webhook_deliveries
SET next_attempt_at = now() + interval '60 seconds',
    attempts = attempts + 1
WHERE id IN (
    SELECT id FROM webhook_deliveries
    WHERE (status = 'pending' AND (next_attempt_at IS NULL OR next_attempt_at <= now()))
       OR (status = 'failed' AND next_attempt_at IS NOT NULL AND next_attempt_at <= now())
    ORDER BY created_at
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: MarkWebhookDelivered :exec
UPDATE webhook_deliveries
SET status = 'delivered', response_status = $2, response_body = $3, delivered_at = now()
WHERE id = $1;

-- name: MarkWebhookRetry :exec
UPDATE webhook_deliveries
SET status = 'failed', response_status = $2, response_body = $3, next_attempt_at = $4
WHERE id = $1;

-- name: MarkWebhookDeadLetter :exec
UPDATE webhook_deliveries
SET status = 'dead_letter', response_status = $2, response_body = $3, next_attempt_at = NULL
WHERE id = $1;

-- name: ListWebhookDeliveriesByTenant :many
SELECT wd.id, wd.endpoint_id, wd.outbox_event_id, wd.provider_id, wd.environment_id, wd.status, wd.attempts, wd.response_status, wd.response_body, wd.next_attempt_at, wd.delivered_at, wd.created_at
FROM webhook_deliveries wd
WHERE wd.provider_id = $1 AND wd.environment_id = $2
ORDER BY wd.created_at DESC
LIMIT $3;

-- name: ListWebhookEndpointsByProvider :many
SELECT * FROM webhook_endpoints WHERE provider_id = $1 ORDER BY created_at;

-- name: GetWebhookEndpointByID :one
SELECT * FROM webhook_endpoints
WHERE id = $1 AND provider_id = $2 AND environment_id = $3;

-- name: GetOutboxEventByIDForWebhook :one
SELECT * FROM outbox_events
WHERE id = $1 AND provider_id = $2 AND environment_id = $3;

-- name: ListWebhookDeliveriesByProvider :many
SELECT * FROM webhook_deliveries WHERE provider_id = $1 ORDER BY created_at DESC LIMIT 200;
