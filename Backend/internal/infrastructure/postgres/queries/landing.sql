-- The public landing of a practice. The tenant comes from app.tenant_id, never from a WHERE
-- clause: a statement without tenant context sees zero rows and writes none.

-- name: GetTenantProfile :one
SELECT *
FROM tenant_profiles;

-- name: UpsertTenantProfile :one
-- The first save creates the row; later saves replace the text. tenant_id is the primary key,
-- so a practice has one landing, and the WITH CHECK of the policy refuses a row for another
-- practice.
INSERT INTO tenant_profiles (tenant_id, display_name, tagline, about, address, phone, email, schedule, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (tenant_id) DO UPDATE
SET display_name = EXCLUDED.display_name,
    tagline      = EXCLUDED.tagline,
    about        = EXCLUDED.about,
    address      = EXCLUDED.address,
    phone        = EXCLUDED.phone,
    email        = EXCLUDED.email,
    schedule     = EXCLUDED.schedule,
    updated_at   = EXCLUDED.updated_at
RETURNING *;

-- name: GetTenantIDByHost :one
-- The host map is not tenant-scoped: the public landing resolves the practice from the host
-- before any tenant context exists.
SELECT tenant_id
FROM tenant_domains
WHERE host = $1;
