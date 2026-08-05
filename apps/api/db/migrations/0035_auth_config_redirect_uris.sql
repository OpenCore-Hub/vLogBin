-- +goose Up
-- 0035: Add redirect_uris column to provider_auth_configs so the Console
-- can display OIDC application redirect URIs without calling ZITADEL on
-- every list. Existing rows get an empty array default.

ALTER TABLE provider_auth_configs
    ADD COLUMN IF NOT EXISTS redirect_uris jsonb NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE provider_auth_configs
    DROP COLUMN IF EXISTS redirect_uris;
