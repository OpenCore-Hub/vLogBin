-- +goose Up
-- Phase 2: Tiered SLA and Reserved Capacity (spec Section 6.2, 16).
-- Providers define SLA tiers (basic, professional, enterprise) with
-- uptime guarantees and reserved capacity limits. Subscriptions are
-- assigned to SLA tiers for differentiated service levels.

CREATE TABLE sla_tiers (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id     uuid NOT NULL,
    environment_id  uuid NOT NULL,
    code            text NOT NULL,
    name            text NOT NULL,
    uptime_sla      double precision NOT NULL DEFAULT 99.90 CHECK (uptime_sla >= 0 AND uptime_sla <= 100),
    priority_level  int NOT NULL DEFAULT 1 CHECK (priority_level >= 1 AND priority_level <= 5),
    reserved_capacity jsonb NOT NULL DEFAULT '{}',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id, environment_id, code)
);

CREATE INDEX idx_sla_tiers_tenant ON sla_tiers (provider_id, environment_id);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON sla_tiers TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE sla_tiers ENABLE ROW LEVEL SECURITY;
ALTER TABLE sla_tiers FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON sla_tiers
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

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation ON sla_tiers;
ALTER TABLE sla_tiers DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS sla_tiers;
