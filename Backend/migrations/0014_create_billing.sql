-- Aveniq — internal vouchers (E4/T39).
--
-- What the practice records to keep its own accounts. These vouchers do not go to the
-- tax authority, carry no authorisation code and are not sent anywhere (S7). Electronic
-- invoicing is a later decision and will add its own tables instead of bending these.
--
-- Three rules live in the schema, not only in the domain, so no code path can skip them:
--   1. Numbers are consecutive per practice and never reused. A counter row, incremented
--      inside the issuing transaction, gives that: a rolled-back issue rolls the counter back too.
--   2. An issued voucher is final. Only its void may change, and a voucher is never deleted:
--      a correction is a void, the same way a clinical note's correction is a new note.
--   3. The buyer and the practice name are a snapshot taken at issue. A later edit to the
--      patient does not rewrite what was issued.

-- +goose Up

-- Composite key so a voucher can point at an appointment of its own practice (same
-- pattern as patients and users in 0007 and 0009).
CREATE UNIQUE INDEX appointments_tenant_id_id_unique ON appointments (tenant_id, id);

-- ─────────────────────────────────────────────────────────────────────────────
-- The counter: one row per practice, locked by the issuing transaction.
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE voucher_counters (
    tenant_id   uuid    NOT NULL PRIMARY KEY REFERENCES tenants (id) ON DELETE CASCADE,
    last_number integer NOT NULL DEFAULT 0,

    CONSTRAINT voucher_counters_non_negative CHECK (last_number >= 0)
);

-- ─────────────────────────────────────────────────────────────────────────────
-- The vouchers
--
-- Money is an integer count of minor units (centavos): a float would round a receipt.
-- The patient and the appointment are optional on purpose: a voucher may be a loose
-- charge with no patient behind it. When it has a patient, the buyer snapshot is
-- mandatory, and when it has none, the snapshot must be empty.
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE vouchers (
    id             uuid        NOT NULL PRIMARY KEY,
    tenant_id      uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    number         integer     NOT NULL,

    patient_id     uuid        NULL,
    appointment_id uuid        NULL,

    concept        text        NOT NULL,
    amount_minor   bigint      NOT NULL,
    currency       char(3)     NOT NULL,

    -- Snapshot at issue. Never refreshed from the patient afterwards.
    practice_name          text NOT NULL,
    buyer_name             text NULL,
    buyer_tax_id           text NULL,
    buyer_fiscal_condition text NULL,

    status         text        NOT NULL DEFAULT 'issued',
    issued_by      uuid        NOT NULL,
    issued_at      timestamptz NOT NULL,

    voided_by      uuid        NULL,
    voided_at      timestamptz NULL,
    void_reason    text        NULL,

    created_at     timestamptz NOT NULL,
    updated_at     timestamptz NOT NULL,

    CONSTRAINT vouchers_number_per_practice UNIQUE (tenant_id, number),
    CONSTRAINT vouchers_number_positive CHECK (number > 0),

    CONSTRAINT vouchers_patient_same_tenant
        FOREIGN KEY (tenant_id, patient_id) REFERENCES patients (tenant_id, id),
    CONSTRAINT vouchers_appointment_same_tenant
        FOREIGN KEY (tenant_id, appointment_id) REFERENCES appointments (tenant_id, id),
    CONSTRAINT vouchers_issuer_same_tenant
        FOREIGN KEY (tenant_id, issued_by) REFERENCES users (tenant_id, id),
    CONSTRAINT vouchers_voider_same_tenant
        FOREIGN KEY (tenant_id, voided_by) REFERENCES users (tenant_id, id),

    CONSTRAINT vouchers_concept_not_blank CHECK (char_length(btrim(concept)) BETWEEN 1 AND 200),
    CONSTRAINT vouchers_amount_positive CHECK (amount_minor > 0),
    CONSTRAINT vouchers_currency_iso CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT vouchers_practice_name_not_blank CHECK (char_length(btrim(practice_name)) > 0),

    CONSTRAINT vouchers_buyer_snapshot_with_patient
        CHECK (patient_id IS NULL OR buyer_name IS NOT NULL),
    CONSTRAINT vouchers_buyer_snapshot_only_with_patient
        CHECK (patient_id IS NOT NULL
               OR (buyer_name IS NULL AND buyer_tax_id IS NULL AND buyer_fiscal_condition IS NULL)),

    CONSTRAINT vouchers_status_known CHECK (status IN ('issued', 'voided')),
    CONSTRAINT vouchers_void_coherent CHECK (
        (status = 'issued'
            AND voided_by IS NULL AND voided_at IS NULL AND void_reason IS NULL)
     OR (status = 'voided'
            AND voided_by IS NOT NULL AND voided_at IS NOT NULL
            AND char_length(btrim(void_reason)) > 0)
    ),
    CONSTRAINT vouchers_void_after_issue CHECK (voided_at IS NULL OR voided_at >= issued_at),
    CONSTRAINT vouchers_updated_after_creation CHECK (updated_at >= created_at)
);

CREATE INDEX vouchers_range_idx
    ON vouchers (tenant_id, issued_at DESC);

CREATE INDEX vouchers_patient_idx
    ON vouchers (tenant_id, patient_id, issued_at DESC);

-- ─────────────────────────────────────────────────────────────────────────────
-- Immutability: a trigger, because a CHECK cannot compare the old row with the new one.
--
-- Allowed: issued -> voided, with its void fields. Everything else is refused: a
-- voided voucher is final, an issued one is frozen except for its void, and no voucher
-- is ever deleted. updated_at may move.
-- ─────────────────────────────────────────────────────────────────────────────

-- +goose StatementBegin
CREATE FUNCTION vouchers_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
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
$$;
-- +goose StatementEnd

CREATE TRIGGER vouchers_guard
    BEFORE UPDATE OR DELETE ON vouchers
    FOR EACH ROW EXECUTE FUNCTION vouchers_guard();

-- ─────────────────────────────────────────────────────────────────────────────
-- Row-level security, same shape as 0002, 0004, 0007 and 0009
-- ─────────────────────────────────────────────────────────────────────────────

ALTER TABLE voucher_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE voucher_counters FORCE ROW LEVEL SECURITY;
CREATE POLICY voucher_counters_tenant_isolation ON voucher_counters
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

ALTER TABLE vouchers ENABLE ROW LEVEL SECURITY;
ALTER TABLE vouchers FORCE ROW LEVEL SECURITY;
CREATE POLICY vouchers_tenant_isolation ON vouchers
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- +goose Down

DROP TRIGGER IF EXISTS vouchers_guard ON vouchers;
DROP FUNCTION IF EXISTS vouchers_guard();
DROP TABLE IF EXISTS vouchers;
DROP TABLE IF EXISTS voucher_counters;
DROP INDEX IF EXISTS appointments_tenant_id_id_unique;
