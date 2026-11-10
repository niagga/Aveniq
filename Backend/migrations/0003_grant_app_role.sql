-- Aveniq — the application role, and why it exists.
--
-- PostgreSQL lets superusers and roles with BYPASSRLS ignore *every* row-level
-- security policy, and the owner of a table bypasses it unless FORCE is set.
-- The stack was initialised with a single `aveniq` role that is superuser and
-- BYPASSRLS, so for a while the policies added in 0002 filtered nothing while
-- appearing to work: every tenant saw every row.
--
-- The application therefore connects with `aveniq_app`, which is neither a
-- superuser nor a bypass role nor the owner of any table. RLS applies to it.
-- Migrations keep running as the owner role.

-- +goose Up

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'aveniq_app') THEN
        CREATE ROLE aveniq_app NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
    END IF;
END
$$;
-- +goose StatementEnd

-- The role needs to reach the schema and the tables, and nothing else.
GRANT USAGE ON SCHEMA public TO aveniq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO aveniq_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO aveniq_app;

-- Tables created by later migrations must inherit the same grants, otherwise
-- every new table would be invisible to the application until someone remembers
-- to grant again.
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO aveniq_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO aveniq_app;

-- +goose Down

-- The role itself is intentionally not dropped: it is a cluster-wide object and
-- other databases in the same cluster may depend on it.
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLES FROM aveniq_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    REVOKE USAGE, SELECT ON SEQUENCES FROM aveniq_app;
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM aveniq_app;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM aveniq_app;
REVOKE USAGE ON SCHEMA public FROM aveniq_app;
