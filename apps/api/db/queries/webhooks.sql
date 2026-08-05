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
SELECT oe.id, oe.provider_id, oe.environment_id, oe.aggregate_type, oe.aggregate_id, oe.event_type, oe.payload, oe.payload_hash, oe.transaction_id, oe.status, oe.attempts, oe.created_at, oe.published_at, oe.next_attempt_at, oe.last_error
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
-- Lifecycle-aware delivery (design baseline §7.4): deliveries for providers
-- in SUSPENDED or OFFBOARDING state are NOT claimed, so no HTTP call is made
-- while the provider is not operational. The rows stay 'pending' (backlog) and
-- are delivered automatically once the provider reactivates (SUSPENDED ->
-- LIVE_ACTIVE). RESTRICTED keeps delivering: it is a limited but operational
-- state. FOR UPDATE OF wd locks only webhook_deliveries rows so provider
-- lifecycle transitions never contend on this lease.
UPDATE webhook_deliveries
SET next_attempt_at = now() + interval '60 seconds',
    attempts = attempts + 1
WHERE id IN (
    SELECT wd.id
    FROM webhook_deliveries wd
    JOIN providers p ON p.id = wd.provider_id
    WHERE p.lifecycle_state NOT IN ('SUSPENDED', 'OFFBOARDING')
      AND ((wd.status = 'pending' AND (wd.next_attempt_at IS NULL OR wd.next_attempt_at <= now()))
        OR (wd.status = 'failed' AND wd.next_attempt_at IS NOT NULL AND wd.next_attempt_at <= now()))
    ORDER BY wd.created_at
    LIMIT $1
    FOR UPDATE OF wd SKIP LOCKED
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

-- name: GetWebhookDelivery :one
-- Scoped lookup used to pre-check a replay: the row must belong to the
-- provider named in the URL so a stale/mismatched delivery id cannot be
-- replayed onto the wrong tenant.
SELECT * FROM webhook_deliveries
WHERE id = sqlc.arg(id) AND provider_id = sqlc.arg(provider_id);

-- name: ReplayWebhookDelivery :one
-- Requeue a terminal delivery (dead_letter | failed) as 'pending' for
-- immediate redelivery: attempts reset, backoff cleared, response trace
-- wiped, delivered_at cleared. The status guard makes the update optimistic —
-- concurrent replays serialize and only the first one applies.
UPDATE webhook_deliveries
SET status = 'pending',
    attempts = 0,
    next_attempt_at = NULL,
    response_status = NULL,
    response_body = NULL,
    delivered_at = NULL
WHERE id = sqlc.arg(id) AND provider_id = sqlc.arg(provider_id)
  AND status IN ('dead_letter', 'failed')
RETURNING *;

-- name: CountWebhookDeliveriesByStatus :many
-- Backlog gauge for Prometheus: total webhook_deliveries rows grouped by
-- delivery status (pending / failed / delivered / dead_letter). Refreshed
-- periodically by the metrics backlog reporter.
SELECT status, COUNT(*)::bigint AS count
FROM webhook_deliveries
GROUP BY status;

-- name: DeleteExpiredWebhookDeliveries :execrows
-- Retention policy: delete terminal webhook deliveries (delivered | dead_letter |
-- failed with retries exhausted) older than the retention cutoff. Pending rows and
-- failed rows still inside their retry window (next_attempt_at set) are never
-- deleted — they may still be delivered or replayed. The terminal-state guard makes
-- the sweep race-safe: a delivery replayed to 'pending' by the operator between the
-- sweep's snapshot and this DELETE is immediately out of scope.
DELETE FROM webhook_deliveries
WHERE created_at < sqlc.arg(cutoff)
  AND (status = 'delivered'
    OR status = 'dead_letter'
    OR (status = 'failed' AND next_attempt_at IS NULL));
