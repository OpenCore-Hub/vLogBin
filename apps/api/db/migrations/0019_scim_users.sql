-- +goose Up
-- Phase 2: SCIM 2.0 User Provisioning (spec Section 4.2, 6.2).
-- Enterprise customers can automate user provisioning via SCIM 2.0.
-- SCIM users are mapped to platform customers with external IDs.
-- The SCIM server endpoints (/scim/v2/Users) are authenticated via
-- bearer token (provider API key with scim:manage scope).

CREATE TABLE scim_users (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id     uuid NOT NULL,
    environment_id  uuid NOT NULL,
    external_id     text NOT NULL,
    display_name    text NOT NULL,
    email           text NOT NULL,
    active          boolean NOT NULL DEFAULT true,
    customer_id     uuid,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id, environment_id, external_id),
    UNIQUE (provider_id, environment_id, email)
);

CREATE INDEX idx_scim_users_tenant ON scim_users (provider_id, environment_id, active);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON scim_users TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE scim_users ENABLE ROW LEVEL SECURITY;
ALTER TABLE scim_users FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON scim_users
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
DROP POLICY IF EXISTS tenant_isolation ON scim_users;
ALTER TABLE scim_users DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS scim_users;
