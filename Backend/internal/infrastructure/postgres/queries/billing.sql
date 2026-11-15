-- Internal vouchers (E4). Tenant comes from app.tenant_id. The consecutive
-- number is reserved in the same transaction as the insert: a rolled-back
-- issue rolls the counter back too, so there is no gap and no reuse.

-- name: NextVoucherNumber :one
INSERT INTO voucher_counters (tenant_id, last_number)
VALUES ($1, 1)
ON CONFLICT (tenant_id) DO UPDATE
SET last_number = voucher_counters.last_number + 1
RETURNING last_number;

-- name: CreateVoucher :one
INSERT INTO vouchers (
    id, tenant_id, number, patient_id, appointment_id,
    concept, amount_minor, currency,
    practice_name, buyer_name, buyer_tax_id, buyer_fiscal_condition,
    status, issued_by, issued_at,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8,
    $9, $10, $11, $12,
    $13, $14, $15,
    $16, $17
)
RETURNING *;

-- name: VoidVoucher :one
UPDATE vouchers
SET status = 'voided',
    voided_by = $2,
    voided_at = $3,
    void_reason = $4,
    updated_at = $5
WHERE id = $1
  AND status = 'issued'
RETURNING *;

-- name: GetVoucher :one
SELECT *
FROM vouchers
WHERE id = $1;

-- name: ListVouchers :many
SELECT *
FROM vouchers
WHERE (sqlc.narg('issued_from')::timestamptz IS NULL OR issued_at >= sqlc.narg('issued_from'))
  AND (sqlc.narg('issued_to')::timestamptz IS NULL OR issued_at < sqlc.narg('issued_to'))
  AND (sqlc.narg('patient_id')::uuid IS NULL OR patient_id = sqlc.narg('patient_id'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY issued_at DESC, number DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: GetBillingSubject :one
-- The billing identity of one patient. Only the fields a receipt prints: the clinical
-- record and the contact details are not selected.
SELECT id, full_name, document_number, tax_id, fiscal_condition
FROM patients
WHERE id = $1;

-- name: SearchBillingSubjects :many
-- Active patients whose name, document or tax id contains the pattern. The caller
-- wraps the fragment in % and escapes the LIKE wildcards; the page is bounded here.
SELECT id, full_name, document_number, tax_id, fiscal_condition
FROM patients
WHERE is_active
  AND (full_name ILIKE sqlc.arg('pattern')
       OR document_number ILIKE sqlc.arg('pattern')
       OR tax_id ILIKE sqlc.arg('pattern'))
ORDER BY full_name, id
LIMIT sqlc.arg('lim');
