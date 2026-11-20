-- Aveniq — the billing currency is chosen by hand, never defaulted (E6/T51).
--
-- Migration 0016 gave patients.billing_currency a default of ARS so the application
-- could keep writing patients before the contract changed. The contract changed in
-- T51: the API requires billing_currency on every patient write, so the default is
-- removed. Existing patients keep the ARS they were given at backfill.

-- +goose Up

ALTER TABLE patients
    ALTER COLUMN billing_currency DROP DEFAULT;

-- +goose Down

ALTER TABLE patients
    ALTER COLUMN billing_currency SET DEFAULT 'ARS';
