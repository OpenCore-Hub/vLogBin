-- +goose Up
-- Phase 2: Bring your own email/SMS (spec Section 6.2, 16).
-- Providers configure their own SMTP/SMS gateways for customer notifications.
-- Credentials are encrypted with AES-256-GCM (same as PSP credentials).

CREATE TABLE notification_configs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id     uuid NOT NULL,
    environment_id  uuid NOT NULL,
    channel         text NOT NULL CHECK (channel IN ('email', 'sms')),
    provider_type   text NOT NULL,
    config_enc      bytea NOT NULL,
    from_address    text NOT NULL,
    enabled         boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id, environment_id, channel)
);

CREATE INDEX idx_notification_configs_tenant ON notification_configs (provider_id, environment_id, enabled);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON notification_configs TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE notification_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_configs FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON notification_configs
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
DROP POLICY IF EXISTS tenant_isolation ON notification_configs;
ALTER TABLE notification_configs DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS notification_configs;
