-- +goose Up
-- providers and environments are operator-governed registry tables:
-- lifecycle_state, cell assignment, slug and issuer must never change from
-- a tenant-context transaction. RLS alone only scopes row visibility (a
-- tenant can still UPDATE its own providers row), so this trigger rejects
-- any UPDATE/DELETE that is not operator-authenticated
-- (current_setting('app.is_operator', true) = 'on'). Like the audit
-- trigger, it fires for every role including the table owner.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION require_operator_write() RETURNS trigger AS $fn$
BEGIN
    IF current_setting('app.is_operator', true) IS DISTINCT FROM 'on' THEN
        RAISE EXCEPTION '% is operator-governed: % requires operator context', TG_TABLE_NAME, TG_OP;
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER providers_operator_write
    BEFORE UPDATE OR DELETE ON providers
    FOR EACH ROW EXECUTE FUNCTION require_operator_write();

CREATE TRIGGER environments_operator_write
    BEFORE UPDATE OR DELETE ON environments
    FOR EACH ROW EXECUTE FUNCTION require_operator_write();

-- +goose Down
DROP TRIGGER IF EXISTS environments_operator_write ON environments;
DROP TRIGGER IF EXISTS providers_operator_write ON providers;
DROP FUNCTION IF EXISTS require_operator_write();
