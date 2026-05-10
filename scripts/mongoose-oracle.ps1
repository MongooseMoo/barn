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
    [string]$ToastBinaryWsl = "/mnt/c/Users/Q/src/toaststunt/moo"
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

    $barnDir = Join-Path $runPath "barn"
    $dbPath = Join-Path $barnDir "mongoose.db"
    $stdoutPath = Join-Path $barnDir "barn.out.log"
    $stderrPath = Join-Path $barnDir "barn.err.log"
    $args = @("-db", $dbPath, "-port", "$BarnPort", "-checkpoint-interval", "0")
    $proc = Start-Process -FilePath $binary -ArgumentList $args -WorkingDirectory $barnDir -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath -WindowStyle Hidden -PassThru
    $pidPath = Join-Path $barnDir "barn.pid"
    $proc.Id | Set-Content $pidPath
    Write-ManifestValue $runPath "barn_pid" $proc.Id
    Write-ManifestValue $runPath "barn_started_utc" ((Get-Date).ToUniversalTime().ToString("o"))
    Write-Output "barn pid=$($proc.Id) port=$BarnPort"
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
    $cmd = "cd '$toastDirWsl' && '$ToastBinaryWsl' -O -4 127.0.0.1 '$dbWsl' '$outDbWsl' -p $ToastPort > '$logWsl' 2>&1 & echo `$! > '$pidWsl'"
    & wsl sh -lc $cmd
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to start Toast via WSL"
    }
    $pid = (Get-Content -Raw $pidPath).Trim()
    Write-ManifestValue $runPath "toast_wsl_pid" $pid
    Write-ManifestValue $runPath "toast_started_utc" ((Get-Date).ToUniversalTime().ToString("o"))
    Write-Output "toast wsl_pid=$pid port=$ToastPort"
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
