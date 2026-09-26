param(
    [string]$EnvFile = ".env.local",
    [switch]$UseDocker
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$envPath = Join-Path $projectRoot $EnvFile
. (Join-Path $PSScriptRoot "env.ps1")
Enable-ProjectToolchain
Import-DotEnv -Path $envPath

$useDocker = $UseDocker -or (Get-Command docker -ErrorAction SilentlyContinue)
if ($useDocker) {
    docker compose --env-file $envPath -f (Join-Path $projectRoot "compose.yaml") up -d postgres
    for ($attempt = 1; $attempt -le 30; $attempt++) {
        docker compose --env-file $envPath -f (Join-Path $projectRoot "compose.yaml") exec -T postgres `
            pg_isready -U $env:POSTGRES_USER -d $env:POSTGRES_DB *> $null
        if ($LASTEXITCODE -eq 0) {
            break
        }
        if ($attempt -eq 30) {
            throw "PostgreSQL did not become ready in time"
        }
        Start-Sleep -Seconds 1
    }
}
else {
    # .env.local may still point TEST_DATABASE_URL at the Docker-only server on
    # port 5432; the embedded TestMain in storage takes over instead.
    Remove-Item Env:TEST_DATABASE_URL -ErrorAction SilentlyContinue
    $env:EMBEDDED_PG = "1"
}

Push-Location $projectRoot
try {
    go test ./... -count=1 -covermode=atomic -coverprofile=coverage.out
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
    go vet ./...
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
    go build ./...
    exit $LASTEXITCODE
}
finally {
    Pop-Location
}
