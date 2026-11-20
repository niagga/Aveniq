-- Aveniq — the photo of a voucher: coverage, modality and professional at issue (E6/T54).
--
-- A voucher keeps what the practice knew when it was issued: how the patient was covered, in
-- which modality the session happened and which professional gave it. A later change to the
-- patient, the appointment or the professional does not rewrite the voucher, the same way the
-- buyer snapshot of 0014 is kept.
--
-- Vouchers issued before this migration carry no photo: their columns stay NULL and the
-- application does not invent one. The checks that require a photo are NOT VALID, so they
-- are enforced on every new row and do not refuse the rows that already exist.
--
-- One issued voucher per appointment: a session is billed once. A void frees the appointment,
-- because the void is final and the session can be billed again with a new number.

-- +goose Up

ALTER TABLE vouchers
    ADD COLUMN coverage_kind        text NULL,
    ADD COLUMN insurer_id           uuid NULL,
    ADD COLUMN insurer_name         text NULL,
    ADD COLUMN modality             text NULL,
    ADD COLUMN professional_user_id uuid NULL,
    ADD COLUMN professional_name    text NULL;

ALTER TABLE vouchers
    ADD CONSTRAINT vouchers_coverage_known
        CHECK (coverage_kind IS NULL OR coverage_kind IN ('private', 'insurer')),
    ADD CONSTRAINT vouchers_coverage_needs_patient
        CHECK (coverage_kind IS NULL OR patient_id IS NOT NULL),
    ADD CONSTRAINT vouchers_coverage_complete
        CHECK (coverage_kind IS NULL
            OR (coverage_kind = 'private' AND insurer_id IS NULL AND insurer_name IS NULL)
            OR (coverage_kind = 'insurer' AND insurer_id IS NOT NULL AND insurer_name IS NOT NULL)),
    ADD CONSTRAINT vouchers_insurer_only_with_coverage
        CHECK (coverage_kind IS NOT NULL OR (insurer_id IS NULL AND insurer_name IS NULL)),
    ADD CONSTRAINT vouchers_modality_known
        CHECK (modality IS NULL OR modality IN ('online', 'presencial')),
    ADD CONSTRAINT vouchers_professional_complete
        CHECK ((professional_user_id IS NULL) = (professional_name IS NULL));

-- A foreign key with a NULL column is not checked (MATCH SIMPLE), so a voucher without an
-- insurer or a professional passes; the ones that carry one must belong to the practice.
ALTER TABLE vouchers
    ADD CONSTRAINT vouchers_insurer_same_tenant
        FOREIGN KEY (tenant_id, insurer_id) REFERENCES insurers (tenant_id, id),
    ADD CONSTRAINT vouchers_professional_same_tenant
        FOREIGN KEY (tenant_id, professional_user_id) REFERENCES users (tenant_id, id);

-- Required for the rows written from now on (NOT VALID: the rows issued before 0020 stay).
ALTER TABLE vouchers
    ADD CONSTRAINT vouchers_patient_photo
        CHECK (patient_id IS NULL OR coverage_kind IS NOT NULL) NOT VALID,
    ADD CONSTRAINT vouchers_appointment_photo
        CHECK (appointment_id IS NULL
            OR (coverage_kind IS NOT NULL AND modality IS NOT NULL AND professional_user_id IS NOT NULL)) NOT VALID;

CREATE UNIQUE INDEX vouchers_one_issued_per_appointment
    ON vouchers (tenant_id, appointment_id)
    WHERE appointment_id IS NOT NULL AND status = 'issued';

-- The guard of 0014 compares the columns of an issued voucher and refuses a change to any of
-- them. The photo is part of what was issued, so the comparison has to name it too.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION vouchers_guard()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'vouchers are never deleted; void them instead'
            USING ERRCODE = 'restrict_violation';
    END IF;

    IF OLD.status = 'voided' THEN
        RAISE EXCEPTION 'a voided voucher is final'
            USING ERRCODE = 'restrict_violation';
    END IF;

    IF ROW(NEW.id, NEW.tenant_id, NEW.number, NEW.patient_id, NEW.appointment_id,
           NEW.concept, NEW.amount_minor, NEW.currency, NEW.practice_name,
           NEW.buyer_name, NEW.buyer_tax_id, NEW.buyer_fiscal_condition,
           NEW.coverage_kind, NEW.insurer_id, NEW.insurer_name, NEW.modality,
           NEW.professional_user_id, NEW.professional_name,
           NEW.issued_by, NEW.issued_at, NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.id, OLD.tenant_id, OLD.number, OLD.patient_id, OLD.appointment_id,
           OLD.concept, OLD.amount_minor, OLD.currency, OLD.practice_name,
           OLD.buyer_name, OLD.buyer_tax_id, OLD.buyer_fiscal_condition,
           OLD.coverage_kind, OLD.insurer_id, OLD.insurer_name, OLD.modality,
           OLD.professional_user_id, OLD.professional_name,
           OLD.issued_by, OLD.issued_at, OLD.created_at) THEN
        RAISE EXCEPTION 'an issued voucher is immutable except for its void'
            USING ERRCODE = 'restrict_violation';
    END IF;

    RETURN NEW;
END;
$function$;
-- +goose StatementEnd

-- +goose Down

DROP INDEX IF EXISTS vouchers_one_issued_per_appointment;

-- Restore the guard of 0014 first: it does not name the columns this migration drops.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION vouchers_guard()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'vouchers are never deleted; void them instead'
            USING ERRCODE = 'restrict_violation';
    END IF;

    IF OLD.status = 'voided' THEN
        RAISE EXCEPTION 'a voided voucher is final'
            USING ERRCODE = 'restrict_violation';
    END IF;

    IF ROW(NEW.id, NEW.tenant_id, NEW.number, NEW.patient_id, NEW.appointment_id,
           NEW.concept, NEW.amount_minor, NEW.currency, NEW.practice_name,
           NEW.buyer_name, NEW.buyer_tax_id, NEW.buyer_fiscal_condition,
           NEW.issued_by, NEW.issued_at, NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.id, OLD.tenant_id, OLD.number, OLD.patient_id, OLD.appointment_id,
           OLD.concept, OLD.amount_minor, OLD.currency, OLD.practice_name,
           OLD.buyer_name, OLD.buyer_tax_id, OLD.buyer_fiscal_condition,
           OLD.issued_by, OLD.issued_at, OLD.created_at) THEN
        RAISE EXCEPTION 'an issued voucher is immutable except for its void'
            USING ERRCODE = 'restrict_violation';
    END IF;

    RETURN NEW;
END;
$function$;
-- +goose StatementEnd

ALTER TABLE vouchers
    DROP CONSTRAINT IF EXISTS vouchers_appointment_photo,
    DROP CONSTRAINT IF EXISTS vouchers_patient_photo,
    DROP CONSTRAINT IF EXISTS vouchers_professional_same_tenant,
    DROP CONSTRAINT IF EXISTS vouchers_insurer_same_tenant;

ALTER TABLE vouchers
    DROP COLUMN IF EXISTS professional_name,
    DROP COLUMN IF EXISTS professional_user_id,
    DROP COLUMN IF EXISTS modality,
    DROP COLUMN IF EXISTS insurer_name,
    DROP COLUMN IF EXISTS insurer_id,
    DROP COLUMN IF EXISTS coverage_kind;
