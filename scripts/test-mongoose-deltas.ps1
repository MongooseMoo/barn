[CmdletBinding()]
param(
    [ValidateSet('Barn','Toast')][string]$Engine = 'Barn',
    [switch]$Packaged,
    [string]$ConformanceRoot = '',
    [string[]]$Suites = @('builtins/notify_output.yaml','audit/background_zero_suspend.yaml','builtins/seconds_elapsed.yaml','builtins/nested_eval_ticks.yaml'),
    [string]$RunDir = '.tmp/mongoose-account-20260917',
    [string]$Distribution = 'Debian',
    [string]$OracleDir = '/root/src/toaststunt-mongoose-login-20260917'
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
Set-Location (Split-Path $PSScriptRoot -Parent)
$run = (Resolve-Path -LiteralPath $RunDir).Path
$repo = (Get-Location).Path
if ($ConformanceRoot) { $ConformanceRoot = (Resolve-Path -LiteralPath $ConformanceRoot).Path }
# These files contain only generic Test.db reductions, never the live workload.
$suiteArgs = @($Suites | ForEach-Object { "--moo-suite-path=$_" })
if ($Engine -eq 'Barn') {
    if (-not $Packaged) { $suiteArgs += @("--candidate-root=$repo", "--moo-suite-root=$repo/tests/mongoose-conformance") }
    $savedPythonPath = $env:PYTHONPATH
    try {
        if ($ConformanceRoot) { $env:PYTHONPATH = Join-Path $ConformanceRoot 'src' }
        & "$PSScriptRoot/run-conformance.ps1" -Binary "$run/barn.exe" -SourceDb Test_conf.db -RunDb "$run/delta-tests.db" -Port 11495 -ReportsRoot "$run/conformance" -ExtraPytestArgs $suiteArgs
        $code = $LASTEXITCODE
    } finally { $env:PYTHONPATH = $savedPythonPath }
    exit $code
}
$installed = (& wsl --list --quiet) -replace "`0", ''
if ($Distribution -notin ($installed | ForEach-Object { $_.Trim() })) { throw "Unknown WSL distribution: $Distribution" }
$linuxRepo = & wsl -d $Distribution --exec wslpath -a (Get-Location).Path.Replace('\','\\')
$code = $LASTEXITCODE
if ($code -ne 0) { throw 'wslpath failed' }
$suiteArgs += "--server-db=$($linuxRepo.Trim())/Test_conf.db"
if (-not $Packaged) { $suiteArgs += @("--candidate-root=$($linuxRepo.Trim())", "--moo-suite-root=$($linuxRepo.Trim())/tests/mongoose-conformance") }
$linuxRun = & wsl -d $Distribution --exec wslpath -a $run.Replace('\','\\')
$code = $LASTEXITCODE
if ($code -ne 0) { throw 'wslpath failed' }
function Quote-Bash([string]$Value) { return "'" + $Value.Replace("'", "'\''") + "'" }
$command = "$OracleDir/build-release/moo {db} {db}.new {port}"
$log = $linuxRun.Trim() + '/toast-deltas-' + (Get-Date -Format 'yyyyMMdd-HHmmss-fff') + '.log'
$body = 'set -euo pipefail; cd ' + (Quote-Bash $linuxRepo.Trim()) + '; export UV_PROJECT_ENVIRONMENT=/root/.cache/barn-mongoose-account-conformance-venv; if uv run --frozen pytest --pyargs moo_conformance --server-command=' + (Quote-Bash $command) + ' --moo-port=17901 -v ' + (($suiteArgs | ForEach-Object { Quote-Bash $_ }) -join ' ') + ' >' + (Quote-Bash $log) + ' 2>&1; then result=0; else result=$?; fi; cat ' + (Quote-Bash $log) + '; exit "$result"'
if ($ConformanceRoot) {
    $linuxConformance = & wsl -d $Distribution --exec wslpath -a $ConformanceRoot.Replace('\','\\')
    $code = $LASTEXITCODE
    if ($code -ne 0) { throw 'Conformance root wslpath failed' }
    $body = $body.Replace('set -euo pipefail; ', 'set -euo pipefail; export PYTHONPATH=' + (Quote-Bash ($linuxConformance.Trim() + '/src')) + '; ')
}
& wsl -d $Distribution --exec bash -lc $body
$code = $LASTEXITCODE
exit $code
