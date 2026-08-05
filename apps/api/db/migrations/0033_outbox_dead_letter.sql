-- +goose Up
-- Outbox dead-letter queue formalization. The relay has always intended a
-- terminal 'dead_letter' state, but the CHECK constraint only admitted
-- pending/published/failed, so markDeadLetter wrote a soft dead-letter
-- (status='failed', next_attempt_at=NULL) instead. That made the
-- reconciliation check CountDeadLetterOutbox (status='dead_letter') a no-op
-- and left no record of why an event died. This migration:
--   1. admits a real 'dead_letter' terminal status,
--   2. adds last_error to record the final failure cause,
--   3. upgrades existing soft dead-letters so the reconciliation check and
--      the backlog gauge reflect the true dead-letter count.
ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_status_check;
ALTER TABLE outbox_events ADD CONSTRAINT outbox_events_status_check
    CHECK (status IN ('pending', 'published', 'failed', 'dead_letter'));
ALTER TABLE outbox_events ADD COLUMN last_error text;

UPDATE outbox_events
SET status = 'dead_letter',
    last_error = 'upgraded from soft dead-letter (relay max attempts, pre-0033)'
WHERE status = 'failed' AND next_attempt_at IS NULL;

-- +goose Down
UPDATE outbox_events
SET status = 'failed',
    last_error = NULL
WHERE status = 'dead_letter';

ALTER TABLE outbox_events DROP COLUMN IF EXISTS last_error;
ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_status_check;
ALTER TABLE outbox_events ADD CONSTRAINT outbox_events_status_check
    CHECK (status IN ('pending', 'published', 'failed'));
