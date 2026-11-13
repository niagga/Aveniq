-- Aveniq — E3/T34: the staff directory permission.
--
-- users:read returns the full account (email, status, role). A professional who
-- opens a clinical entry needs only the author's name, and users:read would also
-- hand them the colleagues' emails. users:directory is the narrow view: id,
-- full name and role, nothing else. It is what lets a note show who wrote it
-- without widening the account listing (S18, S19).

-- +goose Up

INSERT INTO permissions (code, domain, description) VALUES
    ('users:directory', 'users', 'Read the name and role of the practice''s staff, without email or status.');

-- Owner holds every permission by definition.
INSERT INTO role_permissions (role_code, permission_code) VALUES
    ('owner',        'users:directory'),
    ('professional', 'users:directory'),
    ('assistant',    'users:directory');

-- +goose Down

DELETE FROM role_permissions WHERE permission_code = 'users:directory';
DELETE FROM permissions WHERE code = 'users:directory';
