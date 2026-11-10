-- Aveniq — IAM: roles, permissions, users, sessions, invitations and the audit trail.
--
-- Multi-tenant isolation is enforced by PostgreSQL row-level security, not by the
-- application. The RLS section at the end of the Up block explains the reasoning;
-- read it before changing any policy.

-- +goose Up

-- ─────────────────────────────────────────────────────────────────────────────
-- Global catalogs
--
-- Roles and permissions are platform-wide. Per-tenant custom roles are a later
-- feature; keeping the matrix global means it can be reviewed in one place and
-- that a permission cannot mean something different per tenant.
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE permissions (
    code        text NOT NULL PRIMARY KEY,
    domain      text NOT NULL,
    description text NOT NULL,

    CONSTRAINT permissions_code_format CHECK (code ~ '^[a-z]+:[a-z:]+$'),
    CONSTRAINT permissions_domain_format CHECK (domain ~ '^[a-z]+$')
);

COMMENT ON TABLE permissions IS 'Platform-wide permission catalog. Codes are stable API surface.';

CREATE TABLE roles (
    code        text NOT NULL PRIMARY KEY,
    name        text NOT NULL,
    description text NOT NULL
);

CREATE TABLE role_permissions (
    role_code       text NOT NULL REFERENCES roles (code) ON DELETE CASCADE,
    permission_code text NOT NULL REFERENCES permissions (code) ON DELETE CASCADE,

    PRIMARY KEY (role_code, permission_code)
);

-- ─────────────────────────────────────────────────────────────────────────────
-- Tenant-scoped tables
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE users (
    id                uuid        NOT NULL PRIMARY KEY,
    tenant_id         uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    email             text        NOT NULL,
    password_hash     text        NOT NULL,
    full_name         text        NOT NULL,
    role_code         text        NOT NULL REFERENCES roles (code),
    is_active         boolean     NOT NULL DEFAULT true,
    email_verified_at timestamptz NULL,
    mfa_secret        text        NULL,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL,

    -- The same human may belong to two practices, so uniqueness is per tenant.
    CONSTRAINT users_tenant_email_unique UNIQUE (tenant_id, email),

    -- Emails are stored canonical so lookups and uniqueness are case-insensitive
    -- without relying on citext or on the caller remembering to normalise.
    CONSTRAINT users_email_canonical CHECK (email = lower(btrim(email))),
    CONSTRAINT users_email_shape CHECK (email ~ '^[^@[:space:]]+@[^@[:space:]]+$'),
    CONSTRAINT users_full_name_not_blank CHECK (char_length(btrim(full_name)) > 0),
    CONSTRAINT users_password_hash_not_blank CHECK (char_length(btrim(password_hash)) > 0)
);

COMMENT ON COLUMN users.password_hash IS 'Argon2id encoded hash. Never a plaintext or reversible value.';
COMMENT ON COLUMN users.mfa_secret IS 'TOTP shared secret, encrypted at the application layer.';

CREATE TABLE sessions (
    id           uuid        NOT NULL PRIMARY KEY,
    tenant_id    uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    refresh_hash text        NOT NULL,
    user_agent   text        NULL,
    ip           inet        NULL,
    created_at   timestamptz NOT NULL,
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz NULL,

    -- When a refresh token is rotated, the old session is marked revoked and
    -- points at its successor. A token presented after that is a reuse signal.
    replaced_by  uuid        NULL REFERENCES sessions (id),

    CONSTRAINT sessions_refresh_hash_unique UNIQUE (refresh_hash),
    CONSTRAINT sessions_refresh_hash_not_blank CHECK (char_length(btrim(refresh_hash)) > 0),
    CONSTRAINT sessions_expiry_after_creation CHECK (expires_at > created_at)
);

COMMENT ON COLUMN sessions.refresh_hash IS 'SHA-256 of the refresh token. The token itself is never stored.';

CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE invitations (
    id          uuid        NOT NULL PRIMARY KEY,
    tenant_id   uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    email       text        NOT NULL,
    role_code   text        NOT NULL REFERENCES roles (code),
    token_hash  text        NOT NULL,
    invited_by  uuid        NULL REFERENCES users (id) ON DELETE SET NULL,
    expires_at  timestamptz NOT NULL,
    accepted_at timestamptz NULL,
    created_at  timestamptz NOT NULL,

    CONSTRAINT invitations_token_hash_unique UNIQUE (token_hash),
    CONSTRAINT invitations_token_hash_not_blank CHECK (char_length(btrim(token_hash)) > 0),
    CONSTRAINT invitations_email_canonical CHECK (email = lower(btrim(email))),
    CONSTRAINT invitations_email_shape CHECK (email ~ '^[^@[:space:]]+@[^@[:space:]]+$'),
    CONSTRAINT invitations_expiry_after_creation CHECK (expires_at > created_at)
);

CREATE INDEX invitations_tenant_email_idx ON invitations (tenant_id, email);

CREATE TABLE audit_log (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id  uuid        NULL REFERENCES tenants (id) ON DELETE SET NULL,

    -- Deliberately without a foreign key: the actor may be a support operator
    -- outside the tenant, or a row that was later deleted. Auditing must not
    -- fail because its subject disappeared.
    actor_id   uuid        NULL,

    action     text        NOT NULL,
    entity     text        NOT NULL,
    entity_id  text        NULL,
    ip         inet        NULL,
    metadata   jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL,

    CONSTRAINT audit_log_action_not_blank CHECK (char_length(btrim(action)) > 0),
    CONSTRAINT audit_log_entity_not_blank CHECK (char_length(btrim(entity)) > 0)
);

CREATE INDEX audit_log_tenant_created_at_idx ON audit_log (tenant_id, created_at DESC);

-- ─────────────────────────────────────────────────────────────────────────────
-- Seeds: permission catalog and role matrix
-- ─────────────────────────────────────────────────────────────────────────────

INSERT INTO permissions (code, domain, description) VALUES
    ('users:read',        'users',    'List and read the user accounts of the tenant.'),
    ('users:write',       'users',    'Create and update user accounts.'),
    ('users:manage',      'users',    'Invite users, change their role and deactivate them.'),
    ('patients:read',     'patients', 'Read the patient register.'),
    ('patients:write',    'patients', 'Create and update patients.'),
    ('clinical:read:own', 'clinical', 'Read clinical entries of the professional''s own patients.'),
    ('clinical:write:own','clinical', 'Write clinical entries for the professional''s own patients.'),
    ('clinical:read:any', 'clinical', 'Read clinical entries of every professional in the tenant.'),
    ('billing:read',      'billing',  'Read invoices and payments.'),
    ('billing:issue',     'billing',  'Issue invoices.'),
    ('billing:void',      'billing',  'Void an issued document with a credit note.'),
    ('reports:read',      'reports',  'Read accounting and activity reports.'),
    ('tenant:settings',   'tenant',   'Change tenant-level settings.');

INSERT INTO roles (code, name, description) VALUES
    ('owner',        'Owner',        'Full control of the tenant, including clinical records.'),
    ('admin',        'Administrator','Manages users, patients and settings. Never reads clinical records.'),
    ('professional', 'Professional', 'Treating professional: own patients and own clinical entries.'),
    ('assistant',    'Assistant',    'Front desk: patient register and billing overview.'),
    ('accountant',   'Accountant',   'Issues invoices and reads reports.');

-- The owner holds every permission by definition.
INSERT INTO role_permissions (role_code, permission_code)
SELECT 'owner', code FROM permissions;

-- The administrator deliberately does NOT receive any clinical:* permission.
-- The person who runs the system must not be able to read psychotherapy notes;
-- that separation is the reason the matrix exists at all.
INSERT INTO role_permissions (role_code, permission_code) VALUES
    ('admin',        'users:read'),
    ('admin',        'users:write'),
    ('admin',        'users:manage'),
    ('admin',        'patients:read'),
    ('admin',        'patients:write'),
    ('admin',        'billing:read'),
    ('admin',        'reports:read'),
    ('admin',        'tenant:settings'),

    ('professional', 'patients:read'),
    ('professional', 'patients:write'),
    ('professional', 'clinical:read:own'),
    ('professional', 'clinical:write:own'),
    ('professional', 'billing:read'),

    ('assistant',    'patients:read'),
    ('assistant',    'patients:write'),
    ('assistant',    'billing:read'),

    ('accountant',   'billing:read'),
    ('accountant',   'billing:issue'),
    ('accountant',   'billing:void'),
    ('accountant',   'reports:read');

-- ─────────────────────────────────────────────────────────────────────────────
-- Row-level security
--
-- Three rules make this work, and each has a silent failure mode if dropped:
--
--  1. FORCE ROW LEVEL SECURITY. In PostgreSQL the owner of a table bypasses its
--     own policies. The application role owns these tables in the local stack,
--     so without FORCE the policies would not run and everything would appear to
--     work while isolating nothing. In production the application must connect
--     with a role that does NOT own the tables, so that FORCE is the second line
--     of defence rather than the only one.
--
--  2. Fail closed. current_setting('app.tenant_id', true) yields NULL when the
--     setting was never made, and '' when it was made empty. Both must resolve
--     to no comparison at all: NULLIF(..., '')::uuid is NULL, and
--     tenant_id = NULL is NULL, which matches no row. A session without tenant
--     context therefore sees nothing instead of everything.
--
--  3. WITH CHECK next to USING. USING filters what can be read or targeted;
--     WITH CHECK constrains what may be written. With USING alone a bug could
--     INSERT a row into another tenant and the policy would not object.
--
-- The context is set per transaction:
--     SET LOCAL app.tenant_id = '<uuid>';
-- It must never be set with plain SET, because connections come from a pool and
-- are reused by requests from different tenants.
-- ─────────────────────────────────────────────────────────────────────────────

-- users
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;

CREATE POLICY users_tenant_isolation ON users
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- sessions
ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions FORCE ROW LEVEL SECURITY;

CREATE POLICY sessions_tenant_isolation ON sessions
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- invitations
ALTER TABLE invitations ENABLE ROW LEVEL SECURITY;
ALTER TABLE invitations FORCE ROW LEVEL SECURITY;

CREATE POLICY invitations_tenant_isolation ON invitations
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- audit_log
ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log FORCE ROW LEVEL SECURITY;

CREATE POLICY audit_log_tenant_isolation ON audit_log
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- +goose Down

DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS invitations;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS permissions;
