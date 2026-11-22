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
# When the variable is not in the environment, it is read from the repository's .env (git-ignored).
# The value is never printed.
if ([string]::IsNullOrEmpty($env:AVENIQ_SEED_PASSWORD)) {
    $dotEnv = Join-Path (Split-Path -Parent $PSScriptRoot) '.env'
    if (Test-Path $dotEnv) {
        $entry = Get-Content $dotEnv | Where-Object { $_ -match '^\s*AVENIQ_SEED_PASSWORD=' } | Select-Object -First 1
        if ($entry) { $env:AVENIQ_SEED_PASSWORD = $entry.Substring($entry.IndexOf('=') + 1).Trim() }
    }
}
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
# on every run. Accounts that already exist are updated to the new hash (and reactivated), so the
# password in .env is always the one that signs in. This is local-only; no migration does it.
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

# The secret of the standing invitation below (L58) is derived from the development password,
# like the password itself: it is not written into the repository, and the E2E derives the same
# 32 bytes (base64url, 43 characters) from E2E_PASSWORD. Never printed.
$InviteBytes = [Security.Cryptography.SHA256]::Create().ComputeHash([Text.Encoding]::UTF8.GetBytes("aveniq-e2e-expired-invitation:$Password"))
$InviteSecret = [Convert]::ToBase64String($InviteBytes).TrimEnd("=").Replace("+", "-").Replace("/", "_")

Write-Host "`nDevelopment data" -ForegroundColor Green

$sql = @"
-- The demo practice is development data: its trial must not end while the E2E journeys
-- write (L58). GREATEST keeps the later end, so re-running the seed also moves a practice
-- created earlier with the default 14 days.
INSERT INTO tenants (id, slug, name, status, country, created_at, updated_at, trial_ends_at)
VALUES (gen_random_uuid(), '$Slug', '$Name', 'trial', 'AR', now(), now(), now() + interval '10 years')
ON CONFLICT (slug) DO UPDATE
SET trial_ends_at = GREATEST(tenants.trial_ends_at, EXCLUDED.trial_ends_at), updated_at = now();

-- A practice whose trial has already ended (T48): it is read-only, and the E2E journey checks
-- that. Its end is set to the past on every run, so the test does not depend on the day it runs.
-- The creation date stays in the past too, because the schema requires trial_ends_at > created_at.
INSERT INTO tenants (id, slug, name, status, country, created_at, updated_at, trial_ends_at)
VALUES (gen_random_uuid(), 'demo-vencido', 'Consultorio Vencido', 'trial', 'AR', now() - interval '30 days', now(), now() - interval '1 day')
ON CONFLICT (slug) DO UPDATE
SET trial_ends_at = EXCLUDED.trial_ends_at, status = 'trial', updated_at = now();

INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, is_active, created_at, updated_at)
SELECT gen_random_uuid(), t.id, 'vencido@aveniq.local', '$PasswordHash', 'Demo Vencido', 'owner', true, now(), now()
FROM tenants t
WHERE t.slug = 'demo-vencido'
ON CONFLICT ON CONSTRAINT users_tenant_email_unique DO UPDATE
SET password_hash = EXCLUDED.password_hash, full_name = EXCLUDED.full_name, role_code = EXCLUDED.role_code, is_active = true, updated_at = now();

-- The accountant of the expired practice (L59): holds no tenant:read, so it only learns that the
-- trial ended from the practice status route, and the E2E checks that its screens say so.
INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, is_active, created_at, updated_at)
SELECT gen_random_uuid(), t.id, 'contador-vencido@aveniq.local', '$PasswordHash', 'Demo Contador Vencido', 'accountant', true, now(), now()
FROM tenants t
WHERE t.slug = 'demo-vencido'
ON CONFLICT ON CONSTRAINT users_tenant_email_unique DO UPDATE
SET password_hash = EXCLUDED.password_hash, full_name = EXCLUDED.full_name, role_code = EXCLUDED.role_code, is_active = true, updated_at = now();

-- A standing invitation to the expired practice (L58): the E2E opens it and checks that the link
-- does not create an account there. An expired practice cannot issue one (its writes are refused),
-- so the seed does. The token is inv1.<practice id>.<secret> and the table keeps its SHA-256 only.
INSERT INTO invitations (id, tenant_id, email, role_code, token_hash, invited_by, expires_at, accepted_at, created_at)
SELECT gen_random_uuid(), t.id, 'invitada@aveniq.local', 'assistant',
       encode(sha256(convert_to('inv1.' || t.id::text || '.$InviteSecret', 'UTF8')), 'hex'),
       NULL, now() + interval '10 years', NULL, now()
FROM tenants t
WHERE t.slug = 'demo-vencido'
ON CONFLICT ON CONSTRAINT invitations_token_hash_unique DO UPDATE
SET expires_at = EXCLUDED.expires_at, accepted_at = NULL;

INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, is_active, created_at, updated_at)
SELECT gen_random_uuid(), t.id, '$Email', '$PasswordHash', 'Demo Owner', 'owner', true, now(), now()
FROM tenants t
WHERE t.slug = '$Slug'
ON CONFLICT ON CONSTRAINT users_tenant_email_unique DO UPDATE
SET password_hash = EXCLUDED.password_hash, full_name = EXCLUDED.full_name, role_code = EXCLUDED.role_code, is_active = true, updated_at = now();

-- Professionals and front desk for the T38 journeys: a treating professional, a second
-- professional who must not see the first one's patients, and an assistant who must
-- not see the clinical record. Same development credential as the owner.
INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, is_active, created_at, updated_at)
SELECT gen_random_uuid(), t.id, 'profesional@aveniq.local', '$PasswordHash', 'Demo Profesional', 'professional', true, now(), now()
FROM tenants t
WHERE t.slug = '$Slug'
ON CONFLICT ON CONSTRAINT users_tenant_email_unique DO UPDATE
SET password_hash = EXCLUDED.password_hash, full_name = EXCLUDED.full_name, role_code = EXCLUDED.role_code, is_active = true, updated_at = now();

INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, is_active, created_at, updated_at)
SELECT gen_random_uuid(), t.id, 'profesional2@aveniq.local', '$PasswordHash', 'Demo Profesional 2', 'professional', true, now(), now()
FROM tenants t
WHERE t.slug = '$Slug'
ON CONFLICT ON CONSTRAINT users_tenant_email_unique DO UPDATE
SET password_hash = EXCLUDED.password_hash, full_name = EXCLUDED.full_name, role_code = EXCLUDED.role_code, is_active = true, updated_at = now();

INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, is_active, created_at, updated_at)
SELECT gen_random_uuid(), t.id, 'recepcion@aveniq.local', '$PasswordHash', 'Demo Recepcion', 'assistant', true, now(), now()
FROM tenants t
WHERE t.slug = '$Slug'
ON CONFLICT ON CONSTRAINT users_tenant_email_unique DO UPDATE
SET password_hash = EXCLUDED.password_hash, full_name = EXCLUDED.full_name, role_code = EXCLUDED.role_code, is_active = true, updated_at = now();

-- The accountant of E4 (S22): bills and voids vouchers, reads nothing clinical.
INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, is_active, created_at, updated_at)
SELECT gen_random_uuid(), t.id, 'contador@aveniq.local', '$PasswordHash', 'Demo Contador', 'accountant', true, now(), now()
FROM tenants t
WHERE t.slug = '$Slug'
ON CONFLICT ON CONSTRAINT users_tenant_email_unique DO UPDATE
SET password_hash = EXCLUDED.password_hash, full_name = EXCLUDED.full_name, role_code = EXCLUDED.role_code, is_active = true, updated_at = now();

-- The administrator of E4 (L46): reads billing, issues and voids nothing.
INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, is_active, created_at, updated_at)
SELECT gen_random_uuid(), t.id, 'admin@aveniq.local', '$PasswordHash', 'Demo Administracion', 'admin', true, now(), now()
FROM tenants t
WHERE t.slug = '$Slug'
ON CONFLICT ON CONSTRAINT users_tenant_email_unique DO UPDATE
SET password_hash = EXCLUDED.password_hash, full_name = EXCLUDED.full_name, role_code = EXCLUDED.role_code, is_active = true, updated_at = now();
"@

$output = & $Psql "postgres://${DbUser}:${DbPass}@127.0.0.1:${PgPort}/${DbName}?sslmode=disable" -v ON_ERROR_STOP=1 -q -c $sql 2>&1
if ($LASTEXITCODE -ne 0) {
    throw "Seeding failed. Is the local stack up? Run scripts/local-up.ps1.`n$output"
}

Write-Ok "practice '$Slug' and owner '$Email' are in place"
Write-Ok "practice 'demo-vencido' (trial ended, read-only) and owner 'vencido@aveniq.local' and accountant 'contador-vencido@aveniq.local' are in place"
Write-Host "`nSign in with:" -ForegroundColor Green
Write-Host "  consultorio  $Slug"
Write-Host "  email        $Email"
Write-Host "  contraseña   la de AVENIQ_SEED_PASSWORD (no se imprime)"
Write-Host "`n  Development credentials. They live in this stack only and are not shipped by any migration." -ForegroundColor Yellow

