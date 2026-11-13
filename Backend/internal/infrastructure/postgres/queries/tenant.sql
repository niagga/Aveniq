-- name: CreateTenant :one
INSERT INTO tenants (id, slug, name, status, trial_ends_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $6)
RETURNING *;

-- name: GetTenantBySlug :one
SELECT *
FROM tenants
WHERE slug = $1;

-- name: GetTenantByID :one
-- The identity-scoped read: a session may only fetch the practice its token names.
SELECT *
FROM tenants
WHERE id = $1;
