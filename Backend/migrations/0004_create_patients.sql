-- Aveniq — patients.
--
-- The practice's client record: who the person is, how to reach them, and the
-- fiscal data billing needs. The clinical record is a separate table (0005) on
-- purpose: personal and fiscal data have one audience, clinical data has another,
-- and mixing them would make the permission matrix unable to tell them apart.

-- +goose Up

CREATE TABLE patients (
    id          uuid        NOT NULL PRIMARY KEY,
    tenant_id   uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,

    -- Identity. full_name is the only required field: a practice must be able to
    -- register someone who walks in without a document, and refusing that would
    -- push the real data into a notes field where nothing can be validated.
    full_name   text        NOT NULL,
    birth_date  date        NULL,

    -- The document is optional as a pair: a type without a number, or the reverse,
    -- is a half-filled row that cannot be matched against anything.
    document_type   text    NULL,
    document_number text    NULL,

    -- Contact.
    phone                   text NULL,
    email                   text NULL,
    emergency_contact_name  text NULL,
    emergency_contact_phone text NULL,

    -- Fiscal data, present from the start so billing does not have to migrate this
    -- table later (S16). All optional: not every patient is invoiced.
    fiscal_condition text NULL,
    cuit             text NULL,
    legal_name       text NULL,
    fiscal_address   text NULL,

    -- Free-text administrative notes. Deliberately not clinical: the clinical
    -- record is 0005, and administrative notes must never become the place where
    -- clinical information is stored by accident.
    administrative_notes text NULL,

    -- Patients are deactivated, never deleted. Clinical notes will reference this
    -- row (0005), and a delete would either cascade a person's history away or
    -- fail at the foreign key. An inactive patient stays readable and stops
    -- appearing in new appointments.
    is_active   boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,

    CONSTRAINT patients_full_name_not_blank
        CHECK (char_length(btrim(full_name)) > 0),
    CONSTRAINT patients_document_complete
        CHECK (
            (document_type IS NULL AND document_number IS NULL)
            OR (document_type IS NOT NULL
                AND document_number IS NOT NULL
                AND char_length(btrim(document_number)) > 0)
        ),
    CONSTRAINT patients_document_type_known
        CHECK (document_type IS NULL OR document_type IN ('dni', 'lc', 'le', 'passport', 'cuit', 'other')),
    -- The conditions AFIP actually accepts. A free-text field here would produce
    -- invoices that cannot be issued, discovered at the worst moment.
    CONSTRAINT patients_fiscal_condition_known
        CHECK (fiscal_condition IS NULL OR fiscal_condition IN (
            'consumidor_final', 'monotributo', 'responsable_inscripto', 'exento'
        )),
    -- A CUIT is eleven digits; anything else is a typo that would fail at the
    -- fiscal platform instead of at the form.
    CONSTRAINT patients_cuit_shape
        CHECK (cuit IS NULL OR cuit ~ '^[0-9]{11}$'),
    CONSTRAINT patients_updated_after_creation
        CHECK (updated_at >= created_at)
);

-- The same document may exist in two practices (a person can be a patient of two
-- professionals) but only once in each.
CREATE UNIQUE INDEX patients_tenant_document_unique
    ON patients (tenant_id, document_type, document_number)
    WHERE document_number IS NOT NULL;

-- Listing a practice's active patients is the most common read; ordering by name
-- is how the frontend shows them. Substring search will need a trigram index, and
-- that is a decision for when the search box exists.
CREATE INDEX patients_tenant_active_name_idx
    ON patients (tenant_id, full_name)
    WHERE is_active;

-- ─────────────────────────────────────────────────────────────────────────────
-- Row-level security
--
-- Same shape as 0002, and the same reasoning: FORCE is what makes the policy apply
-- to the table owner, and the NULLIF form is what makes a session without
-- app.tenant_id see zero rows instead of everything. Both clauses are needed:
-- USING filters reads, WITH CHECK refuses writes into another tenant.
-- ─────────────────────────────────────────────────────────────────────────────

ALTER TABLE patients ENABLE ROW LEVEL SECURITY;
ALTER TABLE patients FORCE ROW LEVEL SECURITY;

CREATE POLICY patients_tenant_isolation ON patients
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- +goose Down

DROP TABLE patients;
