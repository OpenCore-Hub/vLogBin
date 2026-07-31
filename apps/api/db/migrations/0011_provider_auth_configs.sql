-- +goose Up
-- Phase 2: Provider Hosted Auth configuration. Each provider can enable
-- ZITADEL Hosted Auth by creating a ZITADEL project + OIDC application.
-- The platform stores the project/client IDs and encrypts the client secret.

CREATE TABLE provider_auth_configs (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id             uuid NOT NULL,
    environment_id          uuid NOT NULL,
    zitadel_project_id      text NOT NULL,
    zitadel_app_id          text NOT NULL,
    zitadel_client_id       text NOT NULL,
    zitadel_client_secret   text NOT NULL,  -- AES-256-GCM encrypted
    enabled                 boolean NOT NULL DEFAULT true,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id, environment_id)
);

CREATE INDEX idx_auth_configs_tenant ON provider_auth_configs (provider_id, environment_id, enabled);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON provider_auth_configs TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE provider_auth_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE provider_auth_configs FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON provider_auth_configs
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
DROP POLICY IF EXISTS tenant_isolation ON provider_auth_configs;
ALTER TABLE provider_auth_configs DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS provider_auth_configs;
