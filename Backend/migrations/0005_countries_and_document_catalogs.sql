-- Aveniq — countries, document kinds and fiscal catalogs.
--
-- 0004 assumed Argentina: the document types were a CHECK of AR values, the CUIT
-- was eleven digits and the fiscal condition was the AFIP list. A practice in
-- Spain needs DNI and NIE, Portugal has the Cartão de Cidadão and NIF, Brazil has
-- CPF and CNPJ. None of that survives a hardcoded CHECK.
--
-- The rule this migration follows: a country is DATA, not a branch. Adding a
-- country means inserting rows, never writing `if country == ...` in the domain.
-- The catalog carries the shape and names the checksum strategy; Go implements the
-- strategies by name, so the seam stays one map instead of a switch per layer.
--
-- What a catalog CANNOT do, written down so nobody tries it twice: a CHECK
-- constraint cannot reference another table, so the per-country pattern is
-- enforced by the domain (reading `pattern` from here) and not by Postgres. The
-- database keeps what it can keep declaratively: the kind exists, and it belongs
-- to the country the row claims.

-- +goose Up

CREATE TABLE countries (
    code char(2) NOT NULL PRIMARY KEY,
    name text    NOT NULL,

    CONSTRAINT countries_code_upper CHECK (code = upper(code))
);

INSERT INTO countries (code, name) VALUES
    ('AR', 'Argentina'),
    ('BR', 'Brasil'),
    ('ES', 'España'),
    ('PT', 'Portugal');

-- Personal identity documents.
CREATE TABLE document_kinds (
    country_code char(2) NOT NULL REFERENCES countries (code),
    code         text    NOT NULL,
    label        text    NOT NULL,
    -- Canonical shape once the separators are gone. Not a full validation: the
    -- check digit is the domain's job, through `checksum`.
    pattern      text    NOT NULL,
    normalizer   text    NOT NULL,
    -- Name of the check-digit strategy implemented in Go. NULL means the document
    -- has no checksum worth computing (a passport, for instance).
    checksum     text    NULL,

    PRIMARY KEY (country_code, code),

    CONSTRAINT document_kinds_code_canonical CHECK (code = lower(btrim(code)) AND code <> ''),
    CONSTRAINT document_kinds_normalizer_known CHECK (normalizer IN ('digits', 'alnum_upper', 'as_is'))
);

INSERT INTO document_kinds (country_code, code, label, pattern, normalizer, checksum) VALUES
    ('AR', 'dni',      'DNI',                '^[0-9]{7,8}$',                 'digits',      NULL),
    ('AR', 'lc',       'Libreta cívica',     '^[0-9]{7,8}$',                 'digits',      NULL),
    ('AR', 'le',       'Libreta de enrolamiento', '^[0-9]{7,8}$',            'digits',      NULL),
    ('AR', 'passport', 'Pasaporte',          '^[A-Z0-9]{5,12}$',             'alnum_upper', NULL),
    ('BR', 'cpf',      'CPF',                '^[0-9]{11}$',                  'digits',      'br_cpf'),
    ('BR', 'rg',       'RG',                 '^[A-Z0-9]{5,12}$',             'alnum_upper', NULL),
    ('BR', 'passport', 'Passaporte',         '^[A-Z0-9]{5,12}$',             'alnum_upper', NULL),
    ('ES', 'dni',      'DNI',                '^[0-9]{8}[A-Z]$',              'alnum_upper', 'es_dni'),
    ('ES', 'nie',      'NIE',                '^[XYZ][0-9]{7}[A-Z]$',         'alnum_upper', 'es_nie'),
    ('ES', 'passport', 'Pasaporte',          '^[A-Z0-9]{5,12}$',             'alnum_upper', NULL),
    ('PT', 'cc',       'Cartão de Cidadão',  '^[0-9]{9}[A-Z]{2}[0-9]$',      'alnum_upper', 'pt_cc'),
    ('PT', 'passport', 'Passaporte',         '^[A-Z0-9]{5,12}$',             'alnum_upper', NULL);

-- Fiscal identifiers. A separate table from the personal documents on purpose:
-- with a single table plus an `is_tax` flag, a foreign key could never say "this
-- code must be a tax identifier", and that check would have to live in a trigger.
CREATE TABLE tax_id_kinds (
    country_code char(2) NOT NULL REFERENCES countries (code),
    code         text    NOT NULL,
    label        text    NOT NULL,
    pattern      text    NOT NULL,
    normalizer   text    NOT NULL,
    checksum     text    NULL,

    PRIMARY KEY (country_code, code),

    CONSTRAINT tax_id_kinds_code_canonical CHECK (code = lower(btrim(code)) AND code <> ''),
    CONSTRAINT tax_id_kinds_normalizer_known CHECK (normalizer IN ('digits', 'alnum_upper', 'as_is'))
);

INSERT INTO tax_id_kinds (country_code, code, label, pattern, normalizer, checksum) VALUES
    ('AR', 'cuit', 'CUIT', '^[0-9]{11}$',                   'digits',      'ar_cuit'),
    ('BR', 'cpf',  'CPF',  '^[0-9]{11}$',                   'digits',      'br_cpf'),
    ('BR', 'cnpj', 'CNPJ', '^[0-9]{14}$',                   'digits',      'br_cnpj'),
    ('ES', 'nif',  'NIF',  '^[0-9A-Z][0-9]{7}[0-9A-Z]$',    'alnum_upper', 'es_nif'),
    ('PT', 'nif',  'NIF',  '^[0-9]{9}$',                    'digits',      'pt_nif');

-- Fiscal condition is country-specific: "monotributo" means nothing in Portugal.
CREATE TABLE fiscal_conditions (
    country_code char(2) NOT NULL REFERENCES countries (code),
    code         text    NOT NULL,
    label        text    NOT NULL,

    PRIMARY KEY (country_code, code),

    CONSTRAINT fiscal_conditions_code_canonical CHECK (code = lower(btrim(code)) AND code <> '')
);

INSERT INTO fiscal_conditions (country_code, code, label) VALUES
    ('AR', 'consumidor_final',      'Consumidor final'),
    ('AR', 'monotributo',           'Monotributo'),
    ('AR', 'responsable_inscripto', 'Responsable inscripto'),
    ('AR', 'exento',                'Exento'),
    ('BR', 'mei',                   'MEI'),
    ('BR', 'simples_nacional',      'Simples Nacional'),
    ('BR', 'lucro_presumido',       'Lucro presumido'),
    ('ES', 'particular',            'Particular'),
    ('ES', 'autonomo',              'Autónomo'),
    ('ES', 'sociedad',              'Sociedad'),
    ('PT', 'particular',            'Particular'),
    ('PT', 'empresario_individual', 'Empresário individual'),
    ('PT', 'sociedade',             'Sociedade');

-- The countries and their catalogs are code, not tenant data: shipping them
-- versioned with the schema is what keeps them reviewable in one place.
REVOKE INSERT, UPDATE, DELETE ON countries, document_kinds, tax_id_kinds, fiscal_conditions
    FROM aveniq_app;

-- ─────────────────────────────────────────────────────────────────────────────
-- Tenants pick a country; patients inherit it.
--
-- Inheritance lives in the domain: the adapter always writes the effective
-- country on the patient, so no row can carry a document without a country to
-- validate it against.
-- ─────────────────────────────────────────────────────────────────────────────

ALTER TABLE tenants
    ADD COLUMN country char(2) NOT NULL DEFAULT 'AR' REFERENCES countries (code);

-- ─────────────────────────────────────────────────────────────────────────────
-- Patients: from Argentine fields to country-agnostic ones
-- ─────────────────────────────────────────────────────────────────────────────

ALTER TABLE patients
    DROP CONSTRAINT patients_document_type_known,
    DROP CONSTRAINT patients_document_complete,
    DROP CONSTRAINT patients_fiscal_condition_known,
    -- The CUIT column and its eleven-digit CHECK: it becomes tax_id + tax_id_kind.
    DROP COLUMN cuit,
    ADD COLUMN country             char(2) NULL REFERENCES countries (code),
    ADD COLUMN document_normalized text    NULL,
    ADD COLUMN tax_id              text    NULL,
    ADD COLUMN tax_id_kind         text    NULL;

-- A document is present with its number and its country, or absent altogether.
-- The country is required only when the row carries something that needs
-- validating: a patient with a fiscal condition and no document is a real case
-- (that is how most practices start), so the first branch must allow it.
ALTER TABLE patients
    ADD CONSTRAINT patients_document_complete CHECK (
        (document_type IS NULL AND document_number IS NULL)
        OR (document_type IS NOT NULL
            AND document_number IS NOT NULL
            AND country IS NOT NULL
            AND char_length(btrim(document_number)) > 0)
    ),
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
    -- "The kind exists AND belongs to this country". With a CHECK of AR codes the
    -- database could only say the first half.
    ADD CONSTRAINT patients_document_kind_fk
        FOREIGN KEY (country, document_type) REFERENCES document_kinds (country_code, code),
    ADD CONSTRAINT patients_tax_id_kind_fk
        FOREIGN KEY (country, tax_id_kind) REFERENCES tax_id_kinds (country_code, code),
    ADD CONSTRAINT patients_fiscal_condition_fk
        FOREIGN KEY (country, fiscal_condition) REFERENCES fiscal_conditions (country_code, code);

-- The uniqueness rule now compares the normalized form, so the same document typed
-- with separators and without them is the same person.
DROP INDEX patients_tenant_document_unique;
CREATE UNIQUE INDEX patients_tenant_document_unique
    ON patients (tenant_id, document_type, document_normalized)
    WHERE document_normalized IS NOT NULL;

-- +goose Down

DROP INDEX patients_tenant_document_unique;
CREATE UNIQUE INDEX patients_tenant_document_unique
    ON patients (tenant_id, document_type, document_number)
    WHERE document_number IS NOT NULL;

ALTER TABLE patients
    DROP CONSTRAINT patients_fiscal_condition_country,
    DROP CONSTRAINT patients_fiscal_condition_fk,
    DROP CONSTRAINT patients_tax_id_kind_fk,
    DROP CONSTRAINT patients_document_kind_fk,
    DROP CONSTRAINT patients_tax_id_complete,
    DROP CONSTRAINT patients_document_complete,
    DROP COLUMN tax_id_kind,
    DROP COLUMN tax_id,
    DROP COLUMN document_normalized,
    DROP COLUMN country,
    ADD COLUMN cuit text NULL,
    ADD CONSTRAINT patients_document_complete CHECK (
        (document_type IS NULL AND document_number IS NULL)
        OR (document_type IS NOT NULL
            AND document_number IS NOT NULL
            AND char_length(btrim(document_number)) > 0)
    ),
    ADD CONSTRAINT patients_document_type_known CHECK (
        document_type IS NULL OR document_type IN ('dni', 'lc', 'le', 'passport', 'cuit', 'other')
    ),
    ADD CONSTRAINT patients_cuit_shape CHECK (cuit IS NULL OR cuit ~ '^[0-9]{11}$'),
    ADD CONSTRAINT patients_fiscal_condition_known CHECK (
        fiscal_condition IS NULL OR fiscal_condition IN (
            'consumidor_final', 'monotributo', 'responsable_inscripto', 'exento'
        )
    );

ALTER TABLE tenants DROP COLUMN country;

DROP TABLE fiscal_conditions;
DROP TABLE tax_id_kinds;
DROP TABLE document_kinds;
DROP TABLE countries;
