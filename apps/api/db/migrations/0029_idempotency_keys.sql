-- +goose Up
-- Stripe-style Idempotency-Key support. Clients send an Idempotency-Key
-- header on mutating requests (POST/PUT/PATCH/DELETE); the first execution's
-- response is cached and replayed verbatim for identical retries, protecting
-- against duplicate billing or duplicate resource creation caused by network
-- retries. See internal/httpapi/idempotency.go for the middleware contract.
--
-- scope identifies the authenticated caller: 'provider:<uuid>' for tenant
-- requests, 'operator:<sub>' for operator requests. key_hash is sha256(key)
-- so raw keys never touch the database. Rows expire after IDEMPOTENCY_TTL
-- (default 24h) and are purged by the idempotency sweeper.
CREATE TABLE idempotency_keys (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scope           text NOT NULL,
    key_hash        bytea NOT NULL,
    method          text NOT NULL CHECK (method IN ('POST', 'PUT', 'PATCH', 'DELETE')),
    path            text NOT NULL,
    status          text NOT NULL DEFAULT 'in_progress'
                    CHECK (status IN ('in_progress', 'completed')),
    response_status integer CHECK (response_status IS NULL OR response_status >= 100),
    content_type    text,
    response_body   bytea,
    request_id      text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    UNIQUE (scope, key_hash, method, path)
);

CREATE INDEX idx_idempotency_keys_expires_at ON idempotency_keys (expires_at);

-- Grants for the runtime role (created out of band, as in 0002/0004).
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON idempotency_keys TO platform_app;
    END IF;
END $$;
-- +goose StatementEnd

-- RLS: tenant requests only see their own provider scope; operator requests
-- (app.is_operator = 'on') bypass. The middleware writes rows under
-- WithTenant/WithOperator, matching the SET LOCAL context the policies read.
ALTER TABLE idempotency_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE idempotency_keys FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON idempotency_keys
    FOR ALL
    USING (current_setting('app.is_operator', true) = 'on'
           OR scope = 'provider:' || current_setting('app.provider_id', true))
    WITH CHECK (current_setting('app.is_operator', true) = 'on'
                OR scope = 'provider:' || current_setting('app.provider_id', true));

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation ON idempotency_keys;
DROP TABLE IF EXISTS idempotency_keys;
