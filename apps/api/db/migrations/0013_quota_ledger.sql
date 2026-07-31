-- +goose Up
-- Phase 2: Hard Quota Ledger (spec ID #32, Section 11.2).
-- Provides persistent reserve/commit/release quota semantics for scarce
-- or expensive resources. The database is the authoritative truth source;
-- Redis may only accelerate lookups (spec ID #33). This prevents concurrent
-- overspend without distributed locks (Testing #20).

CREATE TABLE quota_limits (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id     uuid NOT NULL,
    environment_id  uuid NOT NULL,
    subscription_id uuid NOT NULL,
    quota_key       text NOT NULL,
    limit_value     bigint NOT NULL CHECK (limit_value >= 0),
    period_type     text NOT NULL DEFAULT 'monthly' CHECK (period_type IN ('daily', 'monthly', 'total')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (subscription_id, quota_key)
);

CREATE TABLE quota_reservations (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id     uuid NOT NULL,
    environment_id  uuid NOT NULL,
    subscription_id uuid NOT NULL,
    quota_key       text NOT NULL,
    amount          bigint NOT NULL CHECK (amount > 0),
    status          text NOT NULL DEFAULT 'reserved' CHECK (status IN ('reserved', 'committed', 'released', 'expired')),
    reservation_id  text NOT NULL,
    expires_at      timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    committed_at    timestamptz,
    released_at     timestamptz,
    UNIQUE (provider_id, environment_id, reservation_id)
);

CREATE INDEX idx_quota_limits_tenant ON quota_limits (provider_id, environment_id, subscription_id);
CREATE INDEX idx_quota_reservations_sub ON quota_reservations (subscription_id, quota_key, status);
CREATE INDEX idx_quota_reservations_expiry ON quota_reservations (status, expires_at) WHERE status = 'reserved';

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON quota_limits TO platform_app;
        GRANT SELECT, INSERT, UPDATE, DELETE ON quota_reservations TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE quota_limits ENABLE ROW LEVEL SECURITY;
ALTER TABLE quota_limits FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON quota_limits
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

ALTER TABLE quota_reservations ENABLE ROW LEVEL SECURITY;
ALTER TABLE quota_reservations FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON quota_reservations
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
DROP POLICY IF EXISTS tenant_isolation ON quota_reservations;
ALTER TABLE quota_reservations DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON quota_limits;
ALTER TABLE quota_limits DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS quota_reservations;
DROP TABLE IF EXISTS quota_limits;
