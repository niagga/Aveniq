#Requires -Version 5.1
<#
.SYNOPSIS
  Stops the portable PostgreSQL and Redis started by scripts/local-up.ps1.

.DESCRIPTION
  Stops services only; it never deletes data. To destroy the local cluster,
  delete .local/pgdata and .local/redis/dump.rdb manually.
#>
[CmdletBinding()]
param()

$ErrorActionPreference = 'Continue'

$RepoRoot = Split-Path -Parent $PSScriptRoot
$LocalDir = Join-Path $RepoRoot '.local'
$PgBin    = Join-Path $LocalDir 'pgsql\bin'
$PgData   = Join-Path $LocalDir 'pgdata'
$RunDir   = Join-Path $LocalDir 'run'

Write-Host "`nPostgreSQL"
if (Test-Path (Join-Path $PgBin 'pg_ctl.exe')) {
    & (Join-Path $PgBin 'pg_ctl.exe') -D $PgData status *> $null
    if ($LASTEXITCODE -eq 0) {
        # -m fast: refuse new connections, wait for in-flight transactions, then exit.
        & (Join-Path $PgBin 'pg_ctl.exe') -D $PgData -m fast -w -t 60 stop | Out-Null
        if ($LASTEXITCODE -eq 0) { Write-Host '  stopped' -ForegroundColor Green }
        else { Write-Host '  failed to stop' -ForegroundColor Red }
    }
    else {
        Write-Host '  not running' -ForegroundColor Yellow
    }
}
else {
    Write-Host '  binaries not installed' -ForegroundColor Yellow
}

Write-Host "`nRedis"
$redisPidFile = Join-Path $RunDir 'redis.pid'
if (Test-Path $redisPidFile) {
    $redisPid = Get-Content $redisPidFile -ErrorAction SilentlyContinue
    $proc = if ($redisPid) { Get-Process -Id $redisPid -ErrorAction SilentlyContinue } else { $null }
    if ($proc -and $proc.ProcessName -like 'redis*') {
        Stop-Process -Id $redisPid -Force
        Write-Host '  stopped' -ForegroundColor Green
    }
    else {
        Write-Host '  not running' -ForegroundColor Yellow
    }
    Remove-Item $redisPidFile -Force -ErrorAction SilentlyContinue
}
else {
    Write-Host '  no pid file' -ForegroundColor Yellow
}

Write-Host "`nLocal stack is down.`n" -ForegroundColor Green
