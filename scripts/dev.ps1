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
    $localdbPort = $env:LOCALDB_PORT
    if (-not $localdbPort) {
        $localdbPort = "5433"
    }
    $localdbDir = Join-Path $projectRoot ".localdb"
    $localdbExe = Join-Path $localdbDir "localdb.exe"
    $localdbDataDir = Join-Path $localdbDir "data"
    New-Item -ItemType Directory -Force -Path $localdbDir | Out-Null

    Push-Location $projectRoot
    try {
        go build -buildvcs=false -o $localdbExe ./cmd/localdb
        if ($LASTEXITCODE -ne 0) {
            throw "Failed to build ./cmd/localdb"
        }
    }
    finally {
        Pop-Location
    }

    # .env.local points DATABASE_URL at the Docker PostgreSQL on 5432; the
    # embedded instance replaces it so the app gets real persistence.
    $env:DATABASE_URL = "postgres://goshorty:goshorty@127.0.0.1:$localdbPort/goshorty?sslmode=disable"

    $stdoutFile = Join-Path $localdbDir "localdb.out.log"
    $stderrFile = Join-Path $localdbDir "localdb.err.log"
    $proc = Start-Process -FilePath $localdbExe `
        -ArgumentList "-port", $localdbPort, "-data", $localdbDataDir `
        -PassThru -NoNewWindow -RedirectStandardOutput $stdoutFile -RedirectStandardError $stderrFile
    try {
        $ready = $false
        for ($attempt = 1; $attempt -le 60; $attempt++) {
            if ($proc.HasExited) {
                throw "Embedded PostgreSQL exited early. See $stderrFile"
            }
            if (Test-NetConnection -ComputerName 127.0.0.1 -Port $localdbPort `
                    -WarningAction SilentlyContinue -InformationLevel Quiet) {
                $ready = $true
                break
            }
            Start-Sleep -Seconds 1
        }
        if (-not $ready) {
            throw "Embedded PostgreSQL did not become ready on port $localdbPort. See $stderrFile"
        }
        Write-Host "Embedded PostgreSQL ready at $env:DATABASE_URL"

        Push-Location $projectRoot
        try {
            go run .
        }
        finally {
            Pop-Location
        }
    }
    finally {
        if (-not $proc.HasExited) {
            Stop-Process -Id $proc.Id -Force
            $proc.WaitForExit()
        }
    }
}
