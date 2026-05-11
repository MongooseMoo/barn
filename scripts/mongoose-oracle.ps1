[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("prepare", "start-barn", "start-toast", "stop", "status")]
    [string]$Action,

    [string]$Root = ".tmp\mongoose-oracle",
    [string]$RunDir = "",
    [string]$SourceDb = "",
    [string]$BarnBinary = "",
    [int]$BarnPort = 17880,
    [int]$ToastPort = 17881,
    [string]$ToastBinaryWsl = "/mnt/c/Users/Q/src/toaststunt/moo",
    [int]$ReadyTimeoutSeconds = 30
)

$ErrorActionPreference = "Stop"

function Resolve-FullPath {
    param([string]$Path)
    return [System.IO.Path]::GetFullPath((Join-Path (Get-Location) $Path))
}

function Convert-ToWslPath {
    param([string]$WindowsPath)
    $full = [System.IO.Path]::GetFullPath($WindowsPath)
    $drive = $full.Substring(0, 1).ToLowerInvariant()
    $rest = $full.Substring(2).Replace("\", "/")
    return "/mnt/$drive$rest"
}

function Get-CurrentRunDir {
    param([string]$RootPath, [string]$ExplicitRunDir)
    if ($ExplicitRunDir -ne "") {
        return Resolve-FullPath $ExplicitRunDir
    }
    $currentFile = Join-Path $RootPath "current-run.txt"
    if (-not (Test-Path $currentFile)) {
        throw "No current run found. Run prepare first or pass -RunDir."
    }
    return [System.IO.Path]::GetFullPath((Get-Content -Raw $currentFile).Trim())
}

function Write-ManifestValue {
    param([string]$RunPath, [string]$Name, [object]$Value)
    $manifestPath = Join-Path $RunPath "manifest.json"
    $manifest = [ordered]@{}
    if (Test-Path $manifestPath) {
        $existing = Get-Content -Raw $manifestPath | ConvertFrom-Json
        foreach ($prop in $existing.PSObject.Properties) {
            $manifest[$prop.Name] = $prop.Value
        }
    }
    $manifest[$Name] = $Value
    $manifest | ConvertTo-Json -Depth 6 | Set-Content -Path $manifestPath
}

function Test-TcpPort {
    param([string]$HostName, [int]$Port)
    $client = [System.Net.Sockets.TcpClient]::new()
    try {
        $connect = $client.BeginConnect($HostName, $Port, $null, $null)
        if (-not $connect.AsyncWaitHandle.WaitOne(250)) {
            return $false
        }
        $client.EndConnect($connect)
        return $true
    } catch {
        return $false
    } finally {
        $client.Close()
    }
}

function Get-ListeningProcess {
    param([int]$Port)
    $connection = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $connection) {
        return $null
    }
    return Get-Process -Id $connection.OwningProcess -ErrorAction SilentlyContinue
}

function Wait-PortClosed {
    param([int]$Port)
    $deadline = (Get-Date).AddSeconds(5)
    while ((Get-Date) -lt $deadline) {
        if (-not (Test-TcpPort "127.0.0.1" $Port)) {
            return
        }
        Start-Sleep -Milliseconds 100
    }
    throw "port $Port remained open after stopping managed listener"
}

function Clear-ManagedBarnListener {
    param([int]$Port, [string]$ExpectedBinary)
    $listener = Get-ListeningProcess $Port
    if (-not $listener) {
        return
    }

    $listenerPath = [System.IO.Path]::GetFullPath($listener.Path)
    $expectedPath = [System.IO.Path]::GetFullPath($ExpectedBinary)
    if ($listenerPath -ne $expectedPath) {
        throw "port $Port is already in use by non-managed process pid=$($listener.Id) path=$listenerPath"
    }

    Stop-Process -Id $listener.Id -ErrorAction SilentlyContinue
    Wait-PortClosed $Port
    Write-Output "stopped stale managed barn pid=$($listener.Id) port=$Port"
}

function Wait-ForManagedListener {
    param(
        [string]$RunPath,
        [string]$Name,
        [int]$Port,
        [string]$LogPath,
        [string]$ReadyText
    )
    $deadline = (Get-Date).AddSeconds($ReadyTimeoutSeconds)
    $sawLog = $false
    while ((Get-Date) -lt $deadline) {
        if ($Name -eq "barn" -and $script:BarnProcess -and $script:BarnProcess.HasExited) {
            $exitCode = $script:BarnProcess.ExitCode
            throw "barn exited before readiness (exit=$exitCode, log=$LogPath)"
        }
        if (-not $sawLog -and (Test-Path $LogPath)) {
            $content = Get-Content -Raw -LiteralPath $LogPath -ErrorAction SilentlyContinue
            if ($content -like "*listen failed*" -or $content -like "*Server error:*") {
                throw "$Name reported startup failure before readiness (log=$LogPath)"
            }
            $sawLog = $content -like "*$ReadyText*"
        }
        if ($sawLog -and (Test-TcpPort "127.0.0.1" $Port)) {
            Write-ManifestValue $RunPath "$($Name)_ready_utc" ((Get-Date).ToUniversalTime().ToString("o"))
            Write-Output "$Name ready port=$Port"
            return
        }
        Start-Sleep -Milliseconds 200
    }
    $logState = if ($sawLog) { "seen" } else { "not seen" }
    $tcpState = if (Test-TcpPort "127.0.0.1" $Port) { "open" } else { "closed" }
    throw "$Name did not become ready within ${ReadyTimeoutSeconds}s (log marker $logState, tcp $tcpState, port=$Port, log=$LogPath)"
}

function New-Run {
    $rootPath = Resolve-FullPath $Root
    $sourcePath = if ($SourceDb -ne "") {
        Resolve-FullPath $SourceDb
    } else {
        Join-Path $rootPath "source\mongoose.db.new"
    }
    if (-not (Test-Path $sourcePath)) {
        throw "Source DB not found: $sourcePath"
    }

    $runId = Get-Date -Format "yyyyMMdd_HHmmss"
    $runPath = Join-Path $rootPath "runs\$runId"
    $barnDir = Join-Path $runPath "barn"
    $toastDir = Join-Path $runPath "toast"
    New-Item -ItemType Directory -Force -Path $barnDir, $toastDir | Out-Null
    Copy-Item -LiteralPath $sourcePath -Destination (Join-Path $barnDir "mongoose.db")
    Copy-Item -LiteralPath $sourcePath -Destination (Join-Path $toastDir "mongoose.db")
    $runPath | Set-Content (Join-Path $rootPath "current-run.txt")

    $hash = (Get-FileHash $sourcePath -Algorithm SHA256).Hash
    $manifest = [ordered]@{
        run_id = $runId
        created_utc = (Get-Date).ToUniversalTime().ToString("o")
        source_db = $sourcePath
        source_sha256 = $hash
        barn_db = Join-Path $barnDir "mongoose.db"
        toast_db = Join-Path $toastDir "mongoose.db"
        barn_port = $BarnPort
        toast_port = $ToastPort
        toast_binary_wsl = $ToastBinaryWsl
    }
    $manifest | ConvertTo-Json -Depth 6 | Set-Content (Join-Path $runPath "manifest.json")
    Write-Output $runPath
}

function Start-BarnServer {
    $runPath = Get-CurrentRunDir $Root $RunDir
    $binary = if ($BarnBinary -ne "") {
        Resolve-FullPath $BarnBinary
    } else {
        Join-Path (Resolve-FullPath $Root) "tools\barn-mongoose.exe"
    }
    if (-not (Test-Path $binary)) {
        throw "Barn binary not found: $binary"
    }
    Clear-ManagedBarnListener $BarnPort $binary

    $barnDir = Join-Path $runPath "barn"
    $dbPath = Join-Path $barnDir "mongoose.db"
    $stdoutPath = Join-Path $barnDir "barn.out.log"
    $stderrPath = Join-Path $barnDir "barn.err.log"
    $args = @("-db", $dbPath, "-port", "$BarnPort", "-checkpoint-interval", "0")
    $proc = Start-Process -FilePath $binary -ArgumentList $args -WorkingDirectory $barnDir -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath -WindowStyle Hidden -PassThru
    $script:BarnProcess = $proc
    $pidPath = Join-Path $barnDir "barn.pid"
    $proc.Id | Set-Content $pidPath
    Write-ManifestValue $runPath "barn_pid" $proc.Id
    Write-ManifestValue $runPath "barn_started_utc" ((Get-Date).ToUniversalTime().ToString("o"))
    Write-ManifestValue $runPath "ready_timeout_seconds" $ReadyTimeoutSeconds
    Write-Output "barn pid=$($proc.Id) port=$BarnPort"
    Wait-ForManagedListener $runPath "barn" $BarnPort $stderrPath "Listening on port $BarnPort"
}

function Start-ToastServer {
    $runPath = Get-CurrentRunDir $Root $RunDir
    $toastDir = Join-Path $runPath "toast"
    $toastDirWsl = Convert-ToWslPath $toastDir
    $dbWsl = "$toastDirWsl/mongoose.db"
    $outDbWsl = "$toastDirWsl/mongoose.toast.out.db"
    $logPath = Join-Path $toastDir "toast.combined.log"
    $logWsl = Convert-ToWslPath $logPath
    $pidPath = Join-Path $toastDir "toast.wsl.pid"
    $pidWsl = Convert-ToWslPath $pidPath
    $launcherPath = Join-Path $toastDir "start-toast.sh"
    $launcher = @"
#!/bin/sh
cd '$toastDirWsl' || exit 1
nohup '$ToastBinaryWsl' -O -4 127.0.0.1 '$dbWsl' '$outDbWsl' -p $ToastPort > '$logWsl' 2>&1 < /dev/null &
echo `$! > '$pidWsl'
"@
    $launcher = $launcher -replace "`r`n", "`n"
    Set-Content -Path $launcherPath -Value $launcher -NoNewline
    $launcherWsl = Convert-ToWslPath $launcherPath
    & wsl sh -lc "chmod +x '$launcherWsl' && '$launcherWsl'"
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to start Toast via WSL"
    }
    $toastPid = (Get-Content -Raw $pidPath).Trim()
    Write-ManifestValue $runPath "toast_wsl_pid" $toastPid
    Write-ManifestValue $runPath "toast_started_utc" ((Get-Date).ToUniversalTime().ToString("o"))
    Write-ManifestValue $runPath "ready_timeout_seconds" $ReadyTimeoutSeconds
    Write-Output "toast wsl_pid=$toastPid port=$ToastPort"
    Wait-ForManagedListener $runPath "toast" $ToastPort $logPath "port $ToastPort"
}

function Stop-Run {
    $runPath = Get-CurrentRunDir $Root $RunDir
    $barnPidPath = Join-Path $runPath "barn\barn.pid"
    if (Test-Path $barnPidPath) {
        $barnPid = (Get-Content -Raw $barnPidPath).Trim()
        if ($barnPid -ne "") {
            Stop-Process -Id ([int]$barnPid) -ErrorAction SilentlyContinue
        }
    }

    $toastPidPath = Join-Path $runPath "toast\toast.wsl.pid"
    if (Test-Path $toastPidPath) {
        $toastPid = (Get-Content -Raw $toastPidPath).Trim()
        if ($toastPid -ne "") {
            & wsl sh -lc "kill '$toastPid' 2>/dev/null || true"
        }
    }
    Write-ManifestValue $runPath "stopped_utc" ((Get-Date).ToUniversalTime().ToString("o"))
    Write-Output "stopped $runPath"
}

function Show-Status {
    $runPath = Get-CurrentRunDir $Root $RunDir
    $manifestPath = Join-Path $runPath "manifest.json"
    if (Test-Path $manifestPath) {
        Get-Content -Raw $manifestPath
    } else {
        Write-Output $runPath
    }
}

switch ($Action) {
    "prepare" { New-Run }
    "start-barn" { Start-BarnServer }
    "start-toast" { Start-ToastServer }
    "stop" { Stop-Run }
    "status" { Show-Status }
}
