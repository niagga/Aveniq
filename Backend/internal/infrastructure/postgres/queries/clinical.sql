-- Clinical record and notes. The tenant comes from app.tenant_id, as everywhere
-- else. The two writes carry the tenant INSIDE their foreign keys (0007), so a row
-- cannot point at another practice's patient even if this file forgot a filter.

-- name: UpsertClinicalRecord :one
-- One frame per patient: an edit is an update, not a second row.
INSERT INTO clinical_records (id, tenant_id, patient_id, chief_complaint, background, objectives, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (tenant_id, patient_id) DO UPDATE
SET chief_complaint = EXCLUDED.chief_complaint,
    background      = EXCLUDED.background,
    objectives      = EXCLUDED.objectives,
    updated_at      = EXCLUDED.updated_at
RETURNING *;

-- name: GetClinicalRecordByPatient :one
SELECT *
FROM clinical_records
WHERE patient_id = $1;

-- name: CreateClinicalNote :one
INSERT INTO clinical_notes (id, tenant_id, patient_id, author_user_id, encountered_at, content, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListClinicalNotesByPatient :many
-- Most recent encounter first: the last session is what a professional opens before
-- the next one.
SELECT *
FROM clinical_notes
WHERE patient_id = $1
ORDER BY encountered_at DESC, id DESC
LIMIT $2 OFFSET $3;
