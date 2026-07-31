-- +goose Up
-- Phase 2: Migration Plane (spec Section 17.1, Implementation Decision #48).
-- Enables providers to migrate from external billing systems with dry-run
-- validation, resumable imports, cutover lock, and rollback (User Stories #70-73).

CREATE TABLE migration_jobs (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id       uuid NOT NULL,
    environment_id    uuid NOT NULL,
    source_system     text NOT NULL,
    status            text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'validating', 'validated', 'importing', 'completed', 'failed', 'rolled_back')),
    dry_run           boolean NOT NULL DEFAULT false,
    total_records     int NOT NULL DEFAULT 0,
    processed_records int NOT NULL DEFAULT 0,
    failed_records    int NOT NULL DEFAULT 0,
    cutover_locked    boolean NOT NULL DEFAULT false,
    started_at        timestamptz,
    completed_at      timestamptz,
    error_summary     text,
    created_by        text NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE migration_records (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    migration_job_id  uuid NOT NULL,
    record_type       text NOT NULL CHECK (record_type IN ('customer', 'subscription')),
    external_id       text NOT NULL,
    status            text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'valid', 'invalid', 'imported', 'failed', 'rolled_back')),
    source_data       jsonb NOT NULL DEFAULT '{}',
    target_id         uuid,
    error_message     text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (migration_job_id, record_type, external_id)
);

CREATE INDEX idx_migration_records_job ON migration_records (migration_job_id, status);
CREATE INDEX idx_migration_jobs_tenant ON migration_jobs (provider_id, environment_id, created_at DESC);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON migration_jobs TO platform_app;
        GRANT SELECT, INSERT, UPDATE, DELETE ON migration_records TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE migration_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE migration_jobs FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON migration_jobs
    USING (
        current_setting('app.is_operator', true) = 'on'
        OR (provider_id::text = current_setting('app.provider_id', true)
            AND environment_id::text = current_setting('app.environment_id', true))
    )
    WITH CHECK (
        current_setting('app.is_operator', true) = 'on'
        OR (provider_id::text = current_setting('app.provider_id', true)
            AND environment_id::text = current_setting('app.environment_id', true))
    );

ALTER TABLE migration_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE migration_records FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON migration_records
    USING (
        current_setting('app.is_operator', true) = 'on'
        OR EXISTS (
            SELECT 1 FROM migration_jobs mj
            WHERE mj.id = migration_records.migration_job_id
            AND mj.provider_id::text = current_setting('app.provider_id', true)
            AND mj.environment_id::text = current_setting('app.environment_id', true)
        )
    )
    WITH CHECK (
        current_setting('app.is_operator', true) = 'on'
        OR EXISTS (
            SELECT 1 FROM migration_jobs mj
            WHERE mj.id = migration_records.migration_job_id
            AND mj.provider_id::text = current_setting('app.provider_id', true)
            AND mj.environment_id::text = current_setting('app.environment_id', true)
        )
    );

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation ON migration_records;
ALTER TABLE migration_records DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON migration_jobs;
ALTER TABLE migration_jobs DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS migration_records;
DROP TABLE IF EXISTS migration_jobs;
