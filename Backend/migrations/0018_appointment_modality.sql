-- Aveniq — modality of an appointment and of a series (E6/T52, L61).
--
-- A session is online or presencial, chosen on the slot and on the series. Every
-- appointment that exists today happened in the practice, so the column arrives with
-- a default of presencial that backfills them. The default is then removed: from here
-- on the application must send the modality, as it does with the billing currency
-- (T51, migration 0017).

-- +goose Up

ALTER TABLE appointments
    ADD COLUMN modality text NOT NULL DEFAULT 'presencial';
ALTER TABLE appointments
    ADD CONSTRAINT appointments_modality_known CHECK (modality IN ('online', 'presencial'));
ALTER TABLE appointments
    ALTER COLUMN modality DROP DEFAULT;

ALTER TABLE appointment_series
    ADD COLUMN modality text NOT NULL DEFAULT 'presencial';
ALTER TABLE appointment_series
    ADD CONSTRAINT appointment_series_modality_known CHECK (modality IN ('online', 'presencial'));
ALTER TABLE appointment_series
    ALTER COLUMN modality DROP DEFAULT;

-- +goose Down

ALTER TABLE appointment_series
    DROP CONSTRAINT IF EXISTS appointment_series_modality_known,
    DROP COLUMN IF EXISTS modality;

ALTER TABLE appointments
    DROP CONSTRAINT IF EXISTS appointments_modality_known,
    DROP COLUMN IF EXISTS modality;
