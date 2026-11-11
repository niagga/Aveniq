-- Patients. Same rule as the other tenant tables: the tenant comes from
-- app.tenant_id, never from a WHERE clause, and a statement without tenant context
-- sees zero rows and writes none.

-- name: CreatePatient :one
INSERT INTO patients (
    id, tenant_id, country, full_name, birth_date,
    document_type, document_number, document_normalized,
    phone, email, emergency_contact_name, emergency_contact_phone,
    tax_id, tax_id_kind, fiscal_condition, fiscal_address,
    administrative_notes, is_active, created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
RETURNING *;

-- name: GetPatientByID :one
SELECT *
FROM patients
WHERE id = $1;

-- name: UpdatePatient :one
UPDATE patients
SET country                = $2,
    full_name              = $3,
    birth_date             = $4,
    document_type          = $5,
    document_number        = $6,
    document_normalized    = $7,
    phone                  = $8,
    email                  = $9,
    emergency_contact_name = $10,
    emergency_contact_phone = $11,
    tax_id                 = $12,
    tax_id_kind            = $13,
    fiscal_condition       = $14,
    fiscal_address         = $15,
    administrative_notes   = $16,
    is_active              = $17,
    updated_at             = $18
WHERE id = $1
RETURNING *;

-- name: ListPatients :many
-- The search matches a fragment of the name or the exact normalized document: a
-- document is either found or not, and a partial match on an identifier is how one
-- patient's history is shown to another by mistake.
SELECT *
FROM patients
WHERE ($1::text = '' OR full_name ILIKE '%' || $1::text || '%' OR document_normalized = upper($1::text))
  AND (NOT $2::boolean OR is_active)
ORDER BY full_name ASC, id ASC
LIMIT $3 OFFSET $4;

-- name: GetTenantCountry :one
-- Resolved outside the tenant scope: `tenants` has no row-level security, and the
-- practice's country is the fallback for a patient that does not choose one.
SELECT country
FROM tenants
WHERE id = $1;

-- ─────────────────────────────────────────────────────────────────────────────
-- Catalogs. Global and read-only for the application role (0005 revoked writes):
-- a country is added by a migration, not by a request.
-- ─────────────────────────────────────────────────────────────────────────────

-- name: GetDocumentKind :one
SELECT country_code, code, label, pattern, normalizer, checksum
FROM document_kinds
WHERE country_code = $1 AND code = $2;

-- name: GetTaxIDKind :one
SELECT country_code, code, label, pattern, normalizer, checksum
FROM tax_id_kinds
WHERE country_code = $1 AND code = $2;

-- name: GetFiscalCondition :one
SELECT country_code, code, label
FROM fiscal_conditions
WHERE country_code = $1 AND code = $2;
