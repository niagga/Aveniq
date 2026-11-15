-- Aveniq — a voucher's appointment belongs to the voucher's patient (E4 follow-up).
--
-- 0014 let a voucher point at any appointment of its own practice. A receipt could then name
-- one patient on the buyer line and another patient's appointment on the same row. This makes
-- the pair one fact, in the schema: the appointment must be a row of the voucher's patient.
--
-- Not an edit of 0014: that migration is applied and shipped, so the rule comes as a new step.

-- +goose Up

-- Composite key over (practice, appointment, patient), the same pattern as 0007 and 0009.
CREATE UNIQUE INDEX appointments_tenant_id_id_patient_unique
    ON appointments (tenant_id, id, patient_id);

ALTER TABLE vouchers DROP CONSTRAINT vouchers_appointment_same_tenant;

ALTER TABLE vouchers ADD CONSTRAINT vouchers_appointment_of_patient
    FOREIGN KEY (tenant_id, appointment_id, patient_id)
    REFERENCES appointments (tenant_id, id, patient_id);

-- A foreign key with a NULL column is not checked (MATCH SIMPLE), so a voucher with an
-- appointment and no patient would slip past it. This check closes that gap.
ALTER TABLE vouchers ADD CONSTRAINT vouchers_appointment_requires_patient
    CHECK (appointment_id IS NULL OR patient_id IS NOT NULL);

-- +goose Down

ALTER TABLE vouchers DROP CONSTRAINT IF EXISTS vouchers_appointment_requires_patient;
ALTER TABLE vouchers DROP CONSTRAINT IF EXISTS vouchers_appointment_of_patient;

ALTER TABLE vouchers ADD CONSTRAINT vouchers_appointment_same_tenant
    FOREIGN KEY (tenant_id, appointment_id) REFERENCES appointments (tenant_id, id);

DROP INDEX IF EXISTS appointments_tenant_id_id_patient_unique;
