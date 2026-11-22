-- Aveniq — public landing of a practice (E8/T64, S24).
--
-- A practice has a public face: the landing that opens at its own address, with the text its
-- owner writes. tenant_profiles holds that text. It is tenant data, so it is protected by
-- row-level security like every business table: a row is visible only in its own practice.
--
-- tenant_domains maps a host to its practice for the hosts that are not the slug subdomain
-- (sur.<base> is resolved from the slug; a practice's own domain is loaded by hand, S24). It is
-- deliberately NOT tenant-scoped: the host is what identifies the practice, so it has to be
-- readable before any tenant context exists, the same way tenants is. It holds no business data.

-- +goose Up

CREATE TABLE tenant_profiles (
    tenant_id    uuid        PRIMARY KEY REFERENCES tenants (id) ON DELETE CASCADE,
    display_name text        NOT NULL,
    tagline      text        NOT NULL DEFAULT '',
    about        text        NOT NULL DEFAULT '',
    address      text        NOT NULL DEFAULT '',
    phone        text        NOT NULL DEFAULT '',
    email        text        NOT NULL DEFAULT '',
    schedule     text        NOT NULL DEFAULT '',
    updated_at   timestamptz NOT NULL,

    CONSTRAINT tenant_profiles_name_length CHECK (char_length(btrim(display_name)) BETWEEN 1 AND 120),
    CONSTRAINT tenant_profiles_tagline_length CHECK (char_length(tagline) <= 160),
    CONSTRAINT tenant_profiles_about_length CHECK (char_length(about) <= 2000),
    CONSTRAINT tenant_profiles_address_length CHECK (char_length(address) <= 200),
    CONSTRAINT tenant_profiles_phone_length CHECK (char_length(phone) <= 40),
    CONSTRAINT tenant_profiles_email_shape CHECK (
        char_length(email) <= 254 AND (email = '' OR email LIKE '%_@_%')
    ),
    CONSTRAINT tenant_profiles_schedule_length CHECK (char_length(schedule) <= 300)
);

COMMENT ON TABLE tenant_profiles IS 'Public landing text of a practice. Tenant-scoped through row-level security.';

-- Same shape as 0002, 0004, 0007, 0009, 0014, 0016 and 0019.
ALTER TABLE tenant_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_profiles FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_profiles_tenant_isolation ON tenant_profiles
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE TABLE tenant_domains (
    -- Lowercase, no port, no trailing dot. The primary key is what makes a host belong to one
    -- practice only.
    host       text        PRIMARY KEY,
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,

    CONSTRAINT tenant_domains_host_canonical CHECK (
        char_length(host) <= 253
        AND host = lower(host)
        AND host ~ '^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$'
    )
);

CREATE INDEX tenant_domains_tenant_id_idx ON tenant_domains (tenant_id);

COMMENT ON TABLE tenant_domains IS 'Host to practice map for custom domains. Not tenant-scoped on purpose: the host identifies the practice.';

-- +goose Down

DROP TABLE IF EXISTS tenant_domains;
DROP TABLE IF EXISTS tenant_profiles;
