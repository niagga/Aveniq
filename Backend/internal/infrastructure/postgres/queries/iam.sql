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

-- Sessions: the refresh grants. Same rule as the users above — the tenant comes
-- from app.tenant_id, never from a WHERE clause, and a statement without tenant
-- context sees zero rows and writes none.

-- name: CreateSession :one
INSERT INTO sessions (id, tenant_id, user_id, refresh_hash, user_agent, ip, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetSessionByID :one
SELECT *
FROM sessions
WHERE id = $1;

-- name: GetSessionByRefreshHash :one
SELECT *
FROM sessions
WHERE refresh_hash = $1;

-- name: RotateSession :execrows
-- Revoke the presented grant and point it at its successor. The `revoked_at IS
-- NULL` guard is what makes rotation single-use: a second attempt affects zero
-- rows instead of overwriting the first successor.
UPDATE sessions
SET revoked_at = $2,
    replaced_by = $3
WHERE id = $1
  AND revoked_at IS NULL;

-- name: RevokeSession :execrows
UPDATE sessions
SET revoked_at = $2
WHERE id = $1
  AND revoked_at IS NULL;

-- name: RevokeSessionChain :execrows
-- Reuse detection kills the whole chain, not the presented leaf: the stolen copy
-- and the legitimate one compete, and nothing here can tell which is which.
WITH RECURSIVE chain (id, replaced_by) AS (
    SELECT s.id, s.replaced_by
    FROM sessions AS s
    WHERE s.id = $1
    UNION ALL
    SELECT s.id, s.replaced_by
    FROM sessions AS s
    JOIN chain AS c ON s.id = c.replaced_by
)
UPDATE sessions
SET revoked_at = $2
WHERE sessions.id IN (SELECT c.id FROM chain AS c)
  AND sessions.revoked_at IS NULL;

-- name: SessionUserVisible :one
-- The cross-tenant guard of decision 7: a foreign key check inside `sessions`
-- does not go through row-level security, so knowing a user UUID from another
-- tenant would be enough to build a session on it. This makes the user's
-- visibility in the open scope a precondition of the insert.
SELECT u.id
FROM users AS u
WHERE u.id = $1;
