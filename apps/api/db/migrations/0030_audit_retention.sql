-- +goose Up
-- Audit retention: bounded, operator-only purging of audit_events.
--
-- audit_events is strictly append-only (see 0002_rls.sql: the
-- audit_events_append_only trigger rejects UPDATE/DELETE even for the table
-- owner). To bound table growth without weakening that invariant, the only
-- code path allowed to delete rows is this SECURITY DEFINER function, which
--   1. verifies the caller is in operator context (app.is_operator = 'on'),
--   2. disables the append-only trigger for its own transaction,
--   3. deletes a bounded batch (created_at < cutoff, oldest first),
--   4. re-enables the trigger before returning.
-- Disable/delete/enable run in one transaction, so any failure rolls the
-- batch back with the trigger untouched. The function is owned by the
-- migration role (which owns audit_events), so the ALTER TABLE is permitted
-- and the superuser ownership also bypasses row-level security for the delete.
CREATE INDEX IF NOT EXISTS idx_audit_events_created_at ON audit_events (created_at);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION purge_audit_events(p_cutoff timestamptz, p_max_rows bigint)
RETURNS bigint
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $fn$
DECLARE
    deleted bigint := 0;
BEGIN
    IF current_setting('app.is_operator', true) <> 'on' THEN
        RAISE EXCEPTION 'audit purge requires operator context (app.is_operator = on)';
    END IF;
    ALTER TABLE audit_events DISABLE TRIGGER audit_events_append_only;
    DELETE FROM audit_events
    WHERE ctid IN (
        SELECT ctid
        FROM audit_events
        WHERE created_at < p_cutoff
        ORDER BY created_at
        LIMIT p_max_rows
    );
    GET DIAGNOSTICS deleted = ROW_COUNT;
    ALTER TABLE audit_events ENABLE TRIGGER audit_events_append_only;
    RETURN deleted;
END;
$fn$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION purge_audit_events(timestamptz, bigint) FROM PUBLIC;
-- Grants for the runtime role (created out of band, as in 0002/0004).
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT EXECUTE ON FUNCTION purge_audit_events(timestamptz, bigint) TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS purge_audit_events(timestamptz, bigint);
DROP INDEX IF EXISTS idx_audit_events_created_at;
