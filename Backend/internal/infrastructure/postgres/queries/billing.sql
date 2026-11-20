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
    coverage_kind, insurer_id, insurer_name, modality, professional_user_id, professional_name,
    status, issued_by, issued_at,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8,
    $9, $10, $11, $12,
    $13, $14, $15, $16, $17, $18,
    $19, $20, $21,
    $22, $23
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
SELECT id, full_name, document_number, tax_id, fiscal_condition, billing_currency
FROM patients
WHERE id = $1;

-- name: SearchBillingSubjects :many
-- Active patients whose name contains the pattern, or whose document or tax id contains
-- the identifier. The caller wraps the name fragment in % and escapes the LIKE wildcards;
-- the identifier is already letters and digits in upper case (billing.IdentifierFragment)
-- and is NULL when the fragment is too short to search identifiers by. Documents and tax
-- ids are compared with their separators stripped, so "30111222" finds "30.111.222" and
-- "20123456786" finds "20-12345678-6". The page is bounded here.
SELECT id, full_name, document_number, tax_id, fiscal_condition, billing_currency
FROM patients
WHERE is_active
  AND (full_name ILIKE sqlc.arg('pattern')
       OR (sqlc.narg('identifier')::text IS NOT NULL
           AND (strpos(regexp_replace(upper(document_normalized), '[^0-9A-Z]', '', 'g'), sqlc.narg('identifier')::text) > 0
                OR strpos(regexp_replace(upper(tax_id), '[^0-9A-Z]', '', 'g'), sqlc.narg('identifier')::text) > 0)))
ORDER BY full_name, id
LIMIT sqlc.arg('lim');

-- name: GetPatientBilling :one
-- How a patient is billed. The insurer name is read here, not from a second call, so the
-- photo of a voucher is taken from one consistent read.
SELECT p.coverage_kind,
       p.insurer_id,
       i.name AS insurer_name,
       p.billing_currency,
       p.is_active
FROM patients p
LEFT JOIN insurers i
  ON i.tenant_id = p.tenant_id AND i.id = p.insurer_id
WHERE p.id = $1;

-- name: GetBillingAppointment :one
-- One session, with its professional's name and the issued voucher that bills it, if any.
SELECT a.id,
       a.patient_id,
       a.professional_user_id,
       u.full_name AS professional_name,
       a.modality,
       a.starts_at,
       a.status,
       (SELECT v.id
          FROM vouchers v
         WHERE v.tenant_id = a.tenant_id
           AND v.appointment_id = a.id
           AND v.status = 'issued') AS billed_voucher_id
FROM appointments a
JOIN users u
  ON u.tenant_id = a.tenant_id AND u.id = a.professional_user_id
WHERE a.id = $1;

-- name: ListPatientAppointments :many
-- The newest sessions of one patient, for the emission screen. Cancelled ones are listed too:
-- the screen shows why a session cannot be billed.
SELECT a.id,
       a.patient_id,
       a.professional_user_id,
       u.full_name AS professional_name,
       a.modality,
       a.starts_at,
       a.status,
       (SELECT v.id
          FROM vouchers v
         WHERE v.tenant_id = a.tenant_id
           AND v.appointment_id = a.id
           AND v.status = 'issued') AS billed_voucher_id
FROM appointments a
JOIN users u
  ON u.tenant_id = a.tenant_id AND u.id = a.professional_user_id
WHERE a.patient_id = sqlc.arg('patient_id')
ORDER BY a.starts_at DESC, a.id
LIMIT sqlc.arg('lim');

-- name: GetBillingProfessional :one
-- An active professional of the practice: the only role a voucher may name.
SELECT id, full_name
FROM users
WHERE id = $1
  AND role_code = 'professional'
  AND is_active;

-- name: ListBillingProfessionals :many
SELECT id, full_name
FROM users
WHERE role_code = 'professional'
  AND is_active
ORDER BY lower(full_name), id;
