# GoShorty API Testing Script (PowerShell)
# Run this script to test all API endpoints

$ApiUrl = "http://localhost:8080"
$headers = @{ "Content-Type" = "application/json" }

function Test-Endpoint {
    param(
        [string]$Name,
        [string]$Method,
        [string]$Endpoint,
        [string]$Body = $null
    )
    
    Write-Host "`n=== $Name ===" -ForegroundColor Green
    
    $params = @{
        Uri     = "$ApiUrl$Endpoint"
        Method  = $Method
        Headers = $headers
    }
    
    if ($Body) {
        $params["Body"] = $Body
    }
    
    try {
        $response = Invoke-RestMethod @params
        $response | ConvertTo-Json -Depth 10
    }
    catch {
        Write-Host "Error: $_" -ForegroundColor Red
    }
}

Write-Host "GoShorty API Testing" -ForegroundColor Blue

# Test 1: Health Check
Test-Endpoint -Name "1. Health Check" -Method "GET" -Endpoint "/health"

# Test 2: Create Short URL
Write-Host "`n=== 2. Create Short URL ===" -ForegroundColor Green
$body = @{
    url        = "https://github.com/torvalds/linux"
    expires_in = "24h"
} | ConvertTo-Json

$response = Invoke-RestMethod -Uri "$ApiUrl/api/shorten" -Method POST -Headers $headers -Body $body
$response | ConvertTo-Json -Depth 10
$shortCode = $response.short_code

# Test 3: Create with Custom Code
$body = @{
    url         = "https://golang.org"
    expires_in  = "1h"
    custom_code = "go"
} | ConvertTo-Json

Test-Endpoint -Name "3. Create with Custom Code" -Method "POST" -Endpoint "/api/shorten" -Body $body

# Test 4: Get URL Info
Test-Endpoint -Name "4. Get URL Info" -Method "GET" -Endpoint "/api/shorten/$shortCode"

# Test 5: Statistics
Test-Endpoint -Name "5. Get Statistics" -Method "GET" -Endpoint "/api/stats"

# Test 6: Create 5-minute TTL
$body = @{
    url        = "https://example.com"
    expires_in = "5m"
} | ConvertTo-Json

Test-Endpoint -Name "6. Create 5-minute TTL" -Method "POST" -Endpoint "/api/shorten" -Body $body

# Test 7: List All URLs
Test-Endpoint -Name "7. List All URLs" -Method "GET" -Endpoint "/api/shorten/all"

Write-Host "`n=== Testing Complete ===" -ForegroundColor Blue
