-- +goose Up
-- Phase 3: Hot Standby + Failover (spec Section 14).
-- Hot standby cells in the same region provide disaster recovery.
-- Failover is manual (no auto-dual-master) with write fencing to
-- prevent split-brain. Unconfirmed Usage and Outbox events are
-- replayed after failover (spec: "切换后重放未确认 Usage 和 Outbox").

CREATE TABLE cell_failovers (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id     uuid NOT NULL,
    from_cell_id    uuid NOT NULL,
    to_cell_id      uuid NOT NULL,
    status          text NOT NULL DEFAULT 'initiated' CHECK (status IN ('initiated', 'fenced', 'switched', 'replaying', 'completed', 'aborted')),
    reason          text NOT NULL,
    initiated_by    text NOT NULL,
    fencing_token   text NOT NULL,
    replayed_usage  int NOT NULL DEFAULT 0,
    replayed_outbox int NOT NULL DEFAULT 0,
    started_at      timestamptz NOT NULL DEFAULT now(),
    completed_at    timestamptz,
    UNIQUE (provider_id, from_cell_id, to_cell_id, fencing_token)
);

CREATE INDEX idx_cell_failovers_provider ON cell_failovers (provider_id, started_at DESC);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON cell_failovers TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE cell_failovers ENABLE ROW LEVEL SECURITY;
ALTER TABLE cell_failovers FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON cell_failovers
    USING (
        current_setting('app.is_operator', true) = 'on'
        OR provider_id::text = current_setting('app.provider_id', true)
    )
    WITH CHECK (
        current_setting('app.is_operator', true) = 'on'
        OR provider_id::text = current_setting('app.provider_id', true)
    );

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation ON cell_failovers;
ALTER TABLE cell_failovers DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS cell_failovers;
