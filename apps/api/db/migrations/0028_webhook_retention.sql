-- +goose Up
-- Webhook retention policy: webhook_deliveries and outbox_events accumulate
-- terminal rows forever (no TTL). The retention sweeper
-- (service.NewWebhookRetentionSweeper) deletes terminal deliveries and
-- terminal outbox events older than WEBHOOK_RETENTION_DAYS (default 30d).
-- This index makes the sweep's range scan (created_at < cutoff) efficient;
-- outbox_events is already covered by idx_outbox_events_status (status,
-- created_at). Non-terminal rows (pending, or failed rows still inside their
-- retry window with next_attempt_at set) are never touched by the sweep.
CREATE INDEX idx_webhook_deliveries_created_at ON webhook_deliveries (created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_webhook_deliveries_created_at;
