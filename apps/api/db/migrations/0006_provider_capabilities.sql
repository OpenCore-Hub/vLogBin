-- +goose Up
-- Phase 1: Provider Live capability grants. Each capability (messaging,
-- domains, payments, throughput, event_delivery) is granted independently
-- by the operator — there is no single "go live" switch (spec ID #46).
-- A provider in LIVE_ACTIVE may have some capabilities granted and others
-- pending or revoked.

CREATE TABLE provider_capabilities (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id uuid NOT NULL REFERENCES providers(id),
    capability  text NOT NULL CHECK (capability IN (
        'messaging', 'domains', 'payments', 'throughput', 'event_delivery'
    )),
    status      text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'granted', 'revoked')),
    granted_at  timestamptz,
    revoked_at  timestamptz,
    granted_by  text NOT NULL DEFAULT '',
    reason      text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id, capability)
);

CREATE INDEX idx_provider_capabilities_provider ON provider_capabilities (provider_id);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON provider_capabilities TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE provider_capabilities ENABLE ROW LEVEL SECURITY;
ALTER TABLE provider_capabilities FORCE ROW LEVEL SECURITY;

-- Operators see all rows; providers see only their own. Uses ::text
-- comparison (not ::uuid cast) because the platform_app role defaults
-- app.provider_id to '' at session level — a ::uuid cast of '' raises
-- an error even when is_operator short-circuits the OR.
CREATE POLICY tenant_isolation ON provider_capabilities
    USING (
        current_setting('app.is_operator', true) = 'on'
        OR provider_id::text = current_setting('app.provider_id', true)
    );

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation ON provider_capabilities;
ALTER TABLE provider_capabilities DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS provider_capabilities;
