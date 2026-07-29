param(
    [string]$BaseUrl = "http://127.0.0.1:8080"
)

$ErrorActionPreference = "Stop"
$bash = Get-Command bash -ErrorAction SilentlyContinue
if (-not $bash) {
    throw "The E2E suite requires bash, curl, and jq. Run it from Git Bash or WSL: bash scripts/e2e.sh $BaseUrl"
}

& $bash.Source "scripts/e2e.sh" $BaseUrl
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}
