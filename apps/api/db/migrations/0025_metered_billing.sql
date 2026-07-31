-- +goose Up
-- Phase 4: Metered Billing + FinOps (spec Section 18).
-- Metered billing rules: per-unit pricing, tiered pricing, minimum spend.
-- FinOps: capacity forecasts, cost projections, budget alerts.

CREATE TABLE metered_pricing_rules (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id     uuid NOT NULL,
    environment_id  uuid NOT NULL,
    metric_code     text NOT NULL,
    pricing_model   text NOT NULL CHECK (pricing_model IN ('per_unit', 'tiered', 'volume', 'stairstep')),
    base_price_cents bigint NOT NULL DEFAULT 0,
    tier_config     jsonb NOT NULL DEFAULT '[]',
    minimum_spend_cents bigint NOT NULL DEFAULT 0,
    enabled         boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id, environment_id, metric_code)
);

CREATE TABLE budget_alerts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id     uuid NOT NULL,
    environment_id  uuid NOT NULL,
    subscription_id uuid,
    metric_code     text,
    budget_cents    bigint NOT NULL CHECK (budget_cents > 0),
    threshold_pct   double precision NOT NULL DEFAULT 80.00 CHECK (threshold_pct > 0 AND threshold_pct <= 100),
    current_spend_cents bigint NOT NULL DEFAULT 0,
    alert_status    text NOT NULL DEFAULT 'ok' CHECK (alert_status IN ('ok', 'warning', 'exceeded')),
    last_alerted_at timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_budget_alerts_tenant ON budget_alerts (provider_id, environment_id, alert_status);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON metered_pricing_rules TO platform_app;
        GRANT SELECT, INSERT, UPDATE, DELETE ON budget_alerts TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE metered_pricing_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE metered_pricing_rules FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON metered_pricing_rules
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

ALTER TABLE budget_alerts ENABLE ROW LEVEL SECURITY;
ALTER TABLE budget_alerts FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON budget_alerts
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
DROP POLICY IF EXISTS tenant_isolation ON budget_alerts;
ALTER TABLE budget_alerts DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON metered_pricing_rules;
ALTER TABLE metered_pricing_rules DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS budget_alerts;
DROP TABLE IF EXISTS metered_pricing_rules;
