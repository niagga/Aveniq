-- Clinical grants. A grant lets a professional other than the treating one read a
-- patient's file and clinical record. Tenant isolation is row-level security, as
-- for every tenant table: the queries carry no tenant filter.

-- name: CreateClinicalGrant :one
INSERT INTO clinical_grants (id, tenant_id, patient_id, professional_user_id, granted_by, granted_at, revoked_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetClinicalGrantByID :one
SELECT *
FROM clinical_grants
WHERE id = $1;

-- name: HasLiveClinicalGrant :one
SELECT EXISTS (
    SELECT 1
    FROM clinical_grants
    WHERE patient_id = $1
      AND professional_user_id = $2
      AND revoked_at IS NULL
);

-- name: ListClinicalGrantsByPatient :many
SELECT *
FROM clinical_grants
WHERE patient_id = $1
ORDER BY granted_at DESC, id ASC;

-- name: RevokeClinicalGrant :one
UPDATE clinical_grants
SET revoked_at = $2
WHERE id = $1
RETURNING *;