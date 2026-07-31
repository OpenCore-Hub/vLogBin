-- +goose Up
-- Phase 1: JIT Support Access Plane (spec ID #51, User Stories #77-79).
-- Platform support engineers can request time-limited, approved access
-- to a provider's environment. Standard access requires provider approval;
-- emergency (break-glass) access requires two-person operator authorization.
-- All sessions are fully audited and visible to the provider (Testing #26).

CREATE TABLE support_sessions (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id       uuid NOT NULL,
    environment_id    uuid NOT NULL,
    access_type       text NOT NULL CHECK (access_type IN ('standard', 'emergency')),
    status            text NOT NULL DEFAULT 'requested' CHECK (status IN ('requested', 'active', 'expired', 'revoked', 'denied')),
    requested_by      text NOT NULL,
    reason            text NOT NULL,
    requested_scopes  text[] NOT NULL DEFAULT '{}',
    approved_by       text,
    second_approver   text,
    granted_at        timestamptz,
    expires_at        timestamptz NOT NULL,
    revoked_at        timestamptz,
    revoked_by        text,
    revoke_reason     text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    -- The requester cannot be the approver (two-person rule for emergency).
    CONSTRAINT emergency_two_person CHECK (
        access_type != 'emergency' OR approved_by IS DISTINCT FROM requested_by
    )
);

CREATE INDEX idx_support_sessions_provider ON support_sessions (provider_id, environment_id, created_at DESC);
CREATE INDEX idx_support_sessions_status ON support_sessions (status, expires_at);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON support_sessions TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE support_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE support_sessions FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON support_sessions
    USING (
        current_setting('app.is_operator', true) = 'on'
        OR (provider_id::text = current_setting('app.provider_id', true)
            AND environment_id::text = current_setting('app.environment_id', true))
    );

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation ON support_sessions;
ALTER TABLE support_sessions DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS support_sessions;
