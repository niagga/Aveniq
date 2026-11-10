#Requires -Version 5.1
<#
.SYNOPSIS
  Reports the state of the local stack.
#>
[CmdletBinding()]
param()

$ErrorActionPreference = 'Continue'

$RepoRoot = Split-Path -Parent $PSScriptRoot
$LocalDir = Join-Path $RepoRoot '.local'
$PgBin    = Join-Path $LocalDir 'pgsql\bin'
$PgData   = Join-Path $LocalDir 'pgdata'
$RedisDir = Join-Path $LocalDir 'redis'
$RunDir   = Join-Path $LocalDir 'run'

$PgPort    = 5433
$RedisPort = 6380
$DbUser    = 'aveniq'
$DbName    = 'aveniq'

Write-Host "`nLocal stack status" -ForegroundColor Cyan

# PostgreSQL
if ((Test-Path (Join-Path $PgBin 'pg_ctl.exe')) -and (Test-Path (Join-Path $PgData 'PG_VERSION'))) {
    & (Join-Path $PgBin 'pg_ctl.exe') -D $PgData status *> $null
    if ($LASTEXITCODE -eq 0) {
        & (Join-Path $PgBin 'pg_isready.exe') -h 127.0.0.1 -p $PgPort -d $DbName -U $DbUser *> $null
        if ($LASTEXITCODE -eq 0) { Write-Host "  postgres   running (ready)   127.0.0.1:$PgPort" -ForegroundColor Green }
        else { Write-Host "  postgres   running (not ready) 127.0.0.1:$PgPort" -ForegroundColor Yellow }
    }
    else {
        Write-Host "  postgres   stopped           127.0.0.1:$PgPort" -ForegroundColor Yellow
    }
}
else {
    Write-Host '  postgres   not installed' -ForegroundColor Red
}

# Redis
$redisPidFile = Join-Path $RunDir 'redis.pid'
$redisRunning = $false
if (Test-Path $redisPidFile) {
    $redisPid = Get-Content $redisPidFile -ErrorAction SilentlyContinue
    $proc = if ($redisPid) { Get-Process -Id $redisPid -ErrorAction SilentlyContinue } else { $null }
    $redisRunning = [bool]($proc -and $proc.ProcessName -like 'redis*')
}

if ($redisRunning -and (Test-Path (Join-Path $RedisDir 'redis-cli.exe'))) {
    $ping = & (Join-Path $RedisDir 'redis-cli.exe') -h 127.0.0.1 -p $RedisPort ping 2>$null
    if ($ping -eq 'PONG') { Write-Host "  redis      running (ready)   127.0.0.1:$RedisPort" -ForegroundColor Green }
    else { Write-Host "  redis      running (not ready) 127.0.0.1:$RedisPort" -ForegroundColor Yellow }
}
elseif ($redisRunning) {
    Write-Host "  redis      running           127.0.0.1:$RedisPort" -ForegroundColor Yellow
}
else {
    Write-Host "  redis      stopped           127.0.0.1:$RedisPort" -ForegroundColor Yellow
}

# Ports held by something else
foreach ($port in @($PgPort, $RedisPort)) {
    $holder = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue |
        Select-Object -First 1
    if ($holder) {
        $owner = (Get-Process -Id $holder.OwningProcess -ErrorAction SilentlyContinue).ProcessName
        Write-Host "  port $port  held by $owner (pid $($holder.OwningProcess))" -ForegroundColor DarkGray
    }
}

Write-Host ''
