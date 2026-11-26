-- Aveniq — the fiscal mirror leaves the patients row (E8/T85, S26).
--
-- Since T72 the fiscal data (tax id and its kind, fiscal condition, fiscal address)
-- and the billing currency belong to the billing client (`clients`). The `patients`
-- row kept a copy written once, when the patient was created (T72), and updated by
-- nothing: editing the client never propagated it back. T81 moved every billing and
-- reporting read to the client row, so the copy was written and never read. This
-- migration drops it, with the owner's explicit authorization to remove that data.
--
-- The data itself is not movable: there is nothing to copy back anywhere, which is
-- exactly why the columns can go. The Down recreates the shape, never the values.
--
-- Postgres drops these dependents together with the columns; they are listed here
-- so a reader does not have to guess what goes:
--   - patients_tax_id_complete            (0005) CHECK on tax_id / tax_id_kind
--   - patients_fiscal_condition_country   (0005) CHECK fiscal_condition needs a country
--   - patients_tax_id_kind_fk             (0005) FK to tax_id_kinds (country, tax_id_kind)
--   - patients_fiscal_condition_fk        (0005) FK to fiscal_conditions (country, fiscal_condition)
--   - patients_billing_currency_known     (0016) CHECK billing_currency IN (ARS, EUR, USD)
-- The default of billing_currency was already dropped by 0017, and
-- patients_fiscal_condition_known (0004) by 0005, so none remains to drop.

-- +goose Up

ALTER TABLE patients
    DROP COLUMN tax_id,
    DROP COLUMN tax_id_kind,
    DROP COLUMN fiscal_condition,
    DROP COLUMN fiscal_address,
    DROP COLUMN billing_currency;

-- +goose Down

-- The shape returns as it was across 0004, 0005, 0016 and 0017: the fiscal
-- columns nullable, billing_currency NOT NULL DEFAULT 'ARS' — the default 0016
-- introduced and 0017 dropped; going further Down, 0017's own Down removes it
-- again, so each migration only ever undoes its own shape change. The
-- constraints that can be restored without the catalog rows come back too.
-- The data cannot be restored: the drop removed a copy that nothing read —
-- a patient's fiscal data since T72 lives only on its billing client — so a
-- rolled-back patient row has empty fiscal columns and the client keeps the
-- only truth.
ALTER TABLE patients
    ADD COLUMN tax_id           text    NULL,
    ADD COLUMN tax_id_kind      text    NULL,
    ADD COLUMN fiscal_condition text    NULL,
    ADD COLUMN fiscal_address   text    NULL,
    ADD COLUMN billing_currency char(3) NOT NULL DEFAULT 'ARS';

ALTER TABLE patients
    ADD CONSTRAINT patients_tax_id_complete CHECK (
        (tax_id IS NULL AND tax_id_kind IS NULL)
        OR (tax_id IS NOT NULL
            AND tax_id_kind IS NOT NULL
            AND country IS NOT NULL
            AND char_length(btrim(tax_id)) > 0)
    ),
    ADD CONSTRAINT patients_fiscal_condition_country CHECK (
        fiscal_condition IS NULL OR country IS NOT NULL
    ),
    ADD CONSTRAINT patients_tax_id_kind_fk
        FOREIGN KEY (country, tax_id_kind) REFERENCES tax_id_kinds (country_code, code),
    ADD CONSTRAINT patients_fiscal_condition_fk
        FOREIGN KEY (country, fiscal_condition) REFERENCES fiscal_conditions (country_code, code),
    ADD CONSTRAINT patients_billing_currency_known
        CHECK (billing_currency IN ('ARS', 'EUR', 'USD'));
