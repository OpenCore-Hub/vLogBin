-- +goose Up
-- Phase 3: Cell Migration (spec Section 14, Phase 3 list).
-- Planned cell migration (shared→dedicated, or cross-cell in same region).
-- Unlike failover (emergency), migration is scheduled, low-risk, and
-- includes a pre-migration data integrity check.

CREATE TABLE cell_migrations (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id     uuid NOT NULL,
    from_cell_id    uuid NOT NULL,
    to_cell_id      uuid NOT NULL,
    status          text NOT NULL DEFAULT 'planned' CHECK (status IN ('planned', 'prechecking', 'ready', 'migrating', 'completed', 'failed', 'cancelled')),
    scheduled_at    timestamptz,
    precheck_passed boolean NOT NULL DEFAULT false,
    data_integrity_hash text,
    record_count    int NOT NULL DEFAULT 0,
    reason          text NOT NULL,
    initiated_by    text NOT NULL,
    started_at      timestamptz,
    completed_at    timestamptz,
    error_message   text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_cell_migrations_provider ON cell_migrations (provider_id, created_at DESC);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON cell_migrations TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE cell_migrations ENABLE ROW LEVEL SECURITY;
ALTER TABLE cell_migrations FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON cell_migrations
    USING (
        current_setting('app.is_operator', true) = 'on'
        OR provider_id::text = current_setting('app.provider_id', true)
    )
    WITH CHECK (
        current_setting('app.is_operator', true) = 'on'
        OR provider_id::text = current_setting('app.provider_id', true)
    );

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation ON cell_migrations;
ALTER TABLE cell_migrations DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS cell_migrations;
