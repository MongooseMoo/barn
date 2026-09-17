[CmdletBinding()]
param(
    [string]$RunDir = '.tmp/mongoose-account-20260917',
    [string]$DebugUrl = 'http://127.0.0.1:11486',
    [ValidateRange(1,30)][int]$Seconds = 5
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
Set-Location (Split-Path $PSScriptRoot -Parent)
$run = (Resolve-Path -LiteralPath $RunDir).Path
$prefix = Join-Path $run ('workload-' + (Get-Date -Format 'yyyyMMdd-HHmmss-fff'))
foreach ($pair in @(@('vars','vars.json'), @('pprof/goroutine?debug=2','stacks.txt'))) {
    Invoke-WebRequest "$DebugUrl/debug/$($pair[0])" -OutFile "$prefix-$($pair[1])" -TimeoutSec 10
}
Invoke-WebRequest "$DebugUrl/debug/pprof/profile?seconds=$Seconds" -OutFile "$prefix.cpu" -TimeoutSec ($Seconds + 10)
& go tool pprof -top -nodecount=20 "$run/barn.exe" "$prefix.cpu" 2>&1 | Tee-Object "$prefix-profile.txt"
$code = $LASTEXITCODE
if ($code -ne 0) { throw "Profile report failed: $code" }
& go run ./cmd/barn_logs -dir "$run/barn/logs" -level warn -n 20 2>&1 | Tee-Object "$prefix-logs.txt"
$code = $LASTEXITCODE
# barn_logs returns 1 when it finds errors; retain that evidence rather than
# treating a workload finding as a failed capture.
Write-Output "barn_logs_exit=$code"
if ($code -gt 1) { throw "Log capture failed: $code" }
Write-Output "Evidence prefix: $prefix"
