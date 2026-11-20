-- Aveniq — the copago of each settled session (E6/T55, L67).
--
-- The mutual pays its share of a session, and the patient pays the copago. The settlement's total is
-- the mutual's share only; the copago is kept on each item so the detail can show it, frozen at issue
-- like the rest of the item. It is not summed into any total.

-- +goose Up

ALTER TABLE settlement_items
    ADD COLUMN copago_minor bigint NOT NULL DEFAULT 0,
    ADD CONSTRAINT settlement_items_copago_not_negative CHECK (copago_minor >= 0);

-- The guard now freezes the copago with the rest of the item.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION settlement_items_guard() RETURNS trigger
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
           NEW.starts_at, NEW.currency, NEW.amount_minor, NEW.copago_minor)
       IS DISTINCT FROM
       ROW(OLD.tenant_id, OLD.settlement_id, OLD.appointment_id, OLD.patient_id, OLD.patient_name,
           OLD.member_number, OLD.professional_user_id, OLD.professional_name, OLD.modality,
           OLD.starts_at, OLD.currency, OLD.amount_minor, OLD.copago_minor) THEN
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

-- +goose Down

-- Restore the guard of 0021 before the column goes, so the function never names a missing column.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION settlement_items_guard() RETURNS trigger
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

ALTER TABLE settlement_items
    DROP CONSTRAINT IF EXISTS settlement_items_copago_not_negative,
    DROP COLUMN IF EXISTS copago_minor;
