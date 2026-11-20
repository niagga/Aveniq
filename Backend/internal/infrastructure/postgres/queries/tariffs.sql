-- Tarifario (tariffs) of a practice. Same rule as the other tenant tables: the tenant comes
-- from app.tenant_id, never from a WHERE clause, and a statement without tenant context
-- sees zero rows and writes none.

-- name: CreateTariff :one
INSERT INTO tariffs (
    id, tenant_id, professional_user_id, coverage, insurer_id, modality,
    billing_currency, valid_from, insurer_minor, patient_minor, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: UpdateTariffAmounts :one
UPDATE tariffs
SET insurer_minor = $2,
    patient_minor = $3,
    updated_at    = $4
WHERE id = $1
RETURNING *;

-- name: DeleteTariff :execrows
DELETE FROM tariffs
WHERE id = $1;

-- name: GetTariffByID :one
SELECT *
FROM tariffs
WHERE id = $1;

-- name: ListTariffs :many
-- The screen shows who the price is for and, for a mutual, which one. The names come from
-- the same practice (RLS), so a reader without users:directory or patients:read still sees
-- them: the accountant reads the tariff without the staff directory.
SELECT t.*,
       u.full_name AS professional_name,
       i.name      AS insurer_name
FROM tariffs t
JOIN users u
  ON u.tenant_id = t.tenant_id AND u.id = t.professional_user_id
LEFT JOIN insurers i
  ON i.tenant_id = t.tenant_id AND i.id = t.insurer_id
ORDER BY lower(u.full_name) ASC, t.coverage ASC, lower(i.name) ASC NULLS FIRST,
         t.modality ASC, t.billing_currency ASC, t.valid_from DESC, t.id ASC;

-- name: TariffInForce :one
-- The tariff that prices one session: for its professional, coverage, mutual, modality and
-- currency, the latest validity that starts on or before the session's civil day. A particular
-- tariff has no mutual, so the comparison is IS NOT DISTINCT FROM.
SELECT *
FROM tariffs
WHERE professional_user_id = sqlc.arg('professional_user_id')
  AND coverage = sqlc.arg('coverage')
  AND insurer_id IS NOT DISTINCT FROM sqlc.narg('insurer_id')::uuid
  AND modality = sqlc.arg('modality')
  AND billing_currency = sqlc.arg('billing_currency')::char(3)
  AND valid_from <= sqlc.arg('day')::date
ORDER BY valid_from DESC
LIMIT 1;
