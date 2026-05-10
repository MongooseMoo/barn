param(
    [Parameter(Mandatory = $true)]
    [string]$DbPath,

    [Parameter(Mandatory = $true)]
    [int]$Port,

    [string]$ToastBinary = "/mnt/c/Users/Q/src/toaststunt/moo"
)

$ErrorActionPreference = "Stop"

function Convert-ToWslPath {
    param([Parameter(Mandatory = $true)][string]$Path)

    $resolved = [System.IO.Path]::GetFullPath($Path)
    $full = $resolved -replace "\\", "/"
    if ($full -match "^([A-Za-z]):/(.*)$") {
        return "/mnt/$($Matches[1].ToLowerInvariant())/$($Matches[2])"
    }
    return $full
}

$dbWsl = Convert-ToWslPath -Path $DbPath
$outWsl = "$dbWsl.toast.out.db"
$runtimeDir = Split-Path -Parent ([System.IO.Path]::GetFullPath($DbPath))
$fileDirWsl = Convert-ToWslPath -Path (Join-Path $runtimeDir "files")
$execDirWsl = Convert-ToWslPath -Path (Join-Path $runtimeDir "executables")

& wsl -- mkdir -p "$fileDirWsl" "$execDirWsl"
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

& wsl -- "$ToastBinary" -O -4 127.0.0.1 -i "$fileDirWsl" -x "$execDirWsl" "$dbWsl" "$outWsl" -p "$Port"
exit $LASTEXITCODE
