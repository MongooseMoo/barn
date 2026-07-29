[CmdletBinding()]
param(
    [string]$Binary = ".\barn_parity.exe",
    [string]$SourceDb = "Test_conf.db",
    [string]$RunDb = "Test_run.db",
    [int]$Port = 7788,
    [string]$ServerHost = "127.0.0.1",
    [switch]$Build,
    [string]$BuildTarget = "./cmd/barn/",
    [string]$PytestModule = "moo_conformance",
    [string]$K = "",
    [string[]]$ExtraPytestArgs = @(),
    [string[]]$ExtraServerArgs = @(),
    [string]$ReportsRoot = "reports/runs",
    [switch]$KeepRunDb,
    [switch]$NoFreshDb
)

$ErrorActionPreference = "Stop"

function Write-Section {
    param([string]$Text)
    Write-Host ""
    Write-Host "=== $Text ==="
}

$runId = Get-Date -Format "yyyyMMdd_HHmmss"
$runDir = Join-Path $ReportsRoot $runId
New-Item -ItemType Directory -Path $runDir -Force | Out-Null
$runDir = [System.IO.Path]::GetFullPath($runDir)

$pytestLog = Join-Path $runDir "pytest.log"
$pytestCmdFile = Join-Path $runDir "pytest.command.txt"
$failedTestsFile = Join-Path $runDir "failed-tests.txt"
$summaryFile = Join-Path $runDir "summary.json"

if ($Build) {
    Write-Section "Build"
    & go build -o $Binary $BuildTarget
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed with exit code $LASTEXITCODE"
    }
}

if (-not (Test-Path $Binary)) {
    throw "Server binary not found: $Binary"
}

if (-not $NoFreshDb) {
    if (-not (Test-Path $SourceDb)) {
        throw "Source DB not found: $SourceDb"
    }
    Write-Section "Prepare DB"
    Copy-Item -Force $SourceDb $RunDb
    Write-Host "Copied $SourceDb -> $RunDb"
} elseif (-not (Test-Path $RunDb)) {
    throw "Run DB not found and -NoFreshDb set: $RunDb"
}

$serverBinary = [System.IO.Path]::GetFullPath($Binary)
$managedDb = [System.IO.Path]::GetFullPath($RunDb)
$escapedServerBinary = $serverBinary.Replace('"', '\"')
$serverCommand = "`"$escapedServerBinary`" -db {db} -port {port}"
if ($ExtraServerArgs.Count -gt 0) {
    $serverCommand += " " + ($ExtraServerArgs -join " ")
}

$pytestArgs = @(
    "run",
    "pytest",
    "--pyargs",
    $PytestModule,
    "--server-command=$serverCommand",
    "--server-db=$managedDb",
    "--moo-host=$ServerHost",
    "--moo-port=$Port",
    "-v"
)
if ($K -ne "") {
    $pytestArgs += @("-k", $K)
}
if ($ExtraPytestArgs.Count -gt 0) {
    $pytestArgs += $ExtraPytestArgs
}
$pytestCmdText = "uv " + ($pytestArgs -join " ")
$pytestCmdText | Set-Content -Path $pytestCmdFile

$pytestExit = 1

Write-Section "Run"
Write-Host "Run ID: $runId"
Write-Host "Run Dir: $runDir"
Write-Host "Server:  $serverCommand"
Write-Host "Pytest:  $pytestCmdText"

& uv @pytestArgs 2>&1 | Tee-Object -FilePath $pytestLog
$pytestExit = $LASTEXITCODE

$failedLines = @(Select-String -Path $pytestLog -Pattern '^FAILED ' | ForEach-Object { $_.Line })
if ($failedLines.Count -gt 0) {
    $failedLines | Set-Content -Path $failedTestsFile
} else {
    "" | Set-Content -Path $failedTestsFile
}

$summaryLine = (Select-String -Path $pytestLog -Pattern '={5,}\s+.+\s+in\s+.+' | Select-Object -Last 1)
$summaryText = if ($null -ne $summaryLine) { $summaryLine.Line.Trim() } else { "(pytest summary line not found)" }

$summary = [ordered]@{
    run_id = $runId
    timestamp_utc = (Get-Date).ToUniversalTime().ToString("o")
    binary = [System.IO.Path]::GetFullPath($Binary)
    source_db = [System.IO.Path]::GetFullPath($SourceDb)
    run_db = [System.IO.Path]::GetFullPath($RunDb)
    host = $ServerHost
    port = $Port
    pytest_exit_code = $pytestExit
    pytest_summary = $summaryText
    failed_count = $failedLines.Count
    run_dir = [System.IO.Path]::GetFullPath($runDir)
    server_command = $serverCommand
    pytest_command = $pytestCmdText
    pytest_command_file = [System.IO.Path]::GetFullPath($pytestCmdFile)
    pytest_log = [System.IO.Path]::GetFullPath($pytestLog)
    failed_tests_file = [System.IO.Path]::GetFullPath($failedTestsFile)
}
$summary | ConvertTo-Json -Depth 4 | Set-Content -Path $summaryFile

if (-not $KeepRunDb -and -not $NoFreshDb) {
    Remove-Item -Force $RunDb
}

Write-Section "Summary"
Write-Host $summaryText
Write-Host "Failed tests: $($failedLines.Count)"
Write-Host "Pytest log:   $pytestLog"
Write-Host "Summary JSON: $summaryFile"

exit $pytestExit
