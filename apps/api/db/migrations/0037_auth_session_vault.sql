-- +goose Up
-- 0037: Server-side OIDC token vault for vLogBin web sessions.
-- The web app keeps only a small identity cookie; access/refresh tokens are
-- encrypted at rest here and fetched by the web backend on demand.

CREATE TABLE IF NOT EXISTS auth_session_vault (
    id            text PRIMARY KEY,
    user_sub      text NOT NULL,
    email         text NOT NULL DEFAULT '',
    name          text NOT NULL DEFAULT '',
    roles         jsonb NOT NULL DEFAULT '[]',
    workspace_id  text NOT NULL DEFAULT '',
    env           text NOT NULL DEFAULT '',
    access_token  text NOT NULL,
    refresh_token text NOT NULL DEFAULT '',
    token_exp     bigint NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS auth_session_vault_expires_at_idx
    ON auth_session_vault (expires_at);

GRANT SELECT, INSERT, UPDATE, DELETE ON auth_session_vault TO platform_app;

-- +goose Down
DROP TABLE IF EXISTS auth_session_vault;
