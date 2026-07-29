param(
    [Parameter(Mandatory = $true)]
    [string]$BackupFile,
    [string]$EnvFile = ".env.local",
    [switch]$ConfirmDatabaseReset
)

$ErrorActionPreference = "Stop"
if (-not $ConfirmDatabaseReset) {
    throw "Restore replaces the local database. Re-run with -ConfirmDatabaseReset."
}

$projectRoot = Split-Path -Parent $PSScriptRoot
$resolvedBackup = (Resolve-Path -LiteralPath $BackupFile).Path
. (Join-Path $PSScriptRoot "env.ps1")
Enable-ProjectToolchain
Import-DotEnv -Path (Join-Path $projectRoot $EnvFile)

$compose = @("compose", "--env-file", (Join-Path $projectRoot $EnvFile),
    "-f", (Join-Path $projectRoot "compose.yaml"))
$containerID = (& docker @compose ps -q postgres).Trim()
if (-not $containerID) {
    throw "PostgreSQL container is not running"
}

$containerPath = "/tmp/goshorty-restore.dump"
try {
    & docker cp $resolvedBackup "${containerID}:${containerPath}"
    if ($LASTEXITCODE -ne 0) {
        throw "docker cp failed"
    }
    & docker @compose exec -T postgres pg_restore `
        --username $env:POSTGRES_USER `
        --dbname $env:POSTGRES_DB `
        --clean `
        --if-exists `
        --no-owner `
        --no-privileges `
        $containerPath
    if ($LASTEXITCODE -ne 0) {
        throw "pg_restore failed"
    }
}
finally {
    & docker @compose exec -T postgres rm -f $containerPath | Out-Null
}

Write-Output "Local database restored from: $resolvedBackup"
