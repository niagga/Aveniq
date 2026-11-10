-- name: CreateTenant :one
INSERT INTO tenants (id, slug, name, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)
RETURNING *;

-- name: GetTenantBySlug :one
SELECT *
FROM tenants
WHERE slug = $1;

-- name: ListTenants :many
SELECT *
FROM tenants
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;
