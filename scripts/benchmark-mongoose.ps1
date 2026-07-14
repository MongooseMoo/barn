[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidateSet("Toast", "Barn")]
    [string]$Engine,

    [Parameter(Mandatory)]
    [string]$Database,

    [Parameter(Mandatory)]
    [string]$OutputDir,

    [int]$Port = 9472,
    [int]$DebugPort = 6199,
    [int]$SettleSeconds = 180,
    [int]$StartupTimeoutSeconds = 120,
    [string]$BarnExecutable,
    [string]$ClientExecutable,
    [string]$ToastExecutable = "/root/src/toaststunt-mongoose/build-release/moo"
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
if (-not $BarnExecutable) {
    $BarnExecutable = Join-Path $repoRoot ".tmp/mongoose-convergence/barn.exe"
}
if (-not $ClientExecutable) {
    $ClientExecutable = Join-Path $repoRoot ".tmp/mongoose-convergence/moo_client.exe"
}
if (-not $env:MONGOOSE_LOGIN_SCRIPT) {
    throw "MONGOOSE_LOGIN_SCRIPT must contain the newline-separated PROXY, account, and password commands"
}
if (-not (Test-Path -LiteralPath $Database)) {
    throw "Database not found: $Database"
}
if (-not (Test-Path -LiteralPath $ClientExecutable)) {
    throw "Client executable not found: $ClientExecutable"
}
if ($Engine -eq "Barn" -and -not (Test-Path -LiteralPath $BarnExecutable)) {
    throw "Barn executable not found: $BarnExecutable"
}

$OutputDir = [System.IO.Path]::GetFullPath($OutputDir)
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
$runDatabase = Join-Path $OutputDir "run.db"
Copy-Item -LiteralPath $Database -Destination $runDatabase -Force
$checkpointPath = "$runDatabase.new"
Remove-Item -LiteralPath $checkpointPath -Force -ErrorAction SilentlyContinue

$sourceHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $Database).Hash.ToLowerInvariant()
$copyHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $runDatabase).Hash.ToLowerInvariant()
if ($sourceHash -ne $copyHash) {
    throw "Disposable database hash differs from source"
}

$loginCommands = @($env:MONGOOSE_LOGIN_SCRIPT -split "`r?`n" | Where-Object { $_ -ne "" })
if ($loginCommands.Count -ne 3) {
    throw "MONGOOSE_LOGIN_SCRIPT must contain exactly three non-empty lines"
}
$loginCommands[0] = $loginCommands[0].Replace("{port}", $Port.ToString())
$commands = @(
    $loginCommands[0],
    $loginCommands[1],
    $loginCommands[2],
    "look",
    "west",
    "@who",
    ";{length(queued_tasks()), length(connected_players())}",
    ";dump_database()"
)
$commandsPath = Join-Path $OutputDir "client-commands.tmp"
$clientErrorPath = Join-Path $OutputDir "client-stderr.tmp"
$transcriptPath = Join-Path $OutputDir "client-output.txt"
$eventPath = Join-Path $OutputDir "client-events.jsonl"
Set-Content -LiteralPath $commandsPath -Value $commands -Encoding utf8

$serverOutput = Join-Path $OutputDir "server-stdout.txt"
$serverError = Join-Path $OutputDir "server-stderr.txt"
$profilePath = Join-Path $OutputDir "profile.json"
$serverProcess = $null
$clientProcess = $null
$serverStartedAt = Get-Date

function Test-Listener {
    if ($Engine -eq "Barn") {
        return $null -ne (Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue)
    }
    $listener = & wsl -d Debian -u root -e bash -lc "ss -H -ltn 'sport = :$Port'" 2>$null
    return -not [string]::IsNullOrWhiteSpace(($listener | Out-String))
}

function Get-SendEvent([object[]]$Events, [int]$Index) {
    return $Events | Where-Object { $_.event -eq "send" -and $_.command_index -eq $Index } | Select-Object -First 1
}

function Get-ReceiveLatency([object[]]$Events, [int]$Index, [string]$Pattern) {
    $send = Get-SendEvent $Events $Index
    if ($null -eq $send) {
        return $null
    }
    $text = ""
    foreach ($event in $Events | Where-Object { $_.event -eq "receive" -and $_.elapsed_ms -ge $send.elapsed_ms }) {
        $text += $event.text
        if ([string]::IsNullOrEmpty($Pattern) -or $text -match $Pattern) {
            return [int64]$event.elapsed_ms - [int64]$send.elapsed_ms
        }
    }
    return $null
}

function Get-ToastProcessSample {
    $lines = & wsl -d Debian -u root -e ps -C moo -o pid=,rss=,pcpu=,etime=,args= 2>$null
    foreach ($line in $lines) {
        if ($line -match "^\s*(\d+)\s+(\d+)\s+([0-9.]+)\s+(\S+)\s+(.+)$" -and $matches[5] -like "*$Port*") {
            return [ordered]@{
                pid = [int]$matches[1]
                rss_bytes = [int64]$matches[2] * 1024
                cpu_percent = [double]$matches[3]
                elapsed = $matches[4]
            }
        }
    }
    return $null
}

try {
    if ($Engine -eq "Barn") {
        $arguments = @(
            "--db", $runDatabase,
            "--listen", "tcp://127.0.0.1:$Port",
            "--checkpoint-interval", "0",
            "--config", (Join-Path $repoRoot "profiles/barn/mongoose-outbound-on.conf"),
            "--profile-id", "barn-windows-mongoose-outbound-on",
            "--profile-manifest", $profilePath,
            "--debug-addr", "127.0.0.1:$DebugPort",
            "--log-dir", (Join-Path $OutputDir "logs")
        )
        $serverProcess = Start-Process -FilePath $BarnExecutable -ArgumentList $arguments -PassThru -WindowStyle Hidden -RedirectStandardOutput $serverOutput -RedirectStandardError $serverError
        $hostName = "127.0.0.1"
        $engineRef = (Get-FileHash -Algorithm SHA256 -LiteralPath $BarnExecutable).Hash.ToLowerInvariant()
    } else {
        $wrapper = "/mnt/c/Users/Q/code/barn/scripts/run_toast_wsl.sh"
        $arguments = @("-d", "Debian", "-u", "root", "-e", "env", "TOAST_MOO=$ToastExecutable", "bash", $wrapper, $runDatabase, $Port)
        $serverProcess = Start-Process -FilePath "wsl.exe" -ArgumentList $arguments -PassThru -WindowStyle Hidden -RedirectStandardOutput $serverOutput -RedirectStandardError $serverError
        $hostName = (& wsl -d Debian -u root -e hostname -I).Trim().Split(" ")[0]
        $engineRef = ((& wsl -d Debian -u root -e sha256sum $ToastExecutable) -split "\s+")[0]
    }

    $startupWatch = [System.Diagnostics.Stopwatch]::StartNew()
    while (-not (Test-Listener)) {
        if ($serverProcess.HasExited) {
            throw "$Engine exited before listening; see $serverError"
        }
        if ($startupWatch.Elapsed.TotalSeconds -ge $StartupTimeoutSeconds) {
            throw "$Engine did not listen within $StartupTimeoutSeconds seconds"
        }
        Start-Sleep -Milliseconds 100
        $serverProcess.Refresh()
    }
    $loadToListenMS = [int64]((Get-Date) - $serverStartedAt).TotalMilliseconds

    $clientArguments = @(
        "-host", $hostName,
        "-port", $Port,
        "-file", $commandsPath,
        "-banner-wait", "3000",
        "-inter-cmd", "2500",
        "-timeout", "15",
        "-max-duration", "40",
        "-event-log", $eventPath
    )
    $clientStartedAt = Get-Date
    $clientProcess = Start-Process -FilePath $ClientExecutable -ArgumentList $clientArguments -PassThru -WindowStyle Hidden -RedirectStandardOutput $transcriptPath -RedirectStandardError $clientErrorPath
    $checkpointObservedAt = $null
    while (-not $clientProcess.HasExited) {
        if ($null -eq $checkpointObservedAt -and (Test-Path -LiteralPath $checkpointPath)) {
            $checkpointObservedAt = Get-Date
        }
        Start-Sleep -Milliseconds 50
        $clientProcess.Refresh()
    }
    if ($clientProcess.ExitCode -ne 0) {
        throw "moo_client exited $($clientProcess.ExitCode); see $clientErrorPath"
    }
    if ($null -eq $checkpointObservedAt -and (Test-Path -LiteralPath $checkpointPath)) {
        $checkpointObservedAt = Get-Date
    }

    $events = @(Get-Content -LiteralPath $eventPath | ForEach-Object { $_ | ConvertFrom-Json })
    $connected = $events | Where-Object { $_.event -eq "connected" } | Select-Object -First 1
    $firstReceive = $events | Where-Object { $_.event -eq "receive" } | Select-Object -First 1
    $checkpointSend = Get-SendEvent $events 8
    $checkpointDurationMS = $null
    if ($null -ne $checkpointObservedAt -and $null -ne $checkpointSend) {
        $checkpointFileMS = [int64]($checkpointObservedAt - $clientStartedAt).TotalMilliseconds
        $checkpointDurationMS = $checkpointFileMS - [int64]$checkpointSend.elapsed_ms
    }

    Start-Sleep -Seconds $SettleSeconds
    if ($Engine -eq "Barn") {
        $serverProcess.Refresh()
        $cpuBefore = $serverProcess.TotalProcessorTime.TotalSeconds
        Start-Sleep -Seconds 10
        $serverProcess.Refresh()
        $resourceSample = [ordered]@{
            pid = $serverProcess.Id
            rss_bytes = $serverProcess.WorkingSet64
            cpu_percent = (($serverProcess.TotalProcessorTime.TotalSeconds - $cpuBefore) / 10.0) * 100.0
            elapsed = [int64]((Get-Date) - $serverStartedAt).TotalSeconds
        }
        $debugVars = Invoke-RestMethod -Uri "http://127.0.0.1:$DebugPort/debug/vars"
        $debugVars | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath (Join-Path $OutputDir "debug-vars.json") -Encoding utf8
        $heapProfilePath = Join-Path $OutputDir "heap.pprof"
        Invoke-WebRequest -Uri "http://127.0.0.1:$DebugPort/debug/pprof/heap?gc=1" -OutFile $heapProfilePath
    } else {
        $resourceSample = Get-ToastProcessSample
        $heapProfilePath = $null
    }

    $summary = [ordered]@{
        engine = $Engine
        engine_ref = $engineRef
        database_sha256 = $sourceHash
        disposable_database_sha256 = $copyHash
        port = $Port
        settle_seconds = $SettleSeconds
        metrics_ms = [ordered]@{
            database_load_to_listen = $loadToListenMS
            connect_to_first_banner = if ($null -ne $connected -and $null -ne $firstReceive) { [int64]$firstReceive.elapsed_ms - [int64]$connected.elapsed_ms } else { $null }
            proxy_to_first_output = Get-ReceiveLatency $events 1 ""
            complete_login = Get-ReceiveLatency $events 1 "Codex's Lab"
            startup_command = Get-ReceiveLatency $events 6 ""
            look = Get-ReceiveLatency $events 4 "Codex's Lab"
            movement = Get-ReceiveLatency $events 5 "You (stand from|walk west)"
            liveness_query = Get-ReceiveLatency $events 7 ""
            checkpoint_reply = Get-ReceiveLatency $events 8 ""
            checkpoint_file = $checkpointDurationMS
        }
        resources = $resourceSample
        artifacts = [ordered]@{
            transcript = $transcriptPath
            events = $eventPath
            checkpoint = if (Test-Path -LiteralPath $checkpointPath) { $checkpointPath } else { $null }
            profile = if (Test-Path -LiteralPath $profilePath) { $profilePath } else { $null }
            heap_profile = if ($null -ne $heapProfilePath -and (Test-Path -LiteralPath $heapProfilePath)) { $heapProfilePath } else { $null }
        }
    }
    $summaryPath = Join-Path $OutputDir "summary.json"
    $summary | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $summaryPath -Encoding utf8
    Get-Content -LiteralPath $summaryPath
} finally {
    if ($null -ne $clientProcess -and -not $clientProcess.HasExited) {
        Stop-Process -Id $clientProcess.Id -Force
    }
    if ($null -ne $serverProcess -and -not $serverProcess.HasExited) {
        Stop-Process -Id $serverProcess.Id -Force
    }
    Remove-Item -LiteralPath $commandsPath -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $clientErrorPath -Force -ErrorAction SilentlyContinue
}
