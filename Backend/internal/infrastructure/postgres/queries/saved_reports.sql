-- Saved billing reports of a practice. Same rule as the other tenant tables: the tenant comes
-- from app.tenant_id, never from a WHERE clause, and a statement without tenant context sees
-- zero rows and writes none.

-- name: CreateSavedReport :one
INSERT INTO saved_reports (
    id, tenant_id, name, period, from_day, to_day, group_by,
    professional_user_id, coverage, insurer_id, modality,
    created_by, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: GetSavedReport :one
SELECT *
FROM saved_reports
WHERE id = $1;

-- name: UpdateSavedReport :one
-- What the report asks and when it changed. The author and the creation time stay.
UPDATE saved_reports
SET name                 = $2,
    period               = $3,
    from_day             = $4,
    to_day               = $5,
    group_by             = $6,
    professional_user_id = $7,
    coverage             = $8,
    insurer_id           = $9,
    modality             = $10,
    updated_at           = $11
WHERE id = $1
RETURNING *;

-- name: DeleteSavedReport :execrows
DELETE FROM saved_reports
WHERE id = $1;

-- name: ListSavedReports :many
-- The names come from the same practice (RLS), so a reader without the staff directory still sees
-- who saved a report and which professional and mutual it filters by.
SELECT r.*,
       author.full_name AS created_by_name,
       pro.full_name    AS professional_name,
       i.name           AS insurer_name
FROM saved_reports r
JOIN users author
  ON author.tenant_id = r.tenant_id AND author.id = r.created_by
LEFT JOIN users pro
  ON pro.tenant_id = r.tenant_id AND pro.id = r.professional_user_id
LEFT JOIN insurers i
  ON i.tenant_id = r.tenant_id AND i.id = r.insurer_id
ORDER BY lower(btrim(r.name)) ASC, r.id ASC;
