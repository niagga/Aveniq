-- Aveniq — agenda.
--
-- Appointments are instants in UTC. Recurring series are expanded in the practice's
-- timezone, because "every Tuesday at 18:00" is a civil time, not a UTC time, and
-- DST would otherwise slide the slot twice a year (L14).
--
-- Occurrences are rows, not a formula evaluated at read time. Editing a series
-- rewrites future scheduled appointments and leaves attended ones alone: rewriting
-- a session that already happened is a different kind of lie.

-- +goose Up

CREATE EXTENSION IF NOT EXISTS btree_gist;

ALTER TABLE tenants
    ADD COLUMN timezone text NOT NULL DEFAULT 'America/Argentina/Buenos_Aires';

ALTER TABLE tenants
    ADD CONSTRAINT tenants_timezone_not_blank CHECK (char_length(btrim(timezone)) > 0);

COMMENT ON COLUMN tenants.timezone IS
    'IANA name used to expand recurring series. Instants on appointments stay in UTC.';

CREATE TABLE appointment_series (
    id                    uuid        NOT NULL PRIMARY KEY,
    tenant_id             uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    professional_user_id  uuid        NOT NULL,
    patient_id            uuid        NOT NULL,

    -- Civil recurrence: weekday (Go/Postgres Sunday=0) and minutes from local midnight.
    weekday               smallint    NOT NULL,
    local_start_minute    integer     NOT NULL,
    duration_minutes      integer     NOT NULL,
    starts_on             date        NOT NULL,
    until_on              date        NOT NULL,

    created_at            timestamptz NOT NULL,
    updated_at            timestamptz NOT NULL,

    CONSTRAINT appointment_series_professional_same_tenant
        FOREIGN KEY (tenant_id, professional_user_id) REFERENCES users (tenant_id, id),
    CONSTRAINT appointment_series_patient_same_tenant
        FOREIGN KEY (tenant_id, patient_id) REFERENCES patients (tenant_id, id),
    CONSTRAINT appointment_series_weekday_known CHECK (weekday BETWEEN 0 AND 6),
    CONSTRAINT appointment_series_local_start_minute CHECK (local_start_minute BETWEEN 0 AND 1439),
    CONSTRAINT appointment_series_duration CHECK (duration_minutes BETWEEN 5 AND 480),
    CONSTRAINT appointment_series_range CHECK (until_on >= starts_on),
    CONSTRAINT appointment_series_updated_after_creation CHECK (updated_at >= created_at)
);

CREATE TABLE appointments (
    id                    uuid        NOT NULL PRIMARY KEY,
    tenant_id             uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    professional_user_id  uuid        NOT NULL,
    patient_id            uuid        NOT NULL,
    series_id             uuid        NULL,

    starts_at             timestamptz NOT NULL,
    ends_at               timestamptz NOT NULL,
    status                text        NOT NULL DEFAULT 'scheduled',
    notes                 text        NULL,

    created_at            timestamptz NOT NULL,
    updated_at            timestamptz NOT NULL,

    CONSTRAINT appointments_professional_same_tenant
        FOREIGN KEY (tenant_id, professional_user_id) REFERENCES users (tenant_id, id),
    CONSTRAINT appointments_patient_same_tenant
        FOREIGN KEY (tenant_id, patient_id) REFERENCES patients (tenant_id, id),
    CONSTRAINT appointments_series_same_tenant
        FOREIGN KEY (series_id) REFERENCES appointment_series (id) ON DELETE SET NULL,
    CONSTRAINT appointments_status_known CHECK (status IN ('scheduled', 'attended', 'cancelled')),
    CONSTRAINT appointments_interval CHECK (ends_at > starts_at),
    CONSTRAINT appointments_updated_after_creation CHECK (updated_at >= created_at),

    -- Two live appointments of the same professional cannot occupy the same instant.
    -- Cancelled ones drop out of the constraint so a slot can be reused. btree_gist
    -- is what lets a UUID equality sit next to a range overlap in one exclusion.
    CONSTRAINT appointments_no_overlap EXCLUDE USING gist (
        tenant_id WITH =,
        professional_user_id WITH =,
        tstzrange(starts_at, ends_at, '[)') WITH &&
    ) WHERE (status <> 'cancelled')
);

CREATE INDEX appointments_range_idx
    ON appointments (tenant_id, professional_user_id, starts_at);

CREATE INDEX appointments_patient_idx
    ON appointments (tenant_id, patient_id, starts_at);

CREATE INDEX appointment_series_professional_idx
    ON appointment_series (tenant_id, professional_user_id);

ALTER TABLE appointment_series ENABLE ROW LEVEL SECURITY;
ALTER TABLE appointment_series FORCE ROW LEVEL SECURITY;
CREATE POLICY appointment_series_tenant_isolation ON appointment_series
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

ALTER TABLE appointments ENABLE ROW LEVEL SECURITY;
ALTER TABLE appointments FORCE ROW LEVEL SECURITY;
CREATE POLICY appointments_tenant_isolation ON appointments
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- +goose Down

DROP TABLE IF EXISTS appointments;
DROP TABLE IF EXISTS appointment_series;

ALTER TABLE tenants DROP CONSTRAINT IF EXISTS tenants_timezone_not_blank;
ALTER TABLE tenants DROP COLUMN IF EXISTS timezone;

DROP EXTENSION IF EXISTS btree_gist;
