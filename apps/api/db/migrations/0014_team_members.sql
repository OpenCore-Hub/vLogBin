-- +goose Up
-- Phase 2: Delegated Administration (委派管理).
-- Providers can invite team members with role-based access. Each team
-- member gets a linked API key with scopes derived from their role.
-- Role changes propagate to the credential's scopes. Removing a team
-- member immediately revokes their credential.

CREATE TABLE team_members (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id     uuid NOT NULL,
    environment_id  uuid NOT NULL,
    email           text NOT NULL,
    display_name    text NOT NULL,
    role            text NOT NULL CHECK (role IN ('admin', 'billing_admin', 'developer', 'support_agent')),
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'removed')),
    credential_id   uuid,
    invited_by      text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id, environment_id, email)
);

CREATE INDEX idx_team_members_tenant ON team_members (provider_id, environment_id, status);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON team_members TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE team_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_members FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON team_members
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
DROP POLICY IF EXISTS tenant_isolation ON team_members;
ALTER TABLE team_members DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS team_members;
