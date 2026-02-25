# Send Allure test results to the Allure Docker Service.
#
# Usage:
#   .\send_results.ps1 [-GenerateReport]
#
# Environment variables (or set variables below):
#   ALLURE_SERVER        URL of the Allure service (default: http://localhost:5050)
#   ALLURE_PROJECT_ID    Project ID (default: default)
#   ALLURE_RESULTS_DIR   Directory containing allure-results (default: allure-results-example)
#   ALLURE_USERNAME      Username for authentication (optional)
#   ALLURE_PASSWORD      Password for authentication (optional)
#   EXECUTION_NAME       Label for this run shown in the report (optional)
#   EXECUTION_FROM       URL linking back to the CI build (optional)
#   EXECUTION_TYPE       CI system type, e.g. jenkins, github, gitlab (optional)

param(
    [switch]$GenerateReport
)

$ErrorActionPreference = "Stop"

$AllureServer    = if ($env:ALLURE_SERVER)      { $env:ALLURE_SERVER }      else { "http://localhost:5050" }
$ProjectId       = if ($env:ALLURE_PROJECT_ID)  { $env:ALLURE_PROJECT_ID }  else { "default" }
$ResultsDir      = if ($env:ALLURE_RESULTS_DIR) { $env:ALLURE_RESULTS_DIR } else { "allure-results-example" }
$Username        = if ($env:ALLURE_USERNAME)    { $env:ALLURE_USERNAME }    else { "" }
$Password        = if ($env:ALLURE_PASSWORD)    { $env:ALLURE_PASSWORD }    else { "" }
$ExecutionName   = if ($env:EXECUTION_NAME)     { $env:EXECUTION_NAME }     else { "" }
$ExecutionFrom   = if ($env:EXECUTION_FROM)     { $env:EXECUTION_FROM }     else { "" }
$ExecutionType   = if ($env:EXECUTION_TYPE)     { $env:EXECUTION_TYPE }     else { "" }

$ResultsPath = Join-Path $PSScriptRoot $ResultsDir
if (-not (Test-Path $ResultsPath)) {
    Write-Error "Results directory not found: $ResultsPath"
    exit 1
}

$Files = Get-ChildItem -File -Path $ResultsPath
if ($Files.Count -eq 0) {
    Write-Error "No files found in $ResultsPath"
    exit 1
}

# --- Build request body (base64-encoded file contents) ---
$Results = @()
foreach ($File in $Files) {
    $Bytes = [IO.File]::ReadAllBytes($File.FullName)
    if ($Bytes.Length -gt 0) {
        $Results += @{
            file_name      = $File.Name
            content_base64 = [Convert]::ToBase64String($Bytes)
        }
    } else {
        Write-Host "Skipping empty file: $($File.Name)"
    }
}

$RequestBody = ConvertTo-Json -Depth 3 @{ results = $Results }

# --- Authentication (optional) ---
$Headers = @{ "Content-Type" = "application/json" }
if ($Username -ne "" -and $Password -ne "") {
    Write-Host "--- LOGIN ---"
    $LoginBody = ConvertTo-Json @{ username = $Username; password = $Password }
    $LoginResponse = Invoke-RestMethod -Uri "$AllureServer/login" `
        -Method POST -ContentType "application/json" -Body $LoginBody
    $AccessToken = $LoginResponse.data.access_token
    if (-not $AccessToken) {
        Write-Error "Login failed. Check credentials."
        exit 1
    }
    $Headers["Authorization"] = "Bearer $AccessToken"
    Write-Host "Login successful."
}

# --- Send results ---
Write-Host "--- SEND RESULTS ---"
$SendUri = "$AllureServer/send-results?project_id=$ProjectId"
$SendResponse = Invoke-RestMethod -Uri $SendUri -Method POST -Headers $Headers -Body $RequestBody
Write-Host "Status: $($SendResponse.meta_data.message)"

# --- Generate report (optional) ---
if ($GenerateReport) {
    Write-Host "--- GENERATE REPORT ---"
    $GenUri = "$AllureServer/generate-report?project_id=$ProjectId"
    if ($ExecutionName) { $GenUri += "&execution_name=$([Uri]::EscapeDataString($ExecutionName))" }
    if ($ExecutionFrom)  { $GenUri += "&execution_from=$([Uri]::EscapeDataString($ExecutionFrom))" }
    if ($ExecutionType)  { $GenUri += "&execution_type=$ExecutionType" }

    $GenResponse = Invoke-RestMethod -Uri $GenUri -Method POST -Headers $Headers
    $ReportUrl = $GenResponse.data.report_url
    Write-Host "Report URL: $ReportUrl"
}
