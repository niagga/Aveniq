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

    # pg_ctl must not be piped: on Windows the postgres child inherits stdout and
    # keeps the pipe open forever, hanging the caller. Start-Process with file
    # redirection gives the child a real file handle and -Wait returns reliably.
    $pgCtlArgs = "-D `"$PgData`" -l `"$(Join-Path $LogDir 'postgres.log')`" " +
        "-o `"-p $PgPort -c listen_addresses=127.0.0.1`" -w -t 60 -s start"

    $pgStart = Start-Process -FilePath (Join-Path $PgBin 'pg_ctl.exe') `
        -ArgumentList $pgCtlArgs `
        -RedirectStandardOutput (Join-Path $LogDir 'pg_ctl.out.log') `
        -RedirectStandardError (Join-Path $LogDir 'pg_ctl.err.log') `
        -NoNewWindow -PassThru -Wait

    if ($pgStart.ExitCode -ne 0) {
        throw "Postgres failed to start (exit $($pgStart.ExitCode)). See $LogDir\postgres.log"
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

    $redisArgs = @(
        '--port', $RedisPort,
        '--bind', '127.0.0.1',
        '--dir', $RedisDir,
        '--dbfilename', 'dump.rdb',
        '--save', '""',
        '--appendonly', 'no'
    )
    $redisProc = Start-Process -FilePath (Join-Path $RedisDir 'redis-server.exe') `
        -ArgumentList $redisArgs `
        -WorkingDirectory $RedisDir `
        -RedirectStandardOutput (Join-Path $LogDir 'redis.log') `
        -RedirectStandardError (Join-Path $LogDir 'redis.err.log') `
        -WindowStyle Hidden `
        -PassThru
    Set-Content -Path $redisPidFile -Value $redisProc.Id
    Start-Sleep -Milliseconds 800

    if ($redisProc.HasExited) {
        throw "Redis failed to start. See $LogDir\redis.log and $LogDir\redis.err.log"
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
Write-Host "  DATABASE_URL = postgres://${DbUser}:${DbPass}@127.0.0.1:${PgPort}/${DbName}?sslmode=disable"
Write-Host "  REDIS_ADDR   = 127.0.0.1:${RedisPort}"
Write-Host "  Logs         = $LogDir"
