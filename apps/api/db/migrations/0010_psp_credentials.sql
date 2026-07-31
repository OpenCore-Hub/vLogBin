-- +goose Up
-- Phase 1: Provider PSP credential management (spec Section 13).
-- Providers register their own payment service provider credentials
-- (e.g. Stripe API key + webhook secret). The platform encrypts them
-- at rest with AES-256-GCM and isolates by provider/environment (RLS).
-- Key rotation creates a new version and revokes the old one without
-- downtime (Testing #34).

CREATE TABLE psp_credentials (
    id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id              uuid NOT NULL,
    environment_id           uuid NOT NULL,
    psp_type                 text NOT NULL CHECK (psp_type IN ('stripe', 'adyen', 'mollie', 'paypal', 'other')),
    label                    text NOT NULL DEFAULT '',
    encrypted_api_key        text NOT NULL,
    encrypted_webhook_secret text,
    key_version              int NOT NULL DEFAULT 1,
    active                   boolean NOT NULL DEFAULT true,
    created_at               timestamptz NOT NULL DEFAULT now(),
    rotated_at               timestamptz,
    revoked_at               timestamptz,
    UNIQUE (provider_id, environment_id, psp_type, key_version)
);

CREATE INDEX idx_psp_credentials_tenant ON psp_credentials (provider_id, environment_id, active);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE ON psp_credentials TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE psp_credentials ENABLE ROW LEVEL SECURITY;
ALTER TABLE psp_credentials FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON psp_credentials
    USING (
        current_setting('app.is_operator', true) = 'on'
        OR provider_id::text = current_setting('app.provider_id', true)
    );

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation ON psp_credentials;
ALTER TABLE psp_credentials DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS psp_credentials;
