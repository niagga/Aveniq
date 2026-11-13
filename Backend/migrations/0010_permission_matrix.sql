-- Aveniq — E3/T33: the permission matrix that replaces the coarse role guard.
--
-- 0002 seeded roles and a first catalog. E2 then added the agenda, and E3
-- decided (S18–S23) that identification is shared, HC is not, the assistant
-- works the desk, and the accountant only sees billing. This migration adds
-- the missing verbs and rewrites the grants so the HTTP layer can ask
-- "does this role have this permission?" instead of "is it owner/admin/professional?".
--
-- Instance-level rules (which patient a professional treats, a grant of HC to
-- another professional) are not rows here. clinical:grant exists so T38 can
-- attach them without another catalog change.

-- +goose Up

INSERT INTO permissions (code, domain, description) VALUES
    ('clinical:grant',  'clinical', 'Authorise another professional of the practice to read this patient''s clinical record.'),
    ('schedule:read',   'schedule', 'Read appointments and series of the practice.'),
    ('schedule:write',  'schedule', 'Book, attend, cancel and rewrite appointments and series.'),
    ('tenant:read',     'tenant',   'Read the current practice (name, slug, timezone).');

-- Owner holds every permission by definition, including the ones just added.
INSERT INTO role_permissions (role_code, permission_code)
SELECT 'owner', code FROM permissions
WHERE code NOT IN (SELECT permission_code FROM role_permissions WHERE role_code = 'owner');

INSERT INTO role_permissions (role_code, permission_code) VALUES
    ('admin',        'schedule:read'),
    ('admin',        'schedule:write'),
    ('admin',        'tenant:read'),

    ('professional', 'clinical:grant'),
    ('professional', 'schedule:read'),
    ('professional', 'schedule:write'),
    ('professional', 'tenant:read'),

    ('assistant',    'schedule:read'),
    ('assistant',    'schedule:write'),
    ('assistant',    'tenant:read');

-- Billing is S2 and belongs to the accountant. A professional or assistant who
-- still held billing:read would look authorised for a domain that does not exist
-- yet and that S22 reserved.
DELETE FROM role_permissions
WHERE role_code IN ('professional', 'assistant')
  AND permission_code = 'billing:read';

-- +goose Down

INSERT INTO role_permissions (role_code, permission_code) VALUES
    ('professional', 'billing:read'),
    ('assistant',    'billing:read')
ON CONFLICT DO NOTHING;

DELETE FROM permissions
WHERE code IN ('clinical:grant', 'schedule:read', 'schedule:write', 'tenant:read');
