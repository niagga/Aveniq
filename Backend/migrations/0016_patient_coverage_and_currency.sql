-- Aveniq — patient coverage and billing currency (E6/T50).
--
-- A patient is either private (particular) or covered by an insurer (mutual). A mutual
-- has its own fee schedule and settles with the practice monthly (L61), so the insurer
-- is a row of its own, scoped to the practice, and the patient points at it.
--
-- The billing currency is chosen by hand at registration (L61): nothing derives it from
-- the country. The three currencies are fixed for the MVP.
--
-- The column arrives with a default of ARS so that the application as it stands keeps
-- writing patients. The default is removed together with the API contract change (T51);
-- until then the database has the value, and from then on the application must send it.

-- +goose Up

-- ─────────────────────────────────────────────────────────────────────────────
-- The insurers (mutuales) of one practice.
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE insurers (
    id          uuid        NOT NULL PRIMARY KEY,
    tenant_id   uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,

    name        text        NOT NULL,
    -- The insurer receives the monthly settlement, so the tax identity is what a
    -- settlement is addressed to. Optional until the first settlement is issued (T55).
    tax_id      text        NULL,

    -- Insurers are deactivated, never deleted: patients and settlements reference them.
    is_active   boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,

    CONSTRAINT insurers_name_not_blank CHECK (char_length(btrim(name)) > 0),
    CONSTRAINT insurers_tax_id_shape CHECK (tax_id IS NULL OR tax_id ~ '^[0-9]{11}$'),
    CONSTRAINT insurers_updated_after_creation CHECK (updated_at >= created_at)
);

-- The same mutual cannot be entered twice in one practice, whatever the casing.
CREATE UNIQUE INDEX insurers_tenant_name_unique
    ON insurers (tenant_id, lower(btrim(name)));

-- Composite key so a patient can only point at an insurer of its own practice (same
-- pattern as patients and appointments).
CREATE UNIQUE INDEX insurers_tenant_id_id_unique ON insurers (tenant_id, id);

-- ─────────────────────────────────────────────────────────────────────────────
-- The patient's coverage and billing currency.
-- ─────────────────────────────────────────────────────────────────────────────

ALTER TABLE patients
    ADD COLUMN coverage_kind   text    NOT NULL DEFAULT 'private',
    ADD COLUMN insurer_id      uuid    NULL,
    ADD COLUMN member_number   text    NULL,
    ADD COLUMN billing_currency char(3) NOT NULL DEFAULT 'ARS';

ALTER TABLE patients
    ADD CONSTRAINT patients_coverage_known
        CHECK (coverage_kind IN ('private', 'insurer')),
    -- A private patient has no insurer and no member number. An insured one has both: a
    -- mutual without an affiliate number cannot be billed.
    ADD CONSTRAINT patients_coverage_complete
        CHECK (
            (coverage_kind = 'private'
                AND insurer_id IS NULL AND member_number IS NULL)
         OR (coverage_kind = 'insurer'
                AND insurer_id IS NOT NULL
                AND member_number IS NOT NULL AND char_length(btrim(member_number)) > 0)
        ),
    ADD CONSTRAINT patients_insurer_same_tenant
        FOREIGN KEY (tenant_id, insurer_id) REFERENCES insurers (tenant_id, id),
    ADD CONSTRAINT patients_billing_currency_known
        CHECK (billing_currency IN ('ARS', 'EUR', 'USD'));

-- Settlements group the sessions of one insurer (T55): this is the lookup they use.
CREATE INDEX patients_tenant_insurer_idx
    ON patients (tenant_id, insurer_id)
    WHERE insurer_id IS NOT NULL;

-- ─────────────────────────────────────────────────────────────────────────────
-- Row-level security, same shape as 0002, 0004, 0007, 0009 and 0014
-- ─────────────────────────────────────────────────────────────────────────────

ALTER TABLE insurers ENABLE ROW LEVEL SECURITY;
ALTER TABLE insurers FORCE ROW LEVEL SECURITY;
CREATE POLICY insurers_tenant_isolation ON insurers
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- +goose Down

ALTER TABLE patients
    DROP CONSTRAINT IF EXISTS patients_billing_currency_known,
    DROP CONSTRAINT IF EXISTS patients_insurer_same_tenant,
    DROP CONSTRAINT IF EXISTS patients_coverage_complete,
    DROP CONSTRAINT IF EXISTS patients_coverage_known;

DROP INDEX IF EXISTS patients_tenant_insurer_idx;

ALTER TABLE patients
    DROP COLUMN IF EXISTS billing_currency,
    DROP COLUMN IF EXISTS member_number,
    DROP COLUMN IF EXISTS insurer_id,
    DROP COLUMN IF EXISTS coverage_kind;

DROP TABLE IF EXISTS insurers;
