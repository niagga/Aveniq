-- Agenda. Tenant comes from app.tenant_id. Overlap is a database property
-- (appointments_no_overlap); this file just inserts and lets 23P01 surface.

-- name: CreateAppointment :one
INSERT INTO appointments (
    id, tenant_id, professional_user_id, patient_id, series_id,
    starts_at, ends_at, status, notes, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetAppointment :one
SELECT *
FROM appointments
WHERE id = $1;

-- name: ListAppointmentsInRange :many
-- Overlapping the requested window: a slot that starts before `to` and ends after `from`.
SELECT *
FROM appointments
WHERE starts_at < sqlc.arg('range_to')
  AND ends_at > sqlc.arg('range_from')
ORDER BY starts_at ASC, id ASC
LIMIT 500;

-- name: ListAppointmentsInRangeForProfessional :many
SELECT *
FROM appointments
WHERE professional_user_id = sqlc.arg('professional_user_id')
  AND starts_at < sqlc.arg('range_to')
  AND ends_at > sqlc.arg('range_from')
ORDER BY starts_at ASC, id ASC
LIMIT 500;

-- name: UpdateAppointmentStatus :one
UPDATE appointments
SET status = $2,
    updated_at = $3
WHERE id = $1
RETURNING *;

-- name: CreateAppointmentSeries :one
INSERT INTO appointment_series (
    id, tenant_id, professional_user_id, patient_id,
    weekday, local_start_minute, duration_minutes, starts_on, until_on,
    created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetAppointmentSeries :one
SELECT *
FROM appointment_series
WHERE id = $1;

-- name: UpdateAppointmentSeries :one
UPDATE appointment_series
SET weekday = $2,
    local_start_minute = $3,
    duration_minutes = $4,
    starts_on = $5,
    until_on = $6,
    updated_at = $7
WHERE id = $1
RETURNING *;

-- name: DeleteScheduledOfSeriesFrom :exec
-- Attended and individually cancelled rows stay. That is L14: a series edit must
-- not rewrite a session that already happened.
DELETE FROM appointments
WHERE series_id = $1
  AND status = 'scheduled'
  AND starts_at >= $2;

-- name: CancelScheduledOfSeries :exec
UPDATE appointments
SET status = 'cancelled',
    updated_at = $2
WHERE series_id = $1
  AND status = 'scheduled';
