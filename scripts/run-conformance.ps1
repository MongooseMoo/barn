[CmdletBinding()]
param(
    [string]$Binary = ".\barn_parity.exe",
    [string]$SourceDb = "Test_conf.db",
    [string]$RunDb = "Test_run.db",
    [int]$Port = 7788,
    [string]$ServerHost = "127.0.0.1",
    [switch]$Build,
    [string]$BuildTarget = "./cmd/barn/",
    [string]$K = "",
    [string[]]$ExtraConformanceArgs = @(),
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

$conformanceLog = Join-Path $runDir "conformance.log"
$conformanceCmdFile = Join-Path $runDir "conformance.command.txt"
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

$serverDb = if ($NoFreshDb) { $RunDb } else { $SourceDb }
if (-not (Test-Path $serverDb)) {
    throw "Server DB not found: $serverDb"
}

$binaryPath = [System.IO.Path]::GetFullPath($Binary)
$serverCommand = "`"$binaryPath`" -db {db} -port {port}"
$conformanceArgs = @(
    "run",
    "moo-conformance",
    "--server-command",
    $serverCommand,
    "--server-db",
    $serverDb,
    "--moo-host=$ServerHost",
    "--moo-port=$Port",
    "-v"
)
if ($K -ne "") {
    $conformanceArgs += @("-k", $K)
}
if ($ExtraConformanceArgs.Count -gt 0) {
    $conformanceArgs += $ExtraConformanceArgs
}
$conformanceCmdText = "uv " + ($conformanceArgs -join " ")
$conformanceCmdText | Set-Content -Path $conformanceCmdFile
$conformanceExit = 1

Write-Section "Run"
Write-Host "Run ID: $runId"
Write-Host "Run Dir: $runDir"
Write-Host "Command: $conformanceCmdText"

& uv @conformanceArgs 2>&1 | Tee-Object -FilePath $conformanceLog
$conformanceExit = $LASTEXITCODE

$failedLines = @(Select-String -Path $conformanceLog -Pattern '^FAILED ' | ForEach-Object { $_.Line })
if ($failedLines.Count -gt 0) {
    $failedLines | Set-Content -Path $failedTestsFile
} else {
    "" | Set-Content -Path $failedTestsFile
}

$summaryLine = (Select-String -Path $conformanceLog -Pattern '={5,}\s+.+\s+in\s+.+' | Select-Object -Last 1)
$summaryText = if ($null -ne $summaryLine) { $summaryLine.Line.Trim() } else { "(conformance summary line not found)" }

$summary = [ordered]@{
    run_id = $runId
    timestamp_utc = (Get-Date).ToUniversalTime().ToString("o")
    binary = $binaryPath
    server_db = [System.IO.Path]::GetFullPath($serverDb)
    host = $ServerHost
    port = $Port
    conformance_exit_code = $conformanceExit
    conformance_summary = $summaryText
    failed_count = $failedLines.Count
    run_dir = [System.IO.Path]::GetFullPath($runDir)
    conformance_command = $conformanceCmdText
    conformance_command_file = [System.IO.Path]::GetFullPath($conformanceCmdFile)
    conformance_log = [System.IO.Path]::GetFullPath($conformanceLog)
    failed_tests_file = [System.IO.Path]::GetFullPath($failedTestsFile)
}
$summary | ConvertTo-Json -Depth 4 | Set-Content -Path $summaryFile

Write-Section "Summary"
Write-Host $summaryText
Write-Host "Failed tests: $($failedLines.Count)"
Write-Host "Log:          $conformanceLog"
Write-Host "Summary JSON: $summaryFile"

exit $conformanceExit
