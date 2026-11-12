-- Aveniq — clinical record.
--
-- The most sensitive data this product holds, and the first place where "the tenant
-- filter is applied by the adapter" stops being enough. Everything below is built
-- around one idea: a clinical row cannot point at a patient of another practice, and
-- that refusal must be a property of the schema, not of a code path somebody can
-- forget to call.
--
-- Two tables, because the stable frame and the running story are different things:
--   clinical_records  1:1 with a patient: why they came, what is known, what is aimed at
--   clinical_notes    one per encounter: what happened that day

-- +goose Up

-- ─────────────────────────────────────────────────────────────────────────────
-- Composite keys, so a foreign key can carry the tenant
--
-- A plain `REFERENCES patients (id)` cannot say "and of this tenant": a foreign key
-- check does not consult row-level security, so knowing a patient UUID from another
-- practice would be enough to hang a clinical note off it. That is exactly the hole
-- L10 recorded for sessions and T14 had to close in the adapter with an explicit
-- visibility query.
--
-- For clinical data the guard belongs in the schema: a composite foreign key
-- (tenant_id, patient_id) -> patients (tenant_id, id) requires a matching unique key
-- on the parent, and that is what these two indexes provide.
-- ─────────────────────────────────────────────────────────────────────────────

CREATE UNIQUE INDEX patients_tenant_id_id_unique ON patients (tenant_id, id);
CREATE UNIQUE INDEX users_tenant_id_id_unique ON users (tenant_id, id);

CREATE TABLE clinical_records (
    id         uuid        NOT NULL PRIMARY KEY,
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    patient_id uuid        NOT NULL,

    -- The frame of the treatment. Free text on purpose: a structured taxonomy here
    -- would be a clinical theory baked into a schema, and practices differ.
    chief_complaint text NULL,
    background      text NULL,
    objectives      text NULL,

    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,

    -- One record per patient: it is the frame, not a log.
    CONSTRAINT clinical_records_patient_unique UNIQUE (tenant_id, patient_id),
    CONSTRAINT clinical_records_patient_same_tenant
        FOREIGN KEY (tenant_id, patient_id) REFERENCES patients (tenant_id, id)
        ON DELETE CASCADE,
    CONSTRAINT clinical_records_updated_after_creation CHECK (updated_at >= created_at)
);

CREATE TABLE clinical_notes (
    id         uuid        NOT NULL PRIMARY KEY,
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    patient_id uuid        NOT NULL,

    -- Who wrote it. Taken from the access token by the use case, never from the body:
    -- a note signed by whoever the client claims is not a record, it is a rumour.
    author_user_id uuid NOT NULL,

    -- When the encounter happened, which is not when the row was written: a
    -- professional writes on Sunday about Thursday.
    encountered_at timestamptz NOT NULL,
    content        text        NOT NULL,

    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,

    CONSTRAINT clinical_notes_patient_same_tenant
        FOREIGN KEY (tenant_id, patient_id) REFERENCES patients (tenant_id, id)
        ON DELETE CASCADE,
    CONSTRAINT clinical_notes_author_same_tenant
        FOREIGN KEY (tenant_id, author_user_id) REFERENCES users (tenant_id, id),
    CONSTRAINT clinical_notes_content_not_blank CHECK (char_length(btrim(content)) > 0),
    CONSTRAINT clinical_notes_updated_after_creation CHECK (updated_at >= created_at)
);

-- Reading one patient's history is the only query this table has.
CREATE INDEX clinical_notes_patient_encountered_idx
    ON clinical_notes (tenant_id, patient_id, encountered_at DESC);

COMMENT ON TABLE clinical_notes IS
    'One row per encounter. Notes are never deleted: a correction is a new note, because deleting clinical history is a legal problem, not a UI decision.';
COMMENT ON COLUMN clinical_notes.encountered_at IS
    'When the session happened. Not constrained to the past: now() is not immutable and a CHECK cannot express it, so the domain validates it.';

-- ─────────────────────────────────────────────────────────────────────────────
-- Row-level security, same shape as 0002 and 0004
--
-- FORCE is what makes the policy apply to the table owner; the NULLIF form is what
-- makes a session without app.tenant_id see zero rows instead of everything. USING
-- filters reads, WITH CHECK refuses writes into another tenant — both are needed.
-- ─────────────────────────────────────────────────────────────────────────────

ALTER TABLE clinical_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE clinical_records FORCE ROW LEVEL SECURITY;
CREATE POLICY clinical_records_tenant_isolation ON clinical_records
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

ALTER TABLE clinical_notes ENABLE ROW LEVEL SECURITY;
ALTER TABLE clinical_notes FORCE ROW LEVEL SECURITY;
CREATE POLICY clinical_notes_tenant_isolation ON clinical_notes
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- +goose Down

DROP TABLE clinical_notes;
DROP TABLE clinical_records;

DROP INDEX users_tenant_id_id_unique;
DROP INDEX patients_tenant_id_id_unique;
