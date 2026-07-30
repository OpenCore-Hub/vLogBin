-- +goose Up
-- Row Level Security: every tenant-scoped table is visible only when the
-- transaction sets app.provider_id (and app.environment_id for
-- environment-scoped tables) via SET LOCAL. Operators bypass via
-- app.is_operator = 'on'. The platform_app role is created out of band
-- (deploy/db/init/00-roles.sql for compose, test setup for tests); all
-- grants here are guarded so migrations also run as superuser without it.

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON regions, cells, providers, environments,
            credentials, outbox_events, inbox_events, commerce_accounts TO platform_app;
        GRANT SELECT, INSERT ON audit_events TO platform_app;
        GRANT USAGE, SELECT ON SEQUENCE audit_events_id_seq TO platform_app;
        REVOKE UPDATE, DELETE ON audit_events FROM platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE providers ENABLE ROW LEVEL SECURITY;
ALTER TABLE providers FORCE ROW LEVEL SECURITY;
ALTER TABLE environments ENABLE ROW LEVEL SECURITY;
ALTER TABLE environments FORCE ROW LEVEL SECURITY;
ALTER TABLE credentials ENABLE ROW LEVEL SECURITY;
ALTER TABLE credentials FORCE ROW LEVEL SECURITY;
ALTER TABLE audit_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_events FORCE ROW LEVEL SECURITY;
ALTER TABLE outbox_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox_events FORCE ROW LEVEL SECURITY;
ALTER TABLE inbox_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE inbox_events FORCE ROW LEVEL SECURITY;
ALTER TABLE commerce_accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE commerce_accounts FORCE ROW LEVEL SECURITY;

-- providers: a tenant may read only its own row.
CREATE POLICY tenant_isolation ON providers
    USING (current_setting('app.is_operator', true) = 'on'
           OR id::text = current_setting('app.provider_id', true))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR id::text = current_setting('app.provider_id', true));

CREATE POLICY tenant_isolation ON environments
    USING (current_setting('app.is_operator', true) = 'on'
           OR provider_id::text = current_setting('app.provider_id', true))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR provider_id::text = current_setting('app.provider_id', true));

CREATE POLICY tenant_isolation ON credentials
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)));

-- audit: environment-less rows (e.g. provider-level operator actions) are
-- visible to both environments of that provider.
CREATE POLICY tenant_isolation ON audit_events
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND (environment_id IS NULL
                    OR environment_id::text = current_setting('app.environment_id', true))))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND (environment_id IS NULL
                    OR environment_id::text = current_setting('app.environment_id', true))));

CREATE POLICY tenant_isolation ON outbox_events
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)));

CREATE POLICY tenant_isolation ON inbox_events
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)));

-- commerce: providers never see domain='platform' rows.
CREATE POLICY tenant_isolation ON commerce_accounts
    USING (current_setting('app.is_operator', true) = 'on'
           OR (domain = 'provider'
               AND provider_id::text = current_setting('app.provider_id', true)
               AND (environment_id IS NULL
                    OR environment_id::text = current_setting('app.environment_id', true))))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (domain = 'provider'
               AND provider_id::text = current_setting('app.provider_id', true)
               AND (environment_id IS NULL
                    OR environment_id::text = current_setting('app.environment_id', true))));

-- Append-only enforcement for audit_events, independent of role grants
-- (also fires for the table owner / superuser).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION reject_audit_mutation() RETURNS trigger AS $fn$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only: % not allowed', TG_OP;
END;
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER audit_events_append_only
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION reject_audit_mutation();

-- +goose Down
DROP TRIGGER IF EXISTS audit_events_append_only ON audit_events;
DROP FUNCTION IF EXISTS reject_audit_mutation();

DROP POLICY IF EXISTS tenant_isolation ON commerce_accounts;
DROP POLICY IF EXISTS tenant_isolation ON inbox_events;
DROP POLICY IF EXISTS tenant_isolation ON outbox_events;
DROP POLICY IF EXISTS tenant_isolation ON audit_events;
DROP POLICY IF EXISTS tenant_isolation ON credentials;
DROP POLICY IF EXISTS tenant_isolation ON environments;
DROP POLICY IF EXISTS tenant_isolation ON providers;

ALTER TABLE commerce_accounts NO FORCE ROW LEVEL SECURITY;
ALTER TABLE inbox_events NO FORCE ROW LEVEL SECURITY;
ALTER TABLE outbox_events NO FORCE ROW LEVEL SECURITY;
ALTER TABLE audit_events NO FORCE ROW LEVEL SECURITY;
ALTER TABLE credentials NO FORCE ROW LEVEL SECURITY;
ALTER TABLE environments NO FORCE ROW LEVEL SECURITY;
ALTER TABLE providers NO FORCE ROW LEVEL SECURITY;

ALTER TABLE commerce_accounts DISABLE ROW LEVEL SECURITY;
ALTER TABLE inbox_events DISABLE ROW LEVEL SECURITY;
ALTER TABLE outbox_events DISABLE ROW LEVEL SECURITY;
ALTER TABLE audit_events DISABLE ROW LEVEL SECURITY;
ALTER TABLE credentials DISABLE ROW LEVEL SECURITY;
ALTER TABLE environments DISABLE ROW LEVEL SECURITY;
ALTER TABLE providers DISABLE ROW LEVEL SECURITY;
