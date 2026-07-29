param(
    [string]$EnvFile = ".env.local",
    [string]$OutputDirectory = "backups"
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot "env.ps1")
Enable-ProjectToolchain
Import-DotEnv -Path (Join-Path $projectRoot $EnvFile)

$outputPath = Join-Path $projectRoot $OutputDirectory
New-Item -ItemType Directory -Force -Path $outputPath | Out-Null
$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$backupPath = Join-Path $outputPath "goshorty-$timestamp.dump"
$containerPath = "/tmp/goshorty-$timestamp.dump"
$compose = @("compose", "--env-file", (Join-Path $projectRoot $EnvFile),
    "-f", (Join-Path $projectRoot "compose.yaml"))

& docker @compose exec -T postgres pg_dump `
    --username $env:POSTGRES_USER `
    --dbname $env:POSTGRES_DB `
    --format custom `
    --no-owner `
    --no-privileges `
    --file $containerPath
if ($LASTEXITCODE -ne 0) {
    throw "pg_dump failed"
}

$containerID = (& docker @compose ps -q postgres).Trim()
if (-not $containerID) {
    throw "PostgreSQL container is not running"
}
try {
    & docker cp "${containerID}:${containerPath}" $backupPath
    if ($LASTEXITCODE -ne 0) {
        throw "docker cp failed"
    }
}
finally {
    & docker @compose exec -T postgres rm -f $containerPath | Out-Null
}

$hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $backupPath).Hash.ToLowerInvariant()
Write-Output "Backup created: $backupPath"
Write-Output "SHA256: $hash"
