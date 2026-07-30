-- +goose Up
-- Phase 0 core schema: Region / Cell / Provider / Environment / Credential /
-- Audit / Outbox / Inbox / dual-domain commerce accounts.

CREATE TABLE regions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code         text NOT NULL UNIQUE,
    jurisdiction text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE cells (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    region_id       uuid NOT NULL REFERENCES regions(id),
    code            text NOT NULL UNIQUE,
    cell_type       text NOT NULL CHECK (cell_type IN ('shared', 'dedicated')),
    status          text NOT NULL DEFAULT 'active',
    capacity_limits jsonb NOT NULL DEFAULT '{}',
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE providers (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            text NOT NULL UNIQUE,
    name            text NOT NULL,
    home_region_id  uuid NOT NULL REFERENCES regions(id),
    cell_id         uuid REFERENCES cells(id),
    lifecycle_state text NOT NULL CHECK (lifecycle_state IN (
        'REGISTERED', 'TEST_ACTIVE', 'LIVE_REVIEW', 'LIVE_ACTIVE',
        'RESTRICTED', 'SUSPENDED', 'OFFBOARDING'
    )),
    sla_tier        text NOT NULL DEFAULT 'standard',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE environments (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id uuid NOT NULL REFERENCES providers(id),
    kind        text NOT NULL CHECK (kind IN ('test', 'live')),
    status      text NOT NULL DEFAULT 'active',
    issuer      text NOT NULL UNIQUE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id, kind)
);

CREATE TABLE credentials (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id    uuid NOT NULL REFERENCES providers(id),
    environment_id uuid NOT NULL REFERENCES environments(id),
    name           text NOT NULL,
    key_prefix     text NOT NULL,
    key_hash       text NOT NULL UNIQUE,
    scopes         text[] NOT NULL DEFAULT '{}',
    allowed_cidrs  text[],
    expires_at     timestamptz,
    revoked_at     timestamptz,
    last_used_at   timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now()
);

-- Append-only audit log. UPDATE/DELETE are revoked from the app role and
-- rejected by trigger (0002_rls.sql) as defense in depth.
CREATE TABLE audit_events (
    id             bigserial PRIMARY KEY,
    provider_id    uuid,
    environment_id uuid,
    actor_type     text NOT NULL,
    actor_id       text NOT NULL,
    action         text NOT NULL,
    target_type    text,
    target_id      text,
    metadata       jsonb NOT NULL DEFAULT '{}',
    request_id     text,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE outbox_events (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id    uuid NOT NULL,
    environment_id uuid NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id   text NOT NULL,
    event_type     text NOT NULL,
    payload        jsonb NOT NULL,
    payload_hash   text NOT NULL,
    transaction_id text NOT NULL,
    status         text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'published', 'failed')),
    attempts       int NOT NULL DEFAULT 0,
    created_at     timestamptz NOT NULL DEFAULT now(),
    published_at   timestamptz,
    UNIQUE (provider_id, environment_id, transaction_id)
);

CREATE TABLE inbox_events (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id       uuid NOT NULL,
    environment_id    uuid NOT NULL,
    source            text NOT NULL,
    external_event_id text NOT NULL,
    payload           jsonb NOT NULL,
    received_at       timestamptz NOT NULL DEFAULT now(),
    processed_at      timestamptz,
    UNIQUE (provider_id, environment_id, source, external_event_id)
);

-- Dual commerce domains: platform rows may be environment-less and are never
-- visible to providers (enforced by RLS in 0002_rls.sql).
CREATE TABLE commerce_accounts (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    domain         text NOT NULL CHECK (domain IN ('platform', 'provider')),
    provider_id    uuid REFERENCES providers(id),
    environment_id uuid REFERENCES environments(id),
    display_name   text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_credentials_key_hash ON credentials (key_hash);
CREATE INDEX idx_audit_events_tenant ON audit_events (provider_id, environment_id, created_at);
CREATE INDEX idx_outbox_events_status ON outbox_events (status, created_at);

-- Seed: one region, one shared cell, one platform-domain commerce account.
INSERT INTO regions (code, jurisdiction) VALUES ('cn-shanghai', 'CN');

INSERT INTO cells (region_id, code, cell_type, status, capacity_limits)
SELECT id, 'cn-shanghai-shared-1', 'shared', 'active', '{"max_providers": 1000}'::jsonb
FROM regions WHERE code = 'cn-shanghai';

INSERT INTO commerce_accounts (domain, provider_id, environment_id, display_name)
VALUES ('platform', NULL, NULL, 'platform');

-- +goose Down
DROP TABLE IF EXISTS commerce_accounts;
DROP TABLE IF EXISTS inbox_events;
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS credentials;
DROP TABLE IF EXISTS environments;
DROP TABLE IF EXISTS providers;
DROP TABLE IF EXISTS cells;
DROP TABLE IF EXISTS regions;
