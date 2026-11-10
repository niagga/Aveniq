-- Runs once, on an empty data directory, before the application starts.
--
-- The application must not connect as the database owner: PostgreSQL lets
-- superusers and BYPASSRLS roles ignore every row-level security policy, so the
-- multi-tenant isolation would silently do nothing. Migrations keep running as
-- the owner; the API and the worker use this role.
--
-- Local development credentials only. A deployed environment must supply the
-- password through its secret store.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'aveniq_app') THEN
        CREATE ROLE aveniq_app NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
    END IF;
    ALTER ROLE aveniq_app LOGIN PASSWORD 'aveniq_app_dev';
END
$$;
