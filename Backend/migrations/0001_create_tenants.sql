-- Aveniq — initial schema.
--
-- Only the tenant aggregate exists at this stage. Every business table added later
-- will carry tenant_id and be protected by row-level security; tenants itself is the
-- isolation root and is therefore not tenant-scoped.

-- +goose Up
CREATE TABLE tenants (
    id         uuid        PRIMARY KEY,
    slug       text        NOT NULL,
    name       text        NOT NULL,
    status     text        NOT NULL DEFAULT 'trial',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,

    CONSTRAINT tenants_slug_unique UNIQUE (slug),
    CONSTRAINT tenants_slug_canonical CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    CONSTRAINT tenants_slug_length CHECK (char_length(slug) BETWEEN 3 AND 63),
    CONSTRAINT tenants_name_not_blank CHECK (char_length(btrim(name)) > 0),
    CONSTRAINT tenants_status_known CHECK (status IN ('trial', 'active', 'suspended'))
);

COMMENT ON TABLE tenants IS 'Isolation root. Every business record belongs to exactly one tenant.';
COMMENT ON COLUMN tenants.slug IS 'DNS label used as the subdomain that resolves the tenant.';

-- Listing is always newest-first; the composite index keeps it index-only.
CREATE INDEX tenants_created_at_idx ON tenants (created_at DESC, id DESC);

-- +goose Down
DROP INDEX IF EXISTS tenants_created_at_idx;
DROP TABLE IF EXISTS tenants;
