-- +goose Up
-- Phase 3: Complete Export, Offboarding, and Deletion Proof (US #74-75).
-- Providers can request a complete data export for offboarding, and
-- receive cryptographic proof of deletion (SHA-256 hash tree + signature).

CREATE TABLE data_exports (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id     uuid NOT NULL,
    environment_id  uuid NOT NULL,
    status          text NOT NULL DEFAULT 'requested' CHECK (status IN ('requested', 'processing', 'completed', 'failed')),
    export_type     text NOT NULL DEFAULT 'full' CHECK (export_type IN ('full', 'audit_only', 'billing_only')),
    data_hash       text,
    export_data     bytea,
    record_count    int NOT NULL DEFAULT 0,
    error_message   text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    completed_at    timestamptz
);

CREATE TABLE deletion_proofs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id     uuid NOT NULL,
    environment_id  uuid NOT NULL,
    data_hash       text NOT NULL,
    record_count    int NOT NULL DEFAULT 0,
    deleted_at      timestamptz NOT NULL DEFAULT now(),
    proof_signature text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_data_exports_tenant ON data_exports (provider_id, environment_id, created_at DESC);
CREATE INDEX idx_deletion_proofs_tenant ON deletion_proofs (provider_id, environment_id, created_at DESC);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON data_exports TO platform_app;
        GRANT SELECT, INSERT ON deletion_proofs TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE data_exports ENABLE ROW LEVEL SECURITY;
ALTER TABLE data_exports FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON data_exports
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

ALTER TABLE deletion_proofs ENABLE ROW LEVEL SECURITY;
ALTER TABLE deletion_proofs FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON deletion_proofs
    USING (
        current_setting('app.is_operator', true) = 'on'
        OR (provider_id::text = current_setting('app.provider_id', true)
            AND environment_id::text = current_setting('app.environment_id', true))
    );

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation ON deletion_proofs;
ALTER TABLE deletion_proofs DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON data_exports;
ALTER TABLE data_exports DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS deletion_proofs;
DROP TABLE IF EXISTS data_exports;
