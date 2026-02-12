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
    [string]$ReportsRoot = "reports/runs",
    [int]$StartupTimeoutSec = 20,
    [switch]$KeepRunDb,
    [switch]$NoFreshDb
)

$ErrorActionPreference = "Stop"

function Wait-ForTcpPort {
    param(
        [string]$WaitHost,
        [int]$WaitPort,
        [int]$TimeoutSec
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        try {
            $client = New-Object System.Net.Sockets.TcpClient
            $iar = $client.BeginConnect($WaitHost, $WaitPort, $null, $null)
            if ($iar.AsyncWaitHandle.WaitOne(500)) {
                $client.EndConnect($iar)
                $client.Close()
                return $true
            }
            $client.Close()
        } catch {
            # Port not ready yet.
        }
        Start-Sleep -Milliseconds 200
    }
    return $false
}

function Write-Section {
    param([string]$Text)
    Write-Host ""
    Write-Host "=== $Text ==="
}

$runId = Get-Date -Format "yyyyMMdd_HHmmss"
$runDir = Join-Path $ReportsRoot $runId
New-Item -ItemType Directory -Path $runDir -Force | Out-Null

$serverOutLog = Join-Path $runDir "server.stdout.log"
$serverErrLog = Join-Path $runDir "server.stderr.log"
$pytestLog = Join-Path $runDir "pytest.log"
$pytestCmdFile = Join-Path $runDir "pytest.command.txt"
$failedTestsFile = Join-Path $runDir "failed-tests.txt"
$serverAlertsFile = Join-Path $runDir "server-alerts.txt"
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

$pytestArgs = @("run", "pytest", "--pyargs", $PytestModule, "--moo-port=$Port", "-v")
if ($K -ne "") {
    $pytestArgs += @("-k", $K)
}
if ($ExtraPytestArgs.Count -gt 0) {
    $pytestArgs += $ExtraPytestArgs
}
$pytestCmdText = "uv " + ($pytestArgs -join " ")
$pytestCmdText | Set-Content -Path $pytestCmdFile

$server = $null
$pytestExit = 1

Write-Section "Run"
Write-Host "Run ID: $runId"
Write-Host "Run Dir: $runDir"
Write-Host "Server:  $Binary -db $RunDb -port $Port"
Write-Host "Pytest:  $pytestCmdText"

try {
    $server = Start-Process -FilePath $Binary `
        -ArgumentList @("-db", $RunDb, "-port", $Port.ToString()) `
        -RedirectStandardOutput $serverOutLog `
        -RedirectStandardError $serverErrLog `
        -PassThru

    if (-not (Wait-ForTcpPort -WaitHost $ServerHost -WaitPort $Port -TimeoutSec $StartupTimeoutSec)) {
        throw "Server failed to accept TCP connections on $ServerHost`:$Port within $StartupTimeoutSec seconds."
    }

    & uv @pytestArgs 2>&1 | Tee-Object -FilePath $pytestLog
    $pytestExit = $LASTEXITCODE
}
finally {
    if ($null -ne $server -and -not $server.HasExited) {
        Stop-Process -Id $server.Id -Force
    }
}

$failedLines = @(Select-String -Path $pytestLog -Pattern '^FAILED ' | ForEach-Object { $_.Line })
if ($failedLines.Count -gt 0) {
    $failedLines | Set-Content -Path $failedTestsFile
} else {
    "" | Set-Content -Path $failedTestsFile
}

$alertMatches = @(
    Select-String -Path @($serverOutLog, $serverErrLog) `
        -Pattern 'panic:|runtime error|fatal|Command dispatch error|Traceback for player|user_connected error|user_disconnected error' `
        -CaseSensitive:$false
)
if ($alertMatches.Count -gt 0) {
    $alertMatches | ForEach-Object { "{0}:{1}: {2}" -f $_.Path, $_.LineNumber, $_.Line } | Set-Content -Path $serverAlertsFile
} else {
    "" | Set-Content -Path $serverAlertsFile
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
    pytest_command = $pytestCmdText
    pytest_command_file = [System.IO.Path]::GetFullPath($pytestCmdFile)
    pytest_log = [System.IO.Path]::GetFullPath($pytestLog)
    failed_tests_file = [System.IO.Path]::GetFullPath($failedTestsFile)
    server_stdout_log = [System.IO.Path]::GetFullPath($serverOutLog)
    server_stderr_log = [System.IO.Path]::GetFullPath($serverErrLog)
    server_alerts_file = [System.IO.Path]::GetFullPath($serverAlertsFile)
}
$summary | ConvertTo-Json -Depth 4 | Set-Content -Path $summaryFile

if (-not $KeepRunDb -and -not $NoFreshDb) {
    Remove-Item -Force $RunDb
}

Write-Section "Summary"
Write-Host $summaryText
Write-Host "Failed tests: $($failedLines.Count)"
Write-Host "Pytest log:   $pytestLog"
Write-Host "Server out:   $serverOutLog"
Write-Host "Server err:   $serverErrLog"
Write-Host "Alerts:       $serverAlertsFile"
Write-Host "Summary JSON: $summaryFile"

exit $pytestExit
