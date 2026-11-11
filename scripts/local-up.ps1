#Requires -Version 5.1
<#
.SYNOPSIS
  Starts the hermetic local stack: portable PostgreSQL 17 and Redis.

.DESCRIPTION
  Everything lives under .local/, so nothing is installed system-wide and nothing
  needs administrator rights. Running this twice is safe: each service is started
  only when it is not already running.

  Ports are deliberately non-default so the stack never collides with a system
  Postgres or Redis installed later.

.NOTES
  Postgres  PostgreSQL 17.6 (EDB binaries)  -> 127.0.0.1:5433
  Redis     Redis 5.0.14 (tporadowski)      -> 127.0.0.1:6380
#>
[CmdletBinding()]
param(
    [switch]$Force
)

$ErrorActionPreference = 'Stop'

$RepoRoot = Split-Path -Parent $PSScriptRoot
$LocalDir = Join-Path $RepoRoot '.local'
$PgBin    = Join-Path $LocalDir 'pgsql\bin'
$PgData   = Join-Path $LocalDir 'pgdata'
$RedisDir = Join-Path $LocalDir 'redis'
$LogDir   = Join-Path $LocalDir 'logs'
$RunDir   = Join-Path $LocalDir 'run'

$PgPort    = 5433
$RedisPort = 6380
$DbUser    = 'aveniq'
$DbPass    = 'aveniq_dev'
$DbName    = 'aveniq'

# The API connects with a separate role on purpose. PostgreSQL lets superusers and
# BYPASSRLS roles ignore every row-level security policy, so connecting as the
# owner would make tenant isolation decorative.
$AppUser = 'aveniq_app'
$AppPass = 'aveniq_app_dev'

function Write-Step($Message) { Write-Host "  $Message" -ForegroundColor Cyan }
function Write-Ok($Message) { Write-Host "  $Message" -ForegroundColor Green }
function Write-Warn($Message) { Write-Host "  $Message" -ForegroundColor Yellow }

if (-not (Test-Path (Join-Path $PgBin 'pg_ctl.exe'))) {
    throw "Postgres binaries not found at $PgBin. Run scripts/bootstrap.ps1 first."
}
if (-not (Test-Path (Join-Path $RedisDir 'redis-server.exe'))) {
    throw "Redis binaries not found at $RedisDir. Run scripts/bootstrap.ps1 first."
}

New-Item -ItemType Directory -Force -Path $LogDir, $RunDir | Out-Null

# ─────────────────────────────────────────────────────────────────────────────
# Windows DLL-initialisation workaround
#
# PostgreSQL forks one backend process per connection. When the user profile path
# contains a non-ASCII character (here: C:\Users\Gastón) and TEMP/TMP point inside
# it, every backend dies with STATUS_DLL_INIT_FAILED (0xC0000142) while the
# postmaster itself starts normally. The symptom is deceptive: the log says
# "ready to accept connections" and every client gets "server closed the
# connection unexpectedly".
#
# Pointing the temporary directories at an ASCII path inside the repository fixes
# it without changing anything machine-wide.
$TmpDir = Join-Path $LocalDir 'tmp'
New-Item -ItemType Directory -Force -Path $TmpDir | Out-Null
$env:TEMP = $TmpDir
$env:TMP = $TmpDir
$env:PGTMPDIR = $TmpDir

# ─────────────────────────────────────────────────────────────────────────────
# PostgreSQL
# ─────────────────────────────────────────────────────────────────────────────
Write-Host "`nPostgreSQL 17 (port $PgPort)"

if (-not (Test-Path (Join-Path $PgData 'PG_VERSION'))) {
    Write-Step 'initializing cluster (first run only)'
    $pwFile = Join-Path $LocalDir 'pw.tmp'
    try {
        # No trailing newline: initdb reads the file content verbatim as the password.
        [System.IO.File]::WriteAllText($pwFile, $DbPass)
        & (Join-Path $PgBin 'initdb.exe') `
            -D $PgData `
            -U $DbUser `
            "--pwfile=$pwFile" `
            --auth-local=scram-sha-256 `
            --auth-host=scram-sha-256 `
            --encoding=UTF8 `
            --locale=C 2>&1 | Out-File -FilePath (Join-Path $LogDir 'initdb.log') -Encoding utf8
        if ($LASTEXITCODE -ne 0) {
            throw "initdb failed. See $LogDir\initdb.log"
        }
    }
    finally {
        Remove-Item $pwFile -Force -ErrorAction SilentlyContinue
    }
}

& (Join-Path $PgBin 'pg_ctl.exe') -D $PgData status *> $null
if ($LASTEXITCODE -eq 0) {
    Write-Ok 'already running'
}
else {
    Write-Step 'starting'

    # Two traps here, both about handles that outlive the command:
    #  - `& pg_ctl ...` in a pipeline never returns, because postgres inherits the
    #    pipe and keeps it open for its whole life;
    #  - `Start-Process -Wait` (or any -RedirectStandard* on a daemon) blocks the
    #    same way, for the same reason: PowerShell waits on handles the server keeps.
    # So: start pg_ctl without -Wait and without redirection (its own failure
    # messages reach this console), and poll the readiness condition ourselves --
    # the same condition `pg_ctl -w` polls -- under a bounded deadline. The server
    # log is written by postgres itself through pg_ctl's -l.
    $pgCtlArgs = "-D `"$PgData`" -l `"$(Join-Path $LogDir 'postgres.log')`" " +
        "-o `"-p $PgPort -c listen_addresses=127.0.0.1`" -s start"

    $pgStart = Start-Process -FilePath (Join-Path $PgBin 'pg_ctl.exe') `
        -ArgumentList $pgCtlArgs `
        -NoNewWindow -PassThru

    $deadline = (Get-Date).AddSeconds(60)
    $started = $false
    while ((Get-Date) -lt $deadline) {
        Start-Sleep -Milliseconds 250
        & (Join-Path $PgBin 'pg_isready.exe') -h 127.0.0.1 -p $PgPort -q *> $null
        if ($LASTEXITCODE -eq 0) { $started = $true; break }
        # pg_ctl reports an immediate failure (bad arguments, unreadable data
        # directory) instead of leaving a server that will never become ready.
        if ($pgStart.HasExited -and $pgStart.ExitCode -ne 0) { break }
    }

    if (-not $started) {
        throw "Postgres did not become ready within 60s. See $LogDir\postgres.log"
    }
    Write-Ok 'started'
}

# Create the application database on first run.
$env:PGPASSWORD = $DbPass
$exists = & (Join-Path $PgBin 'psql.exe') -h 127.0.0.1 -p $PgPort -U $DbUser -d postgres `
    -tAc "SELECT 1 FROM pg_database WHERE datname = '$DbName'" 2>$null
if ($exists -ne '1') {
    Write-Step "creating database '$DbName'"
    & (Join-Path $PgBin 'createdb.exe') -h 127.0.0.1 -p $PgPort -U $DbUser $DbName
    if ($LASTEXITCODE -ne 0) { throw "Failed to create database '$DbName'" }
    Write-Ok "database '$DbName' created"
}
else {
    Write-Ok "database '$DbName' present"
}

# ─── Application role ───
# Row-level security is ignored by superusers, by roles holding BYPASSRLS and,
# unless FORCE is set, by the owner of the table. `aveniq` here has all three
# luxuries; `aveniq_app` has none of them, which is what makes isolation real.
Write-Step "ensuring the application role '$AppUser'"
$appRoleSql = @"
DO `$`$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '$AppUser') THEN
        CREATE ROLE $AppUser NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
    END IF;
    ALTER ROLE $AppUser LOGIN PASSWORD '$AppPass';
END
`$`$;
"@
& (Join-Path $PgBin 'psql.exe') -h 127.0.0.1 -p $PgPort -U $DbUser -d $DbName -v ON_ERROR_STOP=1 -q -c $appRoleSql
if ($LASTEXITCODE -ne 0) { throw "Failed to create the application role '$AppUser'" }
Write-Ok "role '$AppUser' ready (grants come from migration 0003)"

$env:PGPASSWORD = $null

# ─────────────────────────────────────────────────────────────────────────────
# Redis
# ─────────────────────────────────────────────────────────────────────────────
Write-Host "`nRedis (port $RedisPort)"

$redisPidFile = Join-Path $RunDir 'redis.pid'
$redisRunning = $false

if (Test-Path $redisPidFile) {
    $existingPid = Get-Content $redisPidFile -ErrorAction SilentlyContinue
    if ($existingPid) {
        $proc = Get-Process -Id $existingPid -ErrorAction SilentlyContinue
        if ($proc -and $proc.ProcessName -like 'redis*') { $redisRunning = $true }
    }
}

if ($redisRunning -and -not $Force) {
    Write-Ok 'already running'
}
else {
    if ($redisRunning) {
        Write-Step 'restarting'
        Stop-Process -Id (Get-Content $redisPidFile) -Force -ErrorAction SilentlyContinue
        Start-Sleep -Milliseconds 500
    }
    else {
        Write-Step 'starting'
    }

    # Redis writes its own log through --logfile: a PowerShell redirect would give
    # the daemon a handle PowerShell then waits on, and the script would never
    # return once the server is up.
    $redisLogFile = Join-Path $LogDir 'redis.log'
    $redisArgs = @(
        '--port', $RedisPort,
        '--bind', '127.0.0.1',
        '--dir', $RedisDir,
        '--dbfilename', 'dump.rdb',
        '--logfile', $redisLogFile,
        '--save', '""',
        '--appendonly', 'no'
    )
    $redisProc = Start-Process -FilePath (Join-Path $RedisDir 'redis-server.exe') `
        -ArgumentList $redisArgs `
        -WorkingDirectory $RedisDir `
        -WindowStyle Hidden `
        -PassThru
    Set-Content -Path $redisPidFile -Value $redisProc.Id
    Start-Sleep -Milliseconds 800

    if ($redisProc.HasExited) {
        throw "Redis failed to start. See $redisLogFile"
    }
    Write-Ok 'started'
}

# ─────────────────────────────────────────────────────────────────────────────
# Readiness
# ─────────────────────────────────────────────────────────────────────────────
Write-Host "`nReadiness"

& (Join-Path $PgBin 'pg_isready.exe') -h 127.0.0.1 -p $PgPort -d $DbName -U $DbUser *> $null
if ($LASTEXITCODE -ne 0) { throw 'Postgres is not accepting connections' }
Write-Ok 'postgres ready'

$ping = & (Join-Path $RedisDir 'redis-cli.exe') -h 127.0.0.1 -p $RedisPort ping 2>$null
if ($ping -ne 'PONG') { throw "Redis did not answer PING (got '$ping')" }
Write-Ok 'redis ready'

Write-Host "`nLocal stack is up." -ForegroundColor Green
Write-Host "  DATABASE_URL = postgres://${AppUser}:${AppPass}@127.0.0.1:${PgPort}/${DbName}?sslmode=disable  (application role, RLS enforced)"
Write-Host "  MIGRATOR     = postgres://${DbUser}:${DbPass}@127.0.0.1:${PgPort}/${DbName}?sslmode=disable  (owner role, migrations only)"
Write-Host "  REDIS_ADDR   = 127.0.0.1:${RedisPort}"
Write-Host "  Logs         = $LogDir"
