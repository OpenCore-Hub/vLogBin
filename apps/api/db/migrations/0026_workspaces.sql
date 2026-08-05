-- +goose Up
-- M1 Control Plane: workspaces + workspace_members (design baseline §3.1 R11).
-- Every platform user gets a default workspace at signup; the first user of a
-- workspace is auto-granted provider_admin. workspace_id maps 1:1 to
-- provider_id (§2.3) — the mapping is enforced by the signup service, not by
-- a FK, because provider provisioning remains an operator-owned pipeline.

CREATE TABLE workspaces (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        text NOT NULL UNIQUE,
    name        text NOT NULL,
    created_by  text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE workspace_members (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_sub      text NOT NULL,
    role          text NOT NULL CHECK (role IN ('provider_admin', 'provider_developer', 'provider_billing')),
    status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'removed')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, user_sub)
);

CREATE INDEX idx_workspace_members_user ON workspace_members (user_sub, status);
CREATE INDEX idx_workspace_members_workspace ON workspace_members (workspace_id, status);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON workspaces, workspace_members TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

-- Control-plane tables are reachable only through the operator transaction
-- (app.is_operator = 'on'). Provider API-key transactions must never see
-- them; per-user ownership is enforced by the handlers on top.
ALTER TABLE workspaces ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspaces FORCE ROW LEVEL SECURITY;
ALTER TABLE workspace_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspace_members FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON workspaces
    USING (current_setting('app.is_operator', true) = 'on')
    WITH CHECK (current_setting('app.is_operator', true) = 'on');

CREATE POLICY tenant_isolation ON workspace_members
    USING (current_setting('app.is_operator', true) = 'on')
    WITH CHECK (current_setting('app.is_operator', true) = 'on');

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation ON workspace_members;
DROP POLICY IF EXISTS tenant_isolation ON workspaces;
ALTER TABLE workspace_members NO FORCE ROW LEVEL SECURITY;
ALTER TABLE workspaces NO FORCE ROW LEVEL SECURITY;
ALTER TABLE workspace_members DISABLE ROW LEVEL SECURITY;
ALTER TABLE workspaces DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS workspace_members;
DROP TABLE IF EXISTS workspaces;
