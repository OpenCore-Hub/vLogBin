-- +goose Up
-- Phase 2: Custom Auth Domains (spec Section 6.2).
-- Enterprise providers can register custom domains for branded auth flows.
-- Domain ownership is verified via DNS TXT records. Domain occupancy is
-- globally unique to prevent takeover attacks (spec Section 6.2: "Domain
-- Control Plane 管理证书、域名占用、吊销、接管防护和到期告警").

CREATE TABLE custom_domains (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id        uuid NOT NULL,
    environment_id     uuid NOT NULL,
    domain             text NOT NULL UNIQUE,
    verification_token text NOT NULL,
    status             text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'verified', 'revoked')),
    verified_at        timestamptz,
    revoked_at         timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_custom_domains_tenant ON custom_domains (provider_id, environment_id, status);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON custom_domains TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE custom_domains ENABLE ROW LEVEL SECURITY;
ALTER TABLE custom_domains FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON custom_domains
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
DROP POLICY IF EXISTS tenant_isolation ON custom_domains;
ALTER TABLE custom_domains DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS custom_domains;
