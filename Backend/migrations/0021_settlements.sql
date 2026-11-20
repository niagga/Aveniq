-- Aveniq — monthly settlements to the mutuales (E6/T55, L61).
--
-- A settlement is what the practice bills one mutual for one month: the attended sessions of
-- its patients, valued at the mutual's share of the tariff in force on the day of each session.
-- It is the document the mutual pays against, so it keeps what was true when it was issued:
-- the mutual's name and tax id, the practice's name and the amount of every session.
--
-- Three rules live in the schema, not only in the application, so no code path can skip them:
--   1. A session is settled once. An item is active while released_at is NULL; a partial unique
--      index allows one active item per appointment, the same way one issued voucher bills a session.
--   2. A settlement is final. Only its void may change, and it is never deleted: a correction is a
--      void, the same way a voucher is corrected.
--   3. Voiding a settlement releases its sessions, and only then: an item is released when its
--      settlement is voided, and a released item is final.

-- +goose Up

-- ─────────────────────────────────────────────────────────────────────────────
-- The settlements: one per insurer and period, with the snapshot of who pays and who is paid.
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE settlements (
    id              uuid        NOT NULL PRIMARY KEY,
    tenant_id       uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,

    insurer_id      uuid        NOT NULL,
    insurer_name    text        NOT NULL,
    insurer_tax_id  text        NOT NULL,
    practice_name   text        NOT NULL,

    -- The first and the last day of the period, both in the practice's civil calendar.
    period_start    date        NOT NULL,
    period_end      date        NOT NULL,

    status          text        NOT NULL DEFAULT 'issued',
    created_by      uuid        NOT NULL,
    created_at      timestamptz NOT NULL,

    voided_by       uuid        NULL,
    voided_at       timestamptz NULL,
    void_reason     text        NULL,

    updated_at      timestamptz NOT NULL,

    CONSTRAINT settlements_insurer_same_tenant
        FOREIGN KEY (tenant_id, insurer_id) REFERENCES insurers (tenant_id, id),
    CONSTRAINT settlements_creator_same_tenant
        FOREIGN KEY (tenant_id, created_by) REFERENCES users (tenant_id, id),
    CONSTRAINT settlements_voider_same_tenant
        FOREIGN KEY (tenant_id, voided_by) REFERENCES users (tenant_id, id),

    CONSTRAINT settlements_insurer_name_not_blank CHECK (char_length(btrim(insurer_name)) > 0),
    CONSTRAINT settlements_tax_id_shape CHECK (insurer_tax_id ~ '^[0-9]{11}$'),
    CONSTRAINT settlements_practice_name_not_blank CHECK (char_length(btrim(practice_name)) > 0),
    CONSTRAINT settlements_period_whole_month CHECK (
        extract(day FROM period_start) = 1 AND period_end > period_start
    ),

    CONSTRAINT settlements_status_known CHECK (status IN ('issued', 'voided')),
    CONSTRAINT settlements_void_coherent CHECK (
        (status = 'issued'
            AND voided_by IS NULL AND voided_at IS NULL AND void_reason IS NULL)
     OR (status = 'voided'
            AND voided_by IS NOT NULL AND voided_at IS NOT NULL
            AND char_length(btrim(void_reason)) > 0)
    ),
    CONSTRAINT settlements_void_after_creation CHECK (voided_at IS NULL OR voided_at >= created_at),
    CONSTRAINT settlements_updated_after_creation CHECK (updated_at >= created_at)
);

-- Composite key so an item can only point at a settlement of its own practice (same pattern
-- as vouchers and appointments).
CREATE UNIQUE INDEX settlements_tenant_id_id_unique ON settlements (tenant_id, id);

CREATE INDEX settlements_tenant_created_idx
    ON settlements (tenant_id, created_at DESC);

-- ─────────────────────────────────────────────────────────────────────────────
-- The sessions of a settlement. The amount is the mutual's share, as valued at issue.
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE settlement_items (
    tenant_id             uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    settlement_id         uuid        NOT NULL,
    appointment_id        uuid        NOT NULL,

    -- Snapshot at issue: the mutual reads the patient's member number and name on its own statement.
    patient_id            uuid        NOT NULL,
    patient_name          text        NOT NULL,
    member_number         text        NOT NULL,

    professional_user_id  uuid        NOT NULL,
    professional_name     text        NOT NULL,
    modality              text        NOT NULL,
    starts_at             timestamptz NOT NULL,

    currency              char(3)     NOT NULL,
    amount_minor          bigint      NOT NULL,

    -- NULL while the settlement stands; set when it is voided.
    released_at           timestamptz NULL,

    PRIMARY KEY (tenant_id, settlement_id, appointment_id),

    CONSTRAINT settlement_items_settlement_same_tenant
        FOREIGN KEY (tenant_id, settlement_id) REFERENCES settlements (tenant_id, id),
    CONSTRAINT settlement_items_appointment_same_tenant
        FOREIGN KEY (tenant_id, appointment_id) REFERENCES appointments (tenant_id, id),
    CONSTRAINT settlement_items_patient_same_tenant
        FOREIGN KEY (tenant_id, patient_id) REFERENCES patients (tenant_id, id),
    CONSTRAINT settlement_items_professional_same_tenant
        FOREIGN KEY (tenant_id, professional_user_id) REFERENCES users (tenant_id, id),

    CONSTRAINT settlement_items_member_not_blank CHECK (char_length(btrim(member_number)) > 0),
    CONSTRAINT settlement_items_modality_known CHECK (modality IN ('online', 'presencial')),
    CONSTRAINT settlement_items_currency_iso CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT settlement_items_amount_positive CHECK (amount_minor > 0)
);

-- A session is settled by one active settlement at a time.
CREATE UNIQUE INDEX settlement_items_one_active_per_appointment
    ON settlement_items (tenant_id, appointment_id)
    WHERE released_at IS NULL;

CREATE INDEX settlement_items_settlement_idx
    ON settlement_items (tenant_id, settlement_id);

-- ─────────────────────────────────────────────────────────────────────────────
-- Immutability: triggers, because a CHECK cannot compare the old row with the new one.
--
-- settlements: issued -> voided with its void fields. Everything else is refused: a voided
-- settlement is final, an issued one is frozen except for its void, and nothing is deleted.
-- ─────────────────────────────────────────────────────────────────────────────

-- +goose StatementBegin
CREATE FUNCTION settlements_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'settlements are never deleted; void them instead'
            USING ERRCODE = 'restrict_violation';
    END IF;

    IF OLD.status = 'voided' THEN
        RAISE EXCEPTION 'a voided settlement is final'
            USING ERRCODE = 'restrict_violation';
    END IF;

    IF ROW(NEW.id, NEW.tenant_id, NEW.insurer_id, NEW.insurer_name, NEW.insurer_tax_id,
           NEW.practice_name, NEW.period_start, NEW.period_end,
           NEW.created_by, NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.id, OLD.tenant_id, OLD.insurer_id, OLD.insurer_name, OLD.insurer_tax_id,
           OLD.practice_name, OLD.period_start, OLD.period_end,
           OLD.created_by, OLD.created_at) THEN
        RAISE EXCEPTION 'an issued settlement is immutable except for its void'
            USING ERRCODE = 'restrict_violation';
    END IF;

    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER settlements_guard
    BEFORE UPDATE OR DELETE ON settlements
    FOR EACH ROW EXECUTE FUNCTION settlements_guard();

-- settlement_items: frozen while the settlement stands. The only change allowed is the
-- release, and only once the settlement is voided. A released item is final.
-- +goose StatementBegin
CREATE FUNCTION settlement_items_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'settled sessions are never deleted; void the settlement instead'
            USING ERRCODE = 'restrict_violation';
    END IF;

    IF OLD.released_at IS NOT NULL THEN
        RAISE EXCEPTION 'a released session is final'
            USING ERRCODE = 'restrict_violation';
    END IF;

    IF NEW.released_at IS NULL THEN
        RAISE EXCEPTION 'a settled session is immutable except for its release'
            USING ERRCODE = 'restrict_violation';
    END IF;

    IF ROW(NEW.tenant_id, NEW.settlement_id, NEW.appointment_id, NEW.patient_id, NEW.patient_name,
           NEW.member_number, NEW.professional_user_id, NEW.professional_name, NEW.modality,
           NEW.starts_at, NEW.currency, NEW.amount_minor)
       IS DISTINCT FROM
       ROW(OLD.tenant_id, OLD.settlement_id, OLD.appointment_id, OLD.patient_id, OLD.patient_name,
           OLD.member_number, OLD.professional_user_id, OLD.professional_name, OLD.modality,
           OLD.starts_at, OLD.currency, OLD.amount_minor) THEN
        RAISE EXCEPTION 'a settled session is immutable except for its release'
            USING ERRCODE = 'restrict_violation';
    END IF;

    IF NOT EXISTS (
        SELECT 1
          FROM settlements s
         WHERE s.tenant_id = NEW.tenant_id
           AND s.id = NEW.settlement_id
           AND s.status = 'voided'
    ) THEN
        RAISE EXCEPTION 'a session is released only when its settlement is voided'
            USING ERRCODE = 'restrict_violation';
    END IF;

    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER settlement_items_guard
    BEFORE UPDATE OR DELETE ON settlement_items
    FOR EACH ROW EXECUTE FUNCTION settlement_items_guard();

-- ─────────────────────────────────────────────────────────────────────────────
-- Row-level security, same shape as 0002, 0004, 0007, 0009 and 0014
-- ─────────────────────────────────────────────────────────────────────────────

ALTER TABLE settlements ENABLE ROW LEVEL SECURITY;
ALTER TABLE settlements FORCE ROW LEVEL SECURITY;
CREATE POLICY settlements_tenant_isolation ON settlements
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

ALTER TABLE settlement_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE settlement_items FORCE ROW LEVEL SECURITY;
CREATE POLICY settlement_items_tenant_isolation ON settlement_items
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- +goose Down

DROP TRIGGER IF EXISTS settlement_items_guard ON settlement_items;
DROP FUNCTION IF EXISTS settlement_items_guard();
DROP TRIGGER IF EXISTS settlements_guard ON settlements;
DROP FUNCTION IF EXISTS settlements_guard();
DROP TABLE IF EXISTS settlement_items;
DROP TABLE IF EXISTS settlements;
