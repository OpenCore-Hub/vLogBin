-- +goose Up
-- Phase 1: Reconciliation results (spec Section 22). The reconciliation
-- worker runs hourly, checks internal consistency, and stores results for
-- operator audit. RLS is operator-only: providers cannot access this table.

CREATE TABLE reconciliation_results (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    check_name      text NOT NULL,
    status          text NOT NULL CHECK (status IN ('ok', 'drift', 'error')),
    expected_count  bigint NOT NULL DEFAULT 0,
    actual_count    bigint NOT NULL DEFAULT 0,
    drift_count     bigint NOT NULL DEFAULT 0,
    details         jsonb NOT NULL DEFAULT '{}',
    checked_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_reconciliation_checked_at ON reconciliation_results (checked_at DESC);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT ON reconciliation_results TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE reconciliation_results ENABLE ROW LEVEL SECURITY;
ALTER TABLE reconciliation_results FORCE ROW LEVEL SECURITY;

CREATE POLICY operator_only ON reconciliation_results
    USING (current_setting('app.is_operator', true) = 'on')
    WITH CHECK (current_setting('app.is_operator', true) = 'on');

-- +goose Down
DROP POLICY IF EXISTS operator_only ON reconciliation_results;
ALTER TABLE reconciliation_results DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS reconciliation_results;
