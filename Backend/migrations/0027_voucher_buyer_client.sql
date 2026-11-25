-- Aveniq — E8/T73: the buyer of a voucher is the billing client (S26).
--
-- From T71 every patient has exactly one client, who pays. 0014 stamped the buyer from
-- the patient's record; this migration links every voucher to the client it belongs to
-- and lets the issuing code stamp the buyer from the client instead.
--
-- Old vouchers keep the snapshot they were issued with: the update below only sets the
-- client link, never the buyer columns. The immutability trigger of 0020 still freezes
-- the buyer snapshot; this migration extends it to freeze the new column too.
--
-- A loose charge keeps no patient and no client: the new column follows patient_id
-- exactly, with the same two-sided check rule the buyer snapshot has.

-- +goose Up

ALTER TABLE vouchers ADD COLUMN client_id uuid NULL;

-- Every voucher with a patient gets the client of its patient: 0026 gave every patient
-- exactly one, so the link is complete for vouchers with a patient and NULL for the
-- loose charges. The snapshot columns are not touched.
--
-- The guard of 0020 refuses any update of a voided voucher ('a voided voucher is final'),
-- and it would refuse this backfill too: a database with a voided receipt could never
-- migrate. The trigger is disabled for the one statement that only fills the new column
-- and enabled again right after; the version re-created below is the final one.
ALTER TABLE vouchers DISABLE TRIGGER vouchers_guard;

UPDATE vouchers v
SET client_id = p.client_id
FROM patients p
WHERE v.tenant_id = p.tenant_id
  AND v.patient_id = p.id;

ALTER TABLE vouchers ENABLE TRIGGER vouchers_guard;

-- client_id is present exactly when patient_id is: the mirror of the buyer snapshot
-- rule of 0014, now in the payer direction.
ALTER TABLE vouchers
    ADD CONSTRAINT vouchers_client_of_patient
        CHECK (client_id IS NULL OR patient_id IS NOT NULL),
    ADD CONSTRAINT vouchers_client_needs_patient
        CHECK (patient_id IS NULL OR client_id IS NOT NULL);

ALTER TABLE vouchers
    ADD CONSTRAINT vouchers_client_same_tenant
        FOREIGN KEY (tenant_id, client_id) REFERENCES clients (tenant_id, id);

CREATE INDEX vouchers_client_idx
    ON vouchers (tenant_id, client_id, issued_at DESC);

-- The guard compares every column an issued voucher may never lose. The client link is
-- part of what was issued (T73), so the comparison names it too.
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

    IF ROW(NEW.id, NEW.tenant_id, NEW.number, NEW.patient_id, NEW.client_id, NEW.appointment_id,
           NEW.concept, NEW.amount_minor, NEW.currency, NEW.practice_name,
           NEW.buyer_name, NEW.buyer_tax_id, NEW.buyer_fiscal_condition,
           NEW.coverage_kind, NEW.insurer_id, NEW.insurer_name, NEW.modality,
           NEW.professional_user_id, NEW.professional_name,
           NEW.issued_by, NEW.issued_at, NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.id, OLD.tenant_id, OLD.number, OLD.patient_id, OLD.client_id, OLD.appointment_id,
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

ALTER TABLE vouchers
    DROP CONSTRAINT IF EXISTS vouchers_client_same_tenant,
    DROP CONSTRAINT IF EXISTS vouchers_client_of_patient,
    DROP CONSTRAINT IF EXISTS vouchers_client_needs_patient;

DROP INDEX IF EXISTS vouchers_client_idx;

ALTER TABLE vouchers DROP COLUMN IF EXISTS client_id;

-- The column is gone first: now the guard of 0020, which does not name client_id, is the
-- frozen shape of the voucher again.
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
