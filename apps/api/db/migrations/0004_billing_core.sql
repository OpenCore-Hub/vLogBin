-- +goose Up
-- Phase 1 billing core: customer accounts, versioned catalog (metrics,
-- plans, prices, entitlement grants), pinned subscriptions, append-only
-- usage events with reversal, entitlement overrides/snapshots, and outbox
-- retry scheduling. All tenant tables propagate provider_id +
-- environment_id and are protected by RLS (ENABLE + FORCE) with the same
-- policy shape as 0002_rls.sql; DB-level triggers enforce catalog
-- immutability, subscription pinning and usage append-only as defense in
-- depth below the service layer.

CREATE TABLE customer_accounts (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id    uuid NOT NULL,
    environment_id uuid NOT NULL,
    external_id    text NOT NULL,
    account_type   text NOT NULL CHECK (account_type IN ('individual', 'business')),
    display_name   text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id, environment_id, external_id)
);

CREATE TABLE catalog_versions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id    uuid NOT NULL,
    environment_id uuid NOT NULL,
    version        int NOT NULL,
    state          text NOT NULL DEFAULT 'draft' CHECK (state IN ('draft', 'validated', 'published', 'retired')),
    created_at     timestamptz NOT NULL DEFAULT now(),
    validated_at   timestamptz,
    published_at   timestamptz,
    retired_at     timestamptz,
    UNIQUE (provider_id, environment_id, version)
);

CREATE TABLE metrics (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    catalog_version_id uuid NOT NULL REFERENCES catalog_versions(id),
    provider_id        uuid NOT NULL,
    environment_id     uuid NOT NULL,
    code               text NOT NULL,
    name               text NOT NULL DEFAULT '',
    aggregation_type   text NOT NULL CHECK (aggregation_type IN ('count', 'sum', 'max', 'unique_count')),
    field_name         text,
    billable           boolean NOT NULL DEFAULT true,
    UNIQUE (catalog_version_id, code)
);

CREATE TABLE plans (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    catalog_version_id uuid NOT NULL REFERENCES catalog_versions(id),
    provider_id        uuid NOT NULL,
    environment_id     uuid NOT NULL,
    code               text NOT NULL,
    name               text NOT NULL DEFAULT '',
    interval           text NOT NULL CHECK (interval IN ('weekly', 'monthly', 'yearly')),
    currency           text NOT NULL CHECK (char_length(currency) = 3),
    UNIQUE (catalog_version_id, code)
);

CREATE TABLE prices (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id            uuid NOT NULL REFERENCES plans(id),
    catalog_version_id uuid NOT NULL REFERENCES catalog_versions(id),
    provider_id        uuid NOT NULL,
    environment_id     uuid NOT NULL,
    metric_id          uuid REFERENCES metrics(id),
    charge_model       text NOT NULL CHECK (charge_model IN ('fixed', 'per_unit', 'tiered')),
    properties         jsonb NOT NULL
);

CREATE TABLE entitlement_grants (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id            uuid NOT NULL REFERENCES plans(id),
    catalog_version_id uuid NOT NULL REFERENCES catalog_versions(id),
    provider_id        uuid NOT NULL,
    environment_id     uuid NOT NULL,
    key                text NOT NULL,
    value_type         text NOT NULL CHECK (value_type IN ('boolean', 'numeric', 'period')),
    value              jsonb NOT NULL,
    UNIQUE (plan_id, key)
);

CREATE TABLE subscriptions (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id         uuid NOT NULL,
    environment_id      uuid NOT NULL,
    external_id         text NOT NULL,
    customer_account_id uuid NOT NULL REFERENCES customer_accounts(id),
    catalog_version_id  uuid NOT NULL REFERENCES catalog_versions(id),
    plan_id             uuid NOT NULL REFERENCES plans(id),
    status              text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'terminated')),
    started_at          timestamptz NOT NULL DEFAULT now(),
    terminated_at       timestamptz,
    UNIQUE (provider_id, environment_id, external_id)
);

-- Append-only usage ledger. Ingestion rows are never updated or deleted;
-- corrections are reversal rows pointing at the original via reverses_id.
CREATE TABLE usage_events (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id         uuid NOT NULL,
    environment_id      uuid NOT NULL,
    transaction_id      text NOT NULL,
    kind                text NOT NULL DEFAULT 'ingestion' CHECK (kind IN ('ingestion', 'reversal')),
    metric_code         text NOT NULL,
    customer_account_id uuid NOT NULL REFERENCES customer_accounts(id),
    subscription_id     uuid NOT NULL REFERENCES subscriptions(id),
    event_timestamp     timestamptz NOT NULL,
    properties          jsonb NOT NULL DEFAULT '{}',
    payload_hash        text NOT NULL,
    reverses_id         uuid REFERENCES usage_events(id),
    reason              text,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id, environment_id, transaction_id)
);

-- At most one reversal per original event.
CREATE UNIQUE INDEX usage_events_one_reversal ON usage_events (reverses_id) WHERE reverses_id IS NOT NULL;

CREATE TABLE entitlement_overrides (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id    uuid NOT NULL,
    environment_id uuid NOT NULL,
    subscription_id uuid NOT NULL REFERENCES subscriptions(id),
    key            text NOT NULL,
    value_type     text NOT NULL CHECK (value_type IN ('boolean', 'numeric', 'period')),
    value          jsonb NOT NULL,
    expires_at     timestamptz,
    reason         text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (subscription_id, key)
);

CREATE TABLE entitlement_snapshots (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id         uuid NOT NULL,
    environment_id      uuid NOT NULL,
    customer_account_id uuid NOT NULL REFERENCES customer_accounts(id),
    subscription_id     uuid NOT NULL REFERENCES subscriptions(id),
    catalog_version_id  uuid NOT NULL,
    payload             jsonb NOT NULL,
    computed_at         timestamptz NOT NULL DEFAULT now()
);

-- Relay retry scheduling for the billing adapter failure semantics.
ALTER TABLE outbox_events ADD COLUMN next_attempt_at timestamptz;

CREATE INDEX idx_customer_accounts_tenant ON customer_accounts (provider_id, environment_id, created_at);
CREATE INDEX idx_subscriptions_tenant ON subscriptions (provider_id, environment_id, started_at);
CREATE INDEX idx_subscriptions_customer ON subscriptions (customer_account_id, status);
CREATE INDEX idx_usage_events_tenant ON usage_events (provider_id, environment_id, created_at);
CREATE INDEX idx_entitlement_snapshots_customer ON entitlement_snapshots (customer_account_id, computed_at);

-- Grants for the runtime role (created out of band, as in 0002). Usage
-- events and entitlement snapshots are insert-only for the app role;
-- immutability is additionally enforced by trigger below.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON customer_accounts, catalog_versions,
            metrics, plans, prices, entitlement_grants, subscriptions,
            entitlement_overrides TO platform_app;
        GRANT SELECT, INSERT ON usage_events, entitlement_snapshots TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE customer_accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE customer_accounts FORCE ROW LEVEL SECURITY;
ALTER TABLE catalog_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE catalog_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE metrics ENABLE ROW LEVEL SECURITY;
ALTER TABLE metrics FORCE ROW LEVEL SECURITY;
ALTER TABLE plans ENABLE ROW LEVEL SECURITY;
ALTER TABLE plans FORCE ROW LEVEL SECURITY;
ALTER TABLE prices ENABLE ROW LEVEL SECURITY;
ALTER TABLE prices FORCE ROW LEVEL SECURITY;
ALTER TABLE entitlement_grants ENABLE ROW LEVEL SECURITY;
ALTER TABLE entitlement_grants FORCE ROW LEVEL SECURITY;
ALTER TABLE subscriptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE subscriptions FORCE ROW LEVEL SECURITY;
ALTER TABLE usage_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE usage_events FORCE ROW LEVEL SECURITY;
ALTER TABLE entitlement_overrides ENABLE ROW LEVEL SECURITY;
ALTER TABLE entitlement_overrides FORCE ROW LEVEL SECURITY;
ALTER TABLE entitlement_snapshots ENABLE ROW LEVEL SECURITY;
ALTER TABLE entitlement_snapshots FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON customer_accounts
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)));

CREATE POLICY tenant_isolation ON catalog_versions
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)));

CREATE POLICY tenant_isolation ON metrics
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)));

CREATE POLICY tenant_isolation ON plans
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)));

CREATE POLICY tenant_isolation ON prices
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)));

CREATE POLICY tenant_isolation ON entitlement_grants
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)));

CREATE POLICY tenant_isolation ON subscriptions
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)));

CREATE POLICY tenant_isolation ON usage_events
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)));

CREATE POLICY tenant_isolation ON entitlement_overrides
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)));

CREATE POLICY tenant_isolation ON entitlement_snapshots
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)));

-- Catalog content (metrics, plans, prices, entitlement_grants) is mutable
-- only while its parent catalog version is a draft. Prices and grants
-- resolve the version through their plan. Like the audit trigger, these
-- fire for every role including the table owner.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION require_draft_catalog() RETURNS trigger AS $fn$
DECLARE
    v_version_id uuid;
    v_state      text;
BEGIN
    IF TG_TABLE_NAME IN ('prices', 'entitlement_grants') THEN
        SELECT p.catalog_version_id INTO v_version_id
        FROM plans p
        WHERE p.id = COALESCE(NEW.plan_id, OLD.plan_id);
    ELSE
        v_version_id := COALESCE(NEW.catalog_version_id, OLD.catalog_version_id);
    END IF;
    SELECT cv.state INTO v_state FROM catalog_versions cv WHERE cv.id = v_version_id;
    IF v_state IS DISTINCT FROM 'draft' THEN
        RAISE EXCEPTION 'catalog version % is %: % on % requires a draft version', v_version_id, v_state, TG_OP, TG_TABLE_NAME;
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER metrics_draft_only
    BEFORE INSERT OR UPDATE OR DELETE ON metrics
    FOR EACH ROW EXECUTE FUNCTION require_draft_catalog();

CREATE TRIGGER plans_draft_only
    BEFORE INSERT OR UPDATE OR DELETE ON plans
    FOR EACH ROW EXECUTE FUNCTION require_draft_catalog();

CREATE TRIGGER prices_draft_only
    BEFORE INSERT OR UPDATE OR DELETE ON prices
    FOR EACH ROW EXECUTE FUNCTION require_draft_catalog();

CREATE TRIGGER entitlement_grants_draft_only
    BEFORE INSERT OR UPDATE OR DELETE ON entitlement_grants
    FOR EACH ROW EXECUTE FUNCTION require_draft_catalog();

-- A catalog version row itself is immutable except for its lifecycle
-- columns (state + transition timestamps); DELETE is allowed only while
-- the version is still a draft.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION catalog_versions_guard() RETURNS trigger AS $fn$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.state <> 'draft' THEN
            RAISE EXCEPTION 'catalog version % is %: DELETE allowed only for drafts', OLD.id, OLD.state;
        END IF;
        RETURN OLD;
    END IF;
    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.provider_id IS DISTINCT FROM OLD.provider_id
       OR NEW.environment_id IS DISTINCT FROM OLD.environment_id
       OR NEW.version IS DISTINCT FROM OLD.version
       OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'catalog_versions is immutable: only state, validated_at, published_at and retired_at may change';
    END IF;
    RETURN NEW;
END;
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER catalog_versions_immutable
    BEFORE UPDATE OR DELETE ON catalog_versions
    FOR EACH ROW EXECUTE FUNCTION catalog_versions_guard();

-- Subscriptions are pinned to their catalog version, plan and customer
-- forever; only status and terminated_at may change.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION subscription_pin_guard() RETURNS trigger AS $fn$
BEGIN
    IF NEW.catalog_version_id IS DISTINCT FROM OLD.catalog_version_id
       OR NEW.plan_id IS DISTINCT FROM OLD.plan_id
       OR NEW.customer_account_id IS DISTINCT FROM OLD.customer_account_id THEN
        RAISE EXCEPTION 'subscription % is pinned: catalog_version_id, plan_id and customer_account_id cannot change', OLD.id;
    END IF;
    RETURN NEW;
END;
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER subscriptions_pinned
    BEFORE UPDATE ON subscriptions
    FOR EACH ROW EXECUTE FUNCTION subscription_pin_guard();

-- usage_events is append-only (same style as audit_events).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION reject_usage_mutation() RETURNS trigger AS $fn$
BEGIN
    RAISE EXCEPTION 'usage_events is append-only: % not allowed', TG_OP;
END;
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER usage_events_append_only
    BEFORE UPDATE OR DELETE ON usage_events
    FOR EACH ROW EXECUTE FUNCTION reject_usage_mutation();

-- +goose Down
DROP TRIGGER IF EXISTS usage_events_append_only ON usage_events;
DROP FUNCTION IF EXISTS reject_usage_mutation();
DROP TRIGGER IF EXISTS subscriptions_pinned ON subscriptions;
DROP FUNCTION IF EXISTS subscription_pin_guard();
DROP TRIGGER IF EXISTS catalog_versions_immutable ON catalog_versions;
DROP FUNCTION IF EXISTS catalog_versions_guard();
DROP TRIGGER IF EXISTS entitlement_grants_draft_only ON entitlement_grants;
DROP TRIGGER IF EXISTS prices_draft_only ON prices;
DROP TRIGGER IF EXISTS plans_draft_only ON plans;
DROP TRIGGER IF EXISTS metrics_draft_only ON metrics;
DROP FUNCTION IF EXISTS require_draft_catalog();

DROP POLICY IF EXISTS tenant_isolation ON entitlement_snapshots;
DROP POLICY IF EXISTS tenant_isolation ON entitlement_overrides;
DROP POLICY IF EXISTS tenant_isolation ON usage_events;
DROP POLICY IF EXISTS tenant_isolation ON subscriptions;
DROP POLICY IF EXISTS tenant_isolation ON entitlement_grants;
DROP POLICY IF EXISTS tenant_isolation ON prices;
DROP POLICY IF EXISTS tenant_isolation ON plans;
DROP POLICY IF EXISTS tenant_isolation ON metrics;
DROP POLICY IF EXISTS tenant_isolation ON catalog_versions;
DROP POLICY IF EXISTS tenant_isolation ON customer_accounts;

DROP TABLE IF EXISTS entitlement_snapshots;
DROP TABLE IF EXISTS entitlement_overrides;
DROP TABLE IF EXISTS usage_events;
DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS entitlement_grants;
DROP TABLE IF EXISTS prices;
DROP TABLE IF EXISTS plans;
DROP TABLE IF EXISTS metrics;
DROP TABLE IF EXISTS catalog_versions;
DROP TABLE IF EXISTS customer_accounts;

ALTER TABLE outbox_events DROP COLUMN IF EXISTS next_attempt_at;
