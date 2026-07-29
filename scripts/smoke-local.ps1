param(
    [string]$Binary = ".data\goshorty-3c-verify.exe",
    [string]$EnvFile = ".env.local",
    [int]$Port = 18080,
    [switch]$ReleaseMode,
    [switch]$AuthFlow
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot "env.ps1")
Import-DotEnv -Path (Join-Path $projectRoot $EnvFile)

$env:PORT = $Port.ToString()
$env:PUBLIC_BASE_URL = "http://127.0.0.1:$Port"
$env:ALLOWED_ORIGINS = $env:PUBLIC_BASE_URL
if ($ReleaseMode) {
    $env:GIN_MODE = "release"
    $env:METRICS_TOKEN = "local-smoke-metrics-token-32-bytes"
}
$binaryPath = (Resolve-Path -LiteralPath (Join-Path $projectRoot $Binary)).Path
$stdoutPath = Join-Path $projectRoot ".data\smoke-stdout.log"
$stderrPath = Join-Path $projectRoot ".data\smoke-stderr.log"

$startInfo = [Diagnostics.ProcessStartInfo]::new()
$startInfo.FileName = $binaryPath
$startInfo.UseShellExecute = $false
$startInfo.CreateNoWindow = $true
$startInfo.RedirectStandardOutput = $true
$startInfo.RedirectStandardError = $true
$process = [Diagnostics.Process]::new()
$process.StartInfo = $startInfo
if (-not $process.Start()) {
    throw "Failed to start smoke-test server"
}
try {
    $ready = $null
    for ($attempt = 1; $attempt -le 30; $attempt++) {
        try {
            $ready = Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$Port/ready"
            break
        }
        catch {
            Start-Sleep -Milliseconds 500
        }
    }
    if (-not $ready) {
        throw "Server did not become ready"
    }
    $health = Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$Port/health"
    $metricsHeaders = @{}
    if ($ReleaseMode) {
        $unauthorizedStatus = 0
        try {
            $unauthorizedMetrics = Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$Port/metrics"
            $unauthorizedStatus = $unauthorizedMetrics.StatusCode
        }
        catch {
            if ($_.Exception.Response) {
                $unauthorizedStatus = [int]$_.Exception.Response.StatusCode
            }
        }
        if ($unauthorizedStatus -ne 401) {
            throw "Release metrics endpoint must reject missing credentials"
        }
        $metricsHeaders["Authorization"] = "Bearer $($env:METRICS_TOKEN)"
    }
    $metrics = Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$Port/metrics" -Headers $metricsHeaders
    $version = Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$Port/version"
    if ($metrics.Content -notmatch "goshorty_http_requests_total") {
        throw "Metrics response is missing request counters"
    }
    if (-not $ready.Headers["X-Request-ID"]) {
        throw "Readiness response is missing X-Request-ID"
    }
    if ($version.Content -notmatch '"version"') {
        throw "Version response is missing release metadata"
    }
    if ($AuthFlow) {
        $loginBody = @{
            username = "admin"
            password = $env:ADMIN_PASSWORD
        } | ConvertTo-Json
        $login = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:$Port/api/auth/login" `
            -ContentType "application/json" -Body $loginBody
        $authHeaders = @{ Authorization = "Bearer $($login.token)" }
        Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:$Port/api/auth/revoke" `
            -Headers $authHeaders | Out-Null
        $revokedStatus = 0
        try {
            Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$Port/api/auth/me" `
                -Headers $authHeaders | Out-Null
        }
        catch {
            if ($_.Exception.Response) {
                $revokedStatus = [int]$_.Exception.Response.StatusCode
            }
        }
        if ($revokedStatus -ne 401) {
            throw "Revoked token must return HTTP 401"
        }
    }
    Write-Output "READY=$($ready.StatusCode) HEALTH=$($health.StatusCode) METRICS=$($metrics.StatusCode) VERSION=$($version.StatusCode)"
    Write-Output "REQUEST_ID=$($ready.Headers['X-Request-ID'])"
}
finally {
    if (-not $process.HasExited) {
        $process.Kill()
        $process.WaitForExit()
    }
    $process.StandardOutput.ReadToEnd() | Set-Content -LiteralPath $stdoutPath
    $process.StandardError.ReadToEnd() | Set-Content -LiteralPath $stderrPath
    $process.Dispose()
}

$requestLog = Get-Content -LiteralPath $stdoutPath |
    Where-Object { $_ -match '"msg":"http_request"' } |
    Select-Object -First 1
if (-not $requestLog) {
    throw "Structured request log was not emitted"
}
Write-Output "JSON_LOG_OK"
if ($AuthFlow) {
    $auditLog = Get-Content -LiteralPath $stdoutPath |
        Where-Object { $_ -match '"msg":"security_audit"' } |
        Select-Object -First 1
    if (-not $auditLog) {
        throw "Structured security audit log was not emitted"
    }
    Write-Output "AUTH_REVOKE_AUDIT_OK"
}
