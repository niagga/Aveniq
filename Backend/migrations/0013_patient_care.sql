-- Aveniq — E3/T38: who treats a patient, and who else may read the clinical record.
--
-- S18 and S23 need two facts the schema did not have: which professional is the
-- treating one (the tratante) of each patient, and which other professionals were
-- granted the clinical record of that patient. Both are instance-level, so both
-- belong to the rows, not to the permission matrix.
--
-- Decisions (AskQuestion, L40): one explicit treating professional per patient,
-- set by the professional who registers it or by an owner or admin; the grant is
-- given only by the treating professional and gives read access to the patient and
-- the clinical record, never the right to book.

-- +goose Up

-- ─────────────────────────────────────────────────────────────────────────────
-- The treating professional
--
-- Nullable on purpose: patients registered before T38 have no treating
-- professional until an owner or admin assigns one. The composite foreign key
-- keeps the professional in the same practice, the same way appointments do.
-- ─────────────────────────────────────────────────────────────────────────────

ALTER TABLE patients ADD COLUMN treating_professional_id uuid NULL;

ALTER TABLE patients ADD CONSTRAINT patients_treating_professional_same_tenant
    FOREIGN KEY (tenant_id, treating_professional_id) REFERENCES users (tenant_id, id);

-- ─────────────────────────────────────────────────────────────────────────────
-- Clinical grants
--
-- A grant is a row, not a flag, because the audit trail needs to show who was
-- allowed to read what and when. Revoking sets revoked_at and keeps the row: a
-- revoked grant is history, not a delete.
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE clinical_grants (
    id                   uuid        NOT NULL PRIMARY KEY,
    tenant_id            uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    patient_id           uuid        NOT NULL,
    professional_user_id uuid        NOT NULL,
    granted_by           uuid        NOT NULL,
    granted_at           timestamptz NOT NULL,
    revoked_at           timestamptz NULL,

    CONSTRAINT clinical_grants_patient_same_tenant
        FOREIGN KEY (tenant_id, patient_id) REFERENCES patients (tenant_id, id),
    CONSTRAINT clinical_grants_grantee_same_tenant
        FOREIGN KEY (tenant_id, professional_user_id) REFERENCES users (tenant_id, id),
    CONSTRAINT clinical_grants_granter_same_tenant
        FOREIGN KEY (tenant_id, granted_by) REFERENCES users (tenant_id, id),
    CONSTRAINT clinical_grants_revoked_after_granted
        CHECK (revoked_at IS NULL OR revoked_at >= granted_at)
);

-- One live grant per patient and professional. A revoked grant does not block a new
-- one, so a grant can be given again after it was withdrawn.
CREATE UNIQUE INDEX clinical_grants_one_live
    ON clinical_grants (tenant_id, patient_id, professional_user_id)
    WHERE revoked_at IS NULL;

ALTER TABLE clinical_grants ENABLE ROW LEVEL SECURITY;
ALTER TABLE clinical_grants FORCE ROW LEVEL SECURITY;
CREATE POLICY clinical_grants_tenant_isolation ON clinical_grants
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- ─────────────────────────────────────────────────────────────────────────────
-- Assigning the treating professional is a permission of its own: a professional
-- must not be able to hand a patient to someone else, and the other roles that
-- hold patients:write must not either.
-- ─────────────────────────────────────────────────────────────────────────────

INSERT INTO permissions (code, domain, description) VALUES
    ('patients:assign', 'patients', 'Set or change the treating professional of a patient.');

INSERT INTO role_permissions (role_code, permission_code) VALUES
    ('owner', 'patients:assign'),
    ('admin', 'patients:assign');

-- +goose Down

DELETE FROM role_permissions WHERE permission_code = 'patients:assign';
DELETE FROM permissions WHERE code = 'patients:assign';

DROP TABLE IF EXISTS clinical_grants;

ALTER TABLE patients DROP CONSTRAINT IF EXISTS patients_treating_professional_same_tenant;
ALTER TABLE patients DROP COLUMN IF EXISTS treating_professional_id;
