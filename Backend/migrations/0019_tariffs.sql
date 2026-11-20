-- Aveniq — fee schedule (tarifario) of a practice (E6/T53, L61).
--
-- A tariff says what one session is worth for one professional, under one coverage (the
-- particular patient or one mutual of the catalog), in one modality and one billing
-- currency, from a date on. Prices are versions: a new price is a new row with a later
-- valid_from, and the rows before it stay, so a past session keeps the value that was in
-- force when it happened (T58 values the sessions with it).
--
-- Every row splits the session into two parts. insurer_minor is what the mutual pays and
-- patient_minor is what the patient pays (the copay). A particular session has no mutual:
-- insurer_minor is 0 and patient_minor is the whole fee, so one shape covers both.

-- +goose Up

CREATE TABLE tariffs (
    id                   uuid        NOT NULL PRIMARY KEY,
    tenant_id            uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    professional_user_id uuid        NOT NULL,

    coverage             text        NOT NULL,
    -- NULL for a particular patient, the mutual of the catalog otherwise.
    insurer_id           uuid        NULL,
    modality             text        NOT NULL,
    billing_currency     char(3)     NOT NULL,
    valid_from           date        NOT NULL,

    insurer_minor        bigint      NOT NULL,
    patient_minor        bigint      NOT NULL,

    created_at           timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL,

    CONSTRAINT tariffs_coverage_known CHECK (coverage IN ('private', 'insurer')),
    -- A particular price has no mutual and the mutual pays nothing. A mutual price names
    -- its mutual.
    CONSTRAINT tariffs_coverage_complete CHECK (
        (coverage = 'private' AND insurer_id IS NULL AND insurer_minor = 0)
     OR (coverage = 'insurer' AND insurer_id IS NOT NULL)
    ),
    CONSTRAINT tariffs_modality_known CHECK (modality IN ('online', 'presencial')),
    CONSTRAINT tariffs_billing_currency_known CHECK (billing_currency IN ('ARS', 'EUR', 'USD')),
    CONSTRAINT tariffs_parts_non_negative CHECK (insurer_minor >= 0 AND patient_minor >= 0),
    -- A session worth nothing is not a price: without one the proposal stays empty (L61).
    CONSTRAINT tariffs_total_positive CHECK (insurer_minor + patient_minor > 0),
    CONSTRAINT tariffs_updated_after_creation CHECK (updated_at >= created_at),

    -- Same-practice references, the composite pattern of patients, schedule and billing.
    CONSTRAINT tariffs_professional_same_tenant
        FOREIGN KEY (tenant_id, professional_user_id) REFERENCES users (tenant_id, id),
    CONSTRAINT tariffs_insurer_same_tenant
        FOREIGN KEY (tenant_id, insurer_id) REFERENCES insurers (tenant_id, id)
);

-- One price per combination and start date. NULLS NOT DISTINCT makes the particular rows
-- (insurer_id NULL) collide with each other, which is the rule: two particular prices for
-- the same modality, currency and day are one too many.
CREATE UNIQUE INDEX tariffs_validity_unique
    ON tariffs (tenant_id, professional_user_id, coverage, insurer_id, modality, billing_currency, valid_from)
    NULLS NOT DISTINCT;

-- ─────────────────────────────────────────────────────────────────────────────
-- Row-level security, same shape as 0002, 0004, 0007, 0009, 0014 and 0016
-- ─────────────────────────────────────────────────────────────────────────────

ALTER TABLE tariffs ENABLE ROW LEVEL SECURITY;
ALTER TABLE tariffs FORCE ROW LEVEL SECURITY;
CREATE POLICY tariffs_tenant_isolation ON tariffs
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- +goose Down

DROP TABLE IF EXISTS tariffs;
