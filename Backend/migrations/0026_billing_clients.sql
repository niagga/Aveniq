-- Aveniq — E8/T71: billing clients (who pays) separate from patients (who is treated).
--
-- Each patient points at exactly one client. The backfill gives every existing patient its
-- own client, copying the fiscal data and billing currency that still live on the patient
-- until T72 moves the API contract. Clients are deactivated, never deleted.

-- +goose Up

INSERT INTO permissions (code, domain, description) VALUES
    ('client:read',  'client', 'Read the billing clients of the practice.'),
    ('client:write', 'client', 'Create and edit billing clients.');

INSERT INTO role_permissions (role_code, permission_code)
SELECT 'owner', code FROM permissions
WHERE code NOT IN (SELECT permission_code FROM role_permissions WHERE role_code = 'owner');

INSERT INTO role_permissions (role_code, permission_code) VALUES
    ('admin',        'client:read'),
    ('admin',        'client:write'),
    ('professional', 'client:read'),
    ('professional', 'client:write'),
    ('assistant',    'client:read'),
    ('assistant',    'client:write'),
    ('accountant',   'client:read');

-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE clients (
    id               uuid        NOT NULL PRIMARY KEY,
    tenant_id        uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,

    display_name     text        NOT NULL,
    legal_name       text        NULL,

    country          char(2)     NOT NULL,

    fiscal_condition text        NULL,
    tax_id           text        NULL,
    tax_id_kind      text        NULL,
    fiscal_address   text        NULL,

    billing_currency char(3)     NOT NULL,

    is_active        boolean     NOT NULL DEFAULT true,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL,

    CONSTRAINT clients_display_name_not_blank
        CHECK (char_length(btrim(display_name)) > 0),
    CONSTRAINT clients_fiscal_condition_known
        CHECK (fiscal_condition IS NULL OR fiscal_condition IN (
            'consumidor_final', 'monotributo', 'responsable_inscripto', 'exento'
        )),
    CONSTRAINT clients_tax_id_shape
        CHECK (tax_id IS NULL OR char_length(btrim(tax_id)) > 0),
    CONSTRAINT clients_billing_currency_known
        CHECK (billing_currency IN ('ARS', 'EUR', 'USD')),
    CONSTRAINT clients_updated_after_creation
        CHECK (updated_at >= created_at)
);

CREATE UNIQUE INDEX clients_tenant_id_id_unique ON clients (tenant_id, id);

CREATE INDEX clients_tenant_active_name_idx
    ON clients (tenant_id, display_name)
    WHERE is_active;

ALTER TABLE clients ENABLE ROW LEVEL SECURITY;
ALTER TABLE clients FORCE ROW LEVEL SECURITY;
CREATE POLICY clients_tenant_isolation ON clients
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- ─────────────────────────────────────────────────────────────────────────────
-- Backfill: one client per patient, then require client_id on patients.
-- ─────────────────────────────────────────────────────────────────────────────

ALTER TABLE patients ADD COLUMN client_id uuid NULL;

WITH paired AS (
    SELECT
        p.id AS patient_id,
        gen_random_uuid() AS client_id,
        p.tenant_id,
        p.full_name,
        p.legal_name,
        p.country,
        p.fiscal_condition,
        p.tax_id,
        p.tax_id_kind,
        p.fiscal_address,
        p.billing_currency,
        p.is_active,
        p.created_at,
        p.updated_at
    FROM patients p
),
ins AS (
    INSERT INTO clients (
        id, tenant_id, display_name, legal_name, country,
        fiscal_condition, tax_id, tax_id_kind, fiscal_address,
        billing_currency, is_active, created_at, updated_at
    )
    SELECT
        client_id, tenant_id, full_name, legal_name, country,
        fiscal_condition, tax_id, tax_id_kind, fiscal_address,
        billing_currency, is_active, created_at, updated_at
    FROM paired
    RETURNING id
)
UPDATE patients p
SET client_id = paired.client_id
FROM paired
WHERE p.id = paired.patient_id;

ALTER TABLE patients ALTER COLUMN client_id SET NOT NULL;

ALTER TABLE patients
    ADD CONSTRAINT patients_client_same_tenant
        FOREIGN KEY (tenant_id, client_id) REFERENCES clients (tenant_id, id);

CREATE INDEX patients_tenant_client_idx ON patients (tenant_id, client_id);

-- +goose Down

DROP INDEX IF EXISTS patients_tenant_client_idx;

ALTER TABLE patients
    DROP CONSTRAINT IF EXISTS patients_client_same_tenant;

ALTER TABLE patients DROP COLUMN IF EXISTS client_id;

DROP TABLE IF EXISTS clients;

DELETE FROM role_permissions
WHERE permission_code IN ('client:read', 'client:write');

DELETE FROM permissions WHERE code IN ('client:read', 'client:write');