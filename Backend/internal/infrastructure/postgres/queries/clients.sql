-- Billing clients of a practice. Same tenant scope rule as the other tables.

-- name: CreateClient :one
INSERT INTO clients (
    id, tenant_id, display_name, legal_name, country,
    fiscal_condition, tax_id, tax_id_kind, fiscal_address,
    billing_currency, is_active, created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: GetClientByID :one
SELECT *
FROM clients
WHERE id = $1;

-- name: ListClients :many
SELECT *
FROM clients
WHERE (NOT $1::boolean OR is_active)
ORDER BY lower(display_name) ASC, id ASC;

-- name: UpdateClient :one
UPDATE clients
SET display_name     = $2,
    legal_name       = $3,
    country          = $4,
    fiscal_condition = $5,
    tax_id           = $6,
    tax_id_kind      = $7,
    fiscal_address   = $8,
    billing_currency = $9,
    is_active        = $10,
    updated_at       = $11
WHERE id = $1
RETURNING *;