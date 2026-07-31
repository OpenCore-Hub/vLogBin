-- +goose Up
-- Phase 2: SCIM 2.0 Groups (spec Section 4.2, 6.2).
-- Enterprise SCIM clients require /Groups endpoint for group lifecycle.
-- Groups can have members (SCIM users) for role-based access management.

CREATE TABLE scim_groups (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id     uuid NOT NULL,
    environment_id  uuid NOT NULL,
    external_id     text NOT NULL,
    display_name    text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id, environment_id, external_id)
);

CREATE TABLE scim_group_members (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id        uuid NOT NULL REFERENCES scim_groups(id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES scim_users(id) ON DELETE CASCADE,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (group_id, user_id)
);

CREATE INDEX idx_scim_groups_tenant ON scim_groups (provider_id, environment_id);
CREATE INDEX idx_scim_group_members_group ON scim_group_members (group_id);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON scim_groups TO platform_app;
        GRANT SELECT, INSERT, DELETE ON scim_group_members TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE scim_groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE scim_groups FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON scim_groups
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

ALTER TABLE scim_group_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE scim_group_members FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON scim_group_members
    USING (
        current_setting('app.is_operator', true) = 'on'
        OR EXISTS (
            SELECT 1 FROM scim_groups sg
            WHERE sg.id = scim_group_members.group_id
            AND sg.provider_id::text = current_setting('app.provider_id', true)
            AND sg.environment_id::text = current_setting('app.environment_id', true)
        )
    )
    WITH CHECK (
        current_setting('app.is_operator', true) = 'on'
        OR EXISTS (
            SELECT 1 FROM scim_groups sg
            WHERE sg.id = scim_group_members.group_id
            AND sg.provider_id::text = current_setting('app.provider_id', true)
            AND sg.environment_id::text = current_setting('app.environment_id', true)
        )
    );

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation ON scim_group_members;
ALTER TABLE scim_group_members DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON scim_groups;
ALTER TABLE scim_groups DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS scim_group_members;
DROP TABLE IF EXISTS scim_groups;
