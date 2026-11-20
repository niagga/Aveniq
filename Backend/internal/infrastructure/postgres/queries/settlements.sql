-- Monthly settlements to the mutuales (E6/T55). Tenant comes from app.tenant_id, as in
-- billing.sql. A settlement and its sessions are written in one tenant transaction.

-- name: GetSettlementInsurer :one
-- One mutual of the practice, active or not. The settlement reads the name and the tax id here,
-- so the accountant does not need to read the insurers screen.
SELECT id, name, tax_id, is_active
FROM insurers
WHERE id = $1;

-- name: ListSettlementInsurers :many
-- The active mutuales of the practice, by name. The tax id tells the screen which ones can be
-- settled: a settlement needs the mutual's CUIT.
SELECT id, name, tax_id, is_active
FROM insurers
WHERE is_active
ORDER BY lower(name), id;

-- name: ListSettlableSessions :many
-- The attended sessions of one mutual's patients in a window, not yet liquidated by an active
-- settlement. Each carries the currency its patient is billed in, which the tariff is priced by.
SELECT a.id,
       a.patient_id,
       p.full_name AS patient_name,
       p.member_number,
       a.professional_user_id,
       u.full_name AS professional_name,
       a.modality,
       a.starts_at,
       p.billing_currency
FROM appointments a
JOIN patients p
  ON p.tenant_id = a.tenant_id AND p.id = a.patient_id
JOIN users u
  ON u.tenant_id = a.tenant_id AND u.id = a.professional_user_id
WHERE p.coverage_kind = 'insurer'
  AND p.insurer_id = sqlc.arg('insurer_id')
  AND a.status = 'attended'
  AND a.starts_at >= sqlc.arg('from_at')
  AND a.starts_at < sqlc.arg('to_at')
  AND NOT EXISTS (
      SELECT 1
        FROM settlement_items si
       WHERE si.tenant_id = a.tenant_id
         AND si.appointment_id = a.id
         AND si.released_at IS NULL
  )
ORDER BY a.starts_at, a.id;

-- name: CreateSettlement :one
INSERT INTO settlements (
    id, tenant_id, insurer_id, insurer_name, insurer_tax_id, practice_name,
    period_start, period_end, status, created_by, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, 'issued', $9, $10, $11
)
RETURNING *;

-- name: CreateSettlementItem :exec
INSERT INTO settlement_items (
    tenant_id, settlement_id, appointment_id,
    patient_id, patient_name, member_number,
    professional_user_id, professional_name, modality, starts_at,
    currency, amount_minor, copago_minor
) VALUES (
    $1, $2, $3,
    $4, $5, $6,
    $7, $8, $9, $10,
    $11, $12, $13
);

-- name: VoidSettlement :one
UPDATE settlements
SET status = 'voided',
    voided_by = $2,
    voided_at = $3,
    void_reason = $4,
    updated_at = $5
WHERE id = $1
  AND status = 'issued'
RETURNING *;

-- name: ReleaseSettlementItems :exec
-- The sessions of a voided settlement become billable again: their active item is released.
UPDATE settlement_items
SET released_at = $2
WHERE settlement_id = $1
  AND released_at IS NULL;

-- name: GetSettlement :one
SELECT *
FROM settlements
WHERE id = $1;

-- name: ListSettlements :many
SELECT *
FROM settlements
WHERE (sqlc.narg('insurer_id')::uuid IS NULL OR insurer_id = sqlc.narg('insurer_id'))
ORDER BY created_at DESC, id
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: ListSettlementItems :many
SELECT *
FROM settlement_items
WHERE settlement_id = ANY(sqlc.arg('settlement_ids')::uuid[])
ORDER BY settlement_id, starts_at, appointment_id;
