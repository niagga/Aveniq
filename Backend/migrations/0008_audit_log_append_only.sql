-- Aveniq — the audit trail is append-only for the application.
--
-- The policy on audit_log is FOR ALL, which means the application role could update
-- or delete its own rows. That is not a security boundary anybody relies on, but it is
-- a foot-gun: a bug that "cleans up" would erase the trail it is supposed to leave,
-- and the first thing an auditor asks for is the thing that got deleted.
--
-- Revoking the privileges is what makes the intent structural. Reading and writing
-- stay: the application records events and reads them for the practice dashboard.
--
-- The tenant on every row comes from app.tenant_id, which is set by the tenant
-- transaction the event is written in. Rows with a NULL tenant are not reachable by
-- this role at all (L26 explains why that case exists and what it costs).

-- +goose Up

REVOKE UPDATE, DELETE ON audit_log FROM aveniq_app;

COMMENT ON TABLE audit_log IS
    'Append-only for the application role (0008 revokes UPDATE and DELETE). Clinical access is recorded here in the same transaction as the read, so an unaudited read cannot commit.';

-- +goose Down

GRANT UPDATE, DELETE ON audit_log TO aveniq_app;
