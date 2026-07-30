-- Creates the application role used by the platform API at runtime.
-- Migrations run as the superuser; the app connects as platform_app so that
-- row level security applies to every query (superusers bypass RLS).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
        CREATE ROLE platform_app LOGIN PASSWORD 'platform_app_dev';
    END IF;
END $$;

GRANT CONNECT ON DATABASE platform TO platform_app;
GRANT USAGE ON SCHEMA public TO platform_app;
