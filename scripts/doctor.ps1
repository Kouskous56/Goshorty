param(
    [string]$EnvFile = ".env.local"
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot "env.ps1")
Enable-ProjectToolchain

$failures = [Collections.Generic.List[string]]::new()
function Report-Check {
    param([bool]$Success, [string]$Message)
    if ($Success) {
        Write-Host "[OK]   $Message" -ForegroundColor Green
    }
    else {
        Write-Host "[FAIL] $Message" -ForegroundColor Red
        $script:failures.Add($Message)
    }
}

$go = Get-Command go -ErrorAction SilentlyContinue
Report-Check ($null -ne $go) "Go is installed"
if ($go) {
    $previousToolchain = $env:GOTOOLCHAIN
    $env:GOTOOLCHAIN = "auto"
    $versionOutput = & $go.Path version
    $env:GOTOOLCHAIN = $previousToolchain
    $required = (Get-Content (Join-Path $projectRoot ".go-version") -Raw).Trim()
    Report-Check ($LASTEXITCODE -eq 0 -and
        $versionOutput -match "go$([regex]::Escape($required))\b") "Go $required is active"
}

$envPath = Join-Path $projectRoot $EnvFile
Report-Check (Test-Path -LiteralPath $envPath) "$EnvFile exists"
if (Test-Path -LiteralPath $envPath) {
    Import-DotEnv -Path $envPath
    $requiredVariables = @(
        "SECRET_KEY", "ADMIN_PASSWORD", "PUBLIC_BASE_URL", "DATABASE_URL",
        "ALLOWED_ORIGINS", "TRUSTED_PROXIES"
    )
    foreach ($name in $requiredVariables) {
        $value = [Environment]::GetEnvironmentVariable($name, "Process")
        Report-Check (-not [string]::IsNullOrWhiteSpace($value)) "$name is configured"
    }
    Report-Check ($env:SECRET_KEY -ne "GENERATE_ME") "SECRET_KEY is not a template placeholder"
    Report-Check ([Text.Encoding]::UTF8.GetByteCount($env:ADMIN_PASSWORD) -ge 12) "ADMIN_PASSWORD is at least 12 bytes"
}

$docker = Get-Command docker -ErrorAction SilentlyContinue
if ($docker) {
    & $docker.Path info *> $null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "[OK]   Docker engine is running"
        if (Test-Path -LiteralPath $envPath) {
            & $docker.Path compose --env-file $envPath -f (Join-Path $projectRoot "compose.yaml") config --quiet
            Report-Check ($LASTEXITCODE -eq 0) "compose.yaml and environment values are valid"
        }
    }
    else {
        Write-Host "[WARN] Docker engine is not running - embedded PostgreSQL (cmd/localdb) will be used." -ForegroundColor Yellow
    }
}
else {
    Write-Host "[WARN] Docker is not installed - embedded PostgreSQL (cmd/localdb) is used for local development and tests." -ForegroundColor Yellow
}

if ($failures.Count -gt 0) {
    Write-Host "`nEnvironment doctor found $($failures.Count) problem(s)." -ForegroundColor Red
    exit 1
}
Write-Host "`nDevelopment environment is ready." -ForegroundColor Green
