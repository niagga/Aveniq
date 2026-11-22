-- Aveniq — E7/T57: reports:read:own, the professional's own figures.
--
-- The accountant and the administrator read the figures of the whole practice with
-- reports:read. The professional reads only what concerns the sessions they gave: how much
-- was billed for them and what they attended and has not been billed yet. It is a narrower
-- permission, not a smaller copy of reports:read, so the report API can tell the two scopes
-- apart from the matrix alone and the voucher list stays out of the professional's reach.
--
-- Owner holds every permission by definition. The administrator and the accountant keep
-- reports:read and do not need the narrower one, because the wider scope already contains it.

-- +goose Up

INSERT INTO permissions (code, domain, description) VALUES
    ('reports:read:own', 'reports', 'Read the billing and activity figures of the professional''s own sessions. Never the voucher list.');

-- Owner holds every permission by definition, including the one just added.
INSERT INTO role_permissions (role_code, permission_code)
SELECT 'owner', code FROM permissions
WHERE code NOT IN (SELECT permission_code FROM role_permissions WHERE role_code = 'owner');

INSERT INTO role_permissions (role_code, permission_code) VALUES
    ('professional', 'reports:read:own');

-- +goose Down

-- The grants go with the permission: role_permissions references it ON DELETE CASCADE.
DELETE FROM permissions WHERE code = 'reports:read:own';
