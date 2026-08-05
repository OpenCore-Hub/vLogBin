-- +goose Up
-- 0036: Add display name to provider_auth_configs so the Console can show
-- meaningful OIDC application names without calling ZITADEL on every list.
-- Existing rows (created before this migration) get a fallback label.

ALTER TABLE provider_auth_configs
    ADD COLUMN IF NOT EXISTS name text NOT NULL DEFAULT '';

UPDATE provider_auth_configs
SET name = zitadel_client_id
WHERE name = '';

-- +goose Down
ALTER TABLE provider_auth_configs
    DROP COLUMN IF EXISTS name;
