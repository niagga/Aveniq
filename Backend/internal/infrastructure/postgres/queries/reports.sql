-- name: ReportBilledVouchers :many
-- What the practice billed: the issued vouchers of the window. A voided voucher bills nothing,
-- so it is not here. The cuts are the photo taken at issue, never the patient's data of today:
-- an old voucher or a loose charge has none of them and does not match a filter on one.
SELECT issued_at,
       amount_minor,
       currency,
       coverage_kind,
       insurer_id,
       insurer_name,
       modality,
       professional_user_id,
       professional_name
FROM vouchers
WHERE status = 'issued'
  AND issued_at >= sqlc.arg('from_at')
  AND issued_at < sqlc.arg('to_at')
  AND (sqlc.narg('professional_id')::uuid IS NULL OR professional_user_id = sqlc.narg('professional_id'))
  AND (sqlc.narg('coverage')::text IS NULL OR coverage_kind = sqlc.narg('coverage'))
  AND (sqlc.narg('insurer_id')::uuid IS NULL OR insurer_id = sqlc.narg('insurer_id'))
  AND (sqlc.narg('modality')::text IS NULL OR modality = sqlc.narg('modality'))
ORDER BY issued_at, number;

-- name: ReportPendingSessions :many
-- What the practice attended and did not bill: the attended sessions of the window with no
-- issued voucher. A voided voucher bills nothing, so its session is here again. The cuts are the
-- session's professional and modality and the patient's coverage, mutual and currency today.
SELECT a.starts_at,
       a.professional_user_id,
       u.full_name AS professional_name,
       a.modality,
       p.coverage_kind,
       p.insurer_id,
       i.name AS insurer_name,
       p.billing_currency
FROM appointments a
JOIN users u
  ON u.tenant_id = a.tenant_id AND u.id = a.professional_user_id
JOIN patients p
  ON p.tenant_id = a.tenant_id AND p.id = a.patient_id
LEFT JOIN insurers i
  ON i.tenant_id = p.tenant_id AND i.id = p.insurer_id
WHERE a.status = 'attended'
  AND a.starts_at >= sqlc.arg('from_at')
  AND a.starts_at < sqlc.arg('to_at')
  AND NOT EXISTS (
    SELECT 1
      FROM vouchers v
     WHERE v.tenant_id = a.tenant_id
       AND v.appointment_id = a.id
       AND v.status = 'issued'
  )
  AND (sqlc.narg('professional_id')::uuid IS NULL OR a.professional_user_id = sqlc.narg('professional_id'))
  AND (sqlc.narg('coverage')::text IS NULL OR p.coverage_kind = sqlc.narg('coverage'))
  AND (sqlc.narg('insurer_id')::uuid IS NULL OR p.insurer_id = sqlc.narg('insurer_id'))
  AND (sqlc.narg('modality')::text IS NULL OR a.modality = sqlc.narg('modality'))
ORDER BY a.starts_at, a.id;
