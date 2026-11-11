-- IAM queries: users. Every statement runs under row-level security, so the
-- tenant comes from app.tenant_id (set per transaction by Transactor.WithTenant),
-- never from a WHERE clause. A session without tenant context sees zero rows and
-- can write none — the policies fail closed.

-- name: CreateUser :one
INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, is_active, email_verified_at, mfa_secret, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = $1;

-- name: ListUsers :many
SELECT *
FROM users
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: UpdateUser :one
UPDATE users
SET password_hash = $2,
    full_name = $3,
    role_code = $4,
    is_active = $5,
    email_verified_at = $6,
    mfa_secret = $7,
    updated_at = $8
WHERE id = $1
RETURNING *;

-- name: DeleteUser :execrows
DELETE FROM users
WHERE id = $1;
