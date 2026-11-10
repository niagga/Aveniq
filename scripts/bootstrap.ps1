#Requires -Version 5.1
<#
.SYNOPSIS
  One-shot setup of a local development environment.

.DESCRIPTION
  Idempotent: every step checks before acting, so running it twice is safe.

    1. verify Go, Node.js and pnpm
    2. install the Go tools (sqlc, goose, air) when missing
    3. download and extract portable PostgreSQL and Redis into .local/
    4. start the stack (delegated to local-up.ps1)
    5. apply migrations
    6. install frontend dependencies

  Nothing is installed machine-wide and no administrator rights are required.

.PARAMETER SkipDownloads
  Reuse the archives already present in .local/downloads.
#>
[CmdletBinding()]
param(
    [switch]$SkipDownloads
)

$ErrorActionPreference = 'Stop'

$RepoRoot = Split-Path -Parent $PSScriptRoot
$LocalDir = Join-Path $RepoRoot '.local'
$DownDir  = Join-Path $LocalDir 'downloads'
$TmpDir   = Join-Path $LocalDir 'tmp'

$PgArchive    = 'postgres-17.6-binaries.zip'
$PgUrl        = 'https://get.enterprisedb.com/postgresql/postgresql-17.6-1-windows-x64-binaries.zip'
$RedisArchive = 'redis-5.0.14.1.zip'
$RedisUrl     = 'https://github.com/tporadowski/redis/releases/download/v5.0.14.1/Redis-x64-5.0.14.1.zip'

$DatabaseUrl = 'postgres://aveniq:aveniq_dev@127.0.0.1:5433/aveniq?sslmode=disable'

function Write-Step($Message) { Write-Host "→ $Message" -ForegroundColor Cyan }
function Write-Ok($Message) { Write-Host "  $Message" -ForegroundColor Green }
function Write-Fail($Message) { Write-Host "  $Message" -ForegroundColor Red }

New-Item -ItemType Directory -Force -Path $LocalDir, $DownDir, $TmpDir | Out-Null

# Same DLL-initialisation workaround as local-up.ps1: keep every spawned process away
# from the non-ASCII user profile path.
$env:TEMP = $TmpDir
$env:TMP = $TmpDir
$env:PGTMPDIR = $TmpDir

# ─────────────────────────────────────────────────────────────────────────────
# 1. Prerequisites
# ─────────────────────────────────────────────────────────────────────────────
Write-Host "`nPrerequisites"

$missing = @()

$goVersion = (Get-Command go -ErrorAction SilentlyContinue)
if ($goVersion) { Write-Ok "go      $((& go version) -replace 'go version ','')" }
else { Write-Fail 'go      not found — install from https://go.dev/dl/'; $missing += 'go' }

$nodeVersion = (Get-Command node -ErrorAction SilentlyContinue)
if ($nodeVersion) { Write-Ok "node    $(& node --version)" }
else { Write-Fail 'node    not found — install from https://nodejs.org/'; $missing += 'node' }

$pnpmVersion = (Get-Command pnpm -ErrorAction SilentlyContinue)
if ($pnpmVersion) { Write-Ok "pnpm    $(& pnpm --version)" }
else { Write-Fail 'pnpm    not found — run: corepack enable'; $missing += 'pnpm' }

if ($missing.Count -gt 0) {
    throw "Missing prerequisites: $($missing -join ', ')"
}

# ─────────────────────────────────────────────────────────────────────────────
# 2. Go tools
# ─────────────────────────────────────────────────────────────────────────────
Write-Host "`nGo tools"

$goBin = Join-Path (& go env GOPATH) 'bin'
$goTools = @(
    @{ Name = 'sqlc';  Package = 'github.com/sqlc-dev/sqlc/cmd/sqlc@latest' },
    @{ Name = 'goose'; Package = 'github.com/pressly/goose/v3/cmd/goose@latest' },
    @{ Name = 'air';   Package = 'github.com/air-verse/air@latest' }
)

foreach ($tool in $goTools) {
    $exe = Join-Path $goBin "$($tool.Name).exe"
    if (Test-Path $exe) {
        Write-Ok "$($tool.Name) already installed"
    }
    else {
        Write-Step "installing $($tool.Name) (this can take a few minutes)"
        & go install $tool.Package
        if ($LASTEXITCODE -ne 0) { throw "Failed to install $($tool.Name)" }
        Write-Ok "$($tool.Name) installed"
    }
}

# ─────────────────────────────────────────────────────────────────────────────
# 3. Portable PostgreSQL and Redis
# ─────────────────────────────────────────────────────────────────────────────
Write-Host "`nPortable services"

function Get-Archive($Name, $Url) {
    $target = Join-Path $DownDir $Name
    if ((Test-Path $target) -and $SkipDownloads) {
        Write-Ok "$Name present (cached)"
        return $target
    }
    if (Test-Path $target) {
        Write-Ok "$Name present"
        return $target
    }

    Write-Step "downloading $Name (about 330 MB for PostgreSQL)"
    Invoke-WebRequest -Uri $Url -OutFile $target -UseBasicParsing
    Write-Ok "$Name downloaded"
    return $target
}

if (-not (Test-Path (Join-Path $LocalDir 'pgsql\bin\pg_ctl.exe'))) {
    $archive = Get-Archive $PgArchive $PgUrl
    Write-Step 'extracting PostgreSQL'
    & "$env:SystemRoot\System32\tar.exe" -xf $archive -C $LocalDir
    if ($LASTEXITCODE -ne 0) { throw 'Failed to extract PostgreSQL' }
    Write-Ok 'PostgreSQL extracted'
}
else {
    Write-Ok 'PostgreSQL already extracted'
}

if (-not (Test-Path (Join-Path $LocalDir 'redis\redis-server.exe'))) {
    $archive = Get-Archive $RedisArchive $RedisUrl
    Write-Step 'extracting Redis'
    $redisDir = Join-Path $LocalDir 'redis'
    New-Item -ItemType Directory -Force -Path $redisDir | Out-Null
    & "$env:SystemRoot\System32\tar.exe" -xf $archive -C $redisDir
    if ($LASTEXITCODE -ne 0) { throw 'Failed to extract Redis' }
    Write-Ok 'Redis extracted'
}
else {
    Write-Ok 'Redis already extracted'
}

# ─────────────────────────────────────────────────────────────────────────────
# 4. Start the stack
# ─────────────────────────────────────────────────────────────────────────────
Write-Host "`nLocal stack"
& (Join-Path $PSScriptRoot 'local-up.ps1')
if ($LASTEXITCODE -ne 0) { throw 'local-up.ps1 failed' }

# ─────────────────────────────────────────────────────────────────────────────
# 5. Migrations
# ─────────────────────────────────────────────────────────────────────────────
Write-Host "`nMigrations"
Push-Location (Join-Path $RepoRoot 'Backend')
try {
    $goose = Join-Path $goBin 'goose.exe'
    & $goose -dir migrations postgres $DatabaseUrl up
    if ($LASTEXITCODE -ne 0) { throw 'Migrations failed' }
    Write-Ok 'schema up to date'
}
finally {
    Pop-Location
}

# ─────────────────────────────────────────────────────────────────────────────
# 6. Frontend dependencies
# ─────────────────────────────────────────────────────────────────────────────
Write-Host "`nFrontend"
Push-Location (Join-Path $RepoRoot 'Frontend')
try {
    if (Test-Path (Join-Path $RepoRoot 'Frontend\node_modules')) {
        Write-Ok 'node_modules present'
    }
    else {
        Write-Step 'pnpm install'
        & pnpm install
        if ($LASTEXITCODE -ne 0) { throw 'pnpm install failed' }
        Write-Ok 'dependencies installed'
    }
}
finally {
    Pop-Location
}

Write-Host "`nReady." -ForegroundColor Green
Write-Host '  Backend:  cd Backend; go run ./cmd/api'
Write-Host '  Frontend: cd Frontend; pnpm dev   ->  http://localhost:3000'
Write-Host "  API docs: http://127.0.0.1:8080/docs`n"
