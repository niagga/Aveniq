# Aveniq — seed a practice and an owner for local development.
#
# Why this exists: after `bootstrap.ps1` the database has a schema and nothing else.
# There is no practice, so there is no user, so you cannot log in — the first thing
# anybody does is write SQL by hand, and the hand-written version drifts between
# machines.
#
# Not a migration, on purpose. A migration runs everywhere the schema runs, so a
# seeded practice with a known password would ship to production. This script talks
# to the local stack and refuses to run against anything else.
#
# Idempotent: run it as often as you like.
#
#   powershell -ExecutionPolicy Bypass -File scripts/local-seed.ps1

[CmdletBinding()]
param(
    # Keep the practice slug stable so re-runs find it.
    [string]$Slug = 'demo',
    [string]$Name = 'Consultorio Demo',
    [string]$Email = 'demo@aveniq.local'
)

$ErrorActionPreference = 'Stop'

# The development password is never written into the repository. It comes from the
# environment, and the seed hashes it on every run. The E2E reads the same value from
# E2E_PASSWORD, so set both to one value.
$Password = $env:AVENIQ_SEED_PASSWORD
if ([string]::IsNullOrEmpty($Password)) {
    throw 'Set AVENIQ_SEED_PASSWORD to the development password first (the E2E reads the same value from E2E_PASSWORD).'
}

$RepoRoot = Split-Path -Parent $PSScriptRoot
$LocalDir = Join-Path $RepoRoot '.local'
$PgBin = Join-Path $LocalDir 'pgsql\bin'
$Psql = Join-Path $PgBin 'psql.exe'

$PgPort = 5433
$DbName = 'aveniq'
$DbUser = 'aveniq'
$DbPass = 'aveniq_dev'

# TEMP must stay inside the repository: a non-ASCII user profile breaks the client
# DLL initialisation on this machine (see local-up.ps1).
$TmpDir = Join-Path $LocalDir 'tmp'
New-Item -ItemType Directory -Force -Path $TmpDir | Out-Null
$env:TEMP = $TmpDir
$env:TMP = $TmpDir

function Write-Step($Message) { Write-Host "  $Message" -ForegroundColor Cyan }
function Write-Ok($Message) { Write-Host "  $Message" -ForegroundColor Green }

if (-not (Test-Path $Psql)) {
    throw "psql not found at $Psql. Run scripts/bootstrap.ps1 first."
}

$env:PGPASSWORD = $DbPass
$env:PGCLIENTENCODING = 'UTF8'

# The argon2id hash of $Password, minted with the production parameters (m=64 MiB, t=3, p=4)
# so the seeded login exercises the real verification path. The hash is salted, so it differs
# on every run. Accounts that already exist keep the hash they were created with.
$PasswordHash = $null
Push-Location (Join-Path $RepoRoot 'Backend')
try {
    $PasswordHash = (go run ./cmd/passhash | Out-String).Trim()
} finally {
    Pop-Location
}
if ($LASTEXITCODE -ne 0 -or -not $PasswordHash.StartsWith('$argon2id$')) {
    throw 'Could not hash the seed password. Check that Go is installed and AVENIQ_SEED_PASSWORD is set.'
}

Write-Host "`nDevelopment data" -ForegroundColor Green

$sql = @"
INSERT INTO tenants (id, slug, name, status, country, created_at, updated_at)
VALUES (gen_random_uuid(), '$Slug', '$Name', 'trial', 'AR', now(), now())
ON CONFLICT (slug) DO NOTHING;

INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, is_active, created_at, updated_at)
SELECT gen_random_uuid(), t.id, '$Email', '$PasswordHash', 'Demo Owner', 'owner', true, now(), now()
FROM tenants t
WHERE t.slug = '$Slug'
ON CONFLICT ON CONSTRAINT users_tenant_email_unique DO NOTHING;

-- Professionals and front desk for the T38 journeys: a treating professional, a second
-- professional who must not see the first one's patients, and an assistant who must
-- not see the clinical record. Same development credential as the owner.
INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, is_active, created_at, updated_at)
SELECT gen_random_uuid(), t.id, 'profesional@aveniq.local', '$PasswordHash', 'Demo Profesional', 'professional', true, now(), now()
FROM tenants t
WHERE t.slug = '$Slug'
ON CONFLICT ON CONSTRAINT users_tenant_email_unique DO NOTHING;

INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, is_active, created_at, updated_at)
SELECT gen_random_uuid(), t.id, 'profesional2@aveniq.local', '$PasswordHash', 'Demo Profesional 2', 'professional', true, now(), now()
FROM tenants t
WHERE t.slug = '$Slug'
ON CONFLICT ON CONSTRAINT users_tenant_email_unique DO NOTHING;

INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, is_active, created_at, updated_at)
SELECT gen_random_uuid(), t.id, 'recepcion@aveniq.local', '$PasswordHash', 'Demo Recepcion', 'assistant', true, now(), now()
FROM tenants t
WHERE t.slug = '$Slug'
ON CONFLICT ON CONSTRAINT users_tenant_email_unique DO NOTHING;

-- The accountant of E4 (S22): bills and voids vouchers, reads nothing clinical.
INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, is_active, created_at, updated_at)
SELECT gen_random_uuid(), t.id, 'contador@aveniq.local', '$PasswordHash', 'Demo Contador', 'accountant', true, now(), now()
FROM tenants t
WHERE t.slug = '$Slug'
ON CONFLICT ON CONSTRAINT users_tenant_email_unique DO NOTHING;
"@

$output = & $Psql "postgres://${DbUser}:${DbPass}@127.0.0.1:${PgPort}/${DbName}?sslmode=disable" -v ON_ERROR_STOP=1 -q -c $sql 2>&1
if ($LASTEXITCODE -ne 0) {
    throw "Seeding failed. Is the local stack up? Run scripts/local-up.ps1.`n$output"
}

Write-Ok "practice '$Slug' and owner '$Email' are in place"
Write-Host "`nSign in with:" -ForegroundColor Green
Write-Host "  consultorio  $Slug"
Write-Host "  email        $Email"
Write-Host "  contraseña   la de AVENIQ_SEED_PASSWORD (no se imprime)"
Write-Host "`n  Development credentials. They live in this stack only and are not shipped by any migration." -ForegroundColor Yellow

