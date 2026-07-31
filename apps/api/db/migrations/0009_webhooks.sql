-- +goose Up
-- Webhook signing & delivery: provider registers an endpoint (URL + HMAC
-- secret); the platform signs each outbox event with HMAC-SHA256 and
-- delivers it. webhook_deliveries dedup (one delivery per event per
-- endpoint) and track retry / dead-letter state. Both tables are
-- tenant-scoped (provider_id + environment_id) and RLS-protected with the
-- same ::text comparison policy shape as 0004_billing_core.sql.

CREATE TABLE webhook_endpoints (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id     uuid NOT NULL,
    environment_id  uuid NOT NULL,
    url             text NOT NULL,
    secret          text NOT NULL,                       -- HMAC-SHA256 signing secret
    enabled         boolean NOT NULL DEFAULT true,
    events          text[] NOT NULL DEFAULT '{}',        -- event type filter, empty = all
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id, environment_id, url)
);

CREATE TABLE webhook_deliveries (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    endpoint_id       uuid NOT NULL REFERENCES webhook_endpoints(id),
    outbox_event_id   uuid NOT NULL,
    provider_id       uuid NOT NULL,
    environment_id    uuid NOT NULL,
    status            text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'delivered', 'failed', 'dead_letter')),
    attempts          int NOT NULL DEFAULT 0,
    response_status   int,
    response_body     text,
    next_attempt_at   timestamptz,
    delivered_at      timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (endpoint_id, outbox_event_id)               -- dedup: one delivery per event per endpoint
);

CREATE INDEX idx_webhook_endpoints_tenant ON webhook_endpoints (provider_id, environment_id, enabled);
CREATE INDEX idx_webhook_deliveries_pending ON webhook_deliveries (status, next_attempt_at) WHERE status = 'pending';
CREATE INDEX idx_webhook_deliveries_endpoint ON webhook_deliveries (endpoint_id);

-- Grants for the runtime role (created out of band, as in 0002/0004).
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON webhook_endpoints, webhook_deliveries TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE webhook_endpoints ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_endpoints FORCE ROW LEVEL SECURITY;
ALTER TABLE webhook_deliveries ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_deliveries FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON webhook_endpoints
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)));

CREATE POLICY tenant_isolation ON webhook_deliveries
    USING (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
           OR (provider_id::text = current_setting('app.provider_id', true)
               AND environment_id::text = current_setting('app.environment_id', true)));

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation ON webhook_deliveries;
DROP POLICY IF EXISTS tenant_isolation ON webhook_endpoints;

ALTER TABLE webhook_deliveries NO FORCE ROW LEVEL SECURITY;
ALTER TABLE webhook_deliveries DISABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_endpoints NO FORCE ROW LEVEL SECURITY;
ALTER TABLE webhook_endpoints DISABLE ROW LEVEL SECURITY;

DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhook_endpoints;
