param(
    [string]$Destination = ".env.local"
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$template = Join-Path $projectRoot ".env.example"
$destinationPath = Join-Path $projectRoot $Destination

if (Test-Path -LiteralPath $destinationPath) {
    Write-Host "$Destination already exists; no values were overwritten."
    exit 0
}

$bytes = [byte[]]::new(32)
$generator = [Security.Cryptography.RandomNumberGenerator]::Create()
try {
    $generator.GetBytes($bytes)
}
finally {
    $generator.Dispose()
}
$secret = ([BitConverter]::ToString($bytes) -replace "-", "").ToLowerInvariant()
$content = (Get-Content -LiteralPath $template -Raw).Replace(
    "SECRET_KEY=GENERATE_ME",
    "SECRET_KEY=$secret"
)
[IO.File]::WriteAllText($destinationPath, $content, [Text.UTF8Encoding]::new($false))

Write-Host "Created $Destination with a random local SECRET_KEY."
Write-Host "Local admin password: local-admin-password-change-me"
Write-Host "This file is ignored by Git. Do not reuse its credentials elsewhere."
