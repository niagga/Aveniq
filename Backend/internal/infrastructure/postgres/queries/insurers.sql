-- Mutuales (insurers) of a practice. Same rule as the other tenant tables: the tenant
-- comes from app.tenant_id, never from a WHERE clause, and a statement without tenant
-- context sees zero rows and writes none.

-- name: CreateInsurer :one
INSERT INTO insurers (id, tenant_id, name, tax_id, is_active, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetInsurerByID :one
SELECT *
FROM insurers
WHERE id = $1;

-- name: ListInsurers :many
-- ActiveOnly ($1) hides the deactivated mutuals. The catalog of one practice is small,
-- so the list is not paged.
SELECT *
FROM insurers
WHERE (NOT $1::boolean OR is_active)
ORDER BY lower(name) ASC, id ASC;

-- name: UpdateInsurer :one
UPDATE insurers
SET name       = $2,
    tax_id     = $3,
    is_active  = $4,
    updated_at = $5
WHERE id = $1
RETURNING *;
