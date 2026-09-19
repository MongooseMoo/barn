[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$FailureList,
    [Parameter(Mandatory)][string]$Binary,
    [string[]]$ExtraServerArgs = @(),
    [string]$RunDir = '.tmp/shared-admission/recheck',
    [int]$Port = 11496
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
Set-Location (Split-Path $PSScriptRoot -Parent)
$failureFile = (Resolve-Path -LiteralPath $FailureList).Path
$binaryPath = (Resolve-Path -LiteralPath $Binary).Path
$run = (New-Item -ItemType Directory -Force -Path $RunDir).FullName
$cases = @(foreach ($line in Get-Content -LiteralPath $failureFile) {
    if ($line -match '^FAILED .+\[(.+\.yaml)::([^\]]+)\]') {
        [pscustomobject]@{ Suite=$Matches[1]; Name=$Matches[2] }
    }
})
if (-not $cases.Count) { throw 'No failed YAML case IDs found.' }
$suites = @($cases.Suite | Sort-Object -Unique | ForEach-Object { "--moo-suite-path=$_" })
$names = @($cases.Name | Sort-Object -Unique)
# Capability admission must accompany the focused cases in this same session.
$selection = (@('capability_admission') + $names) -join ' or '
& "$PSScriptRoot/run-conformance.ps1" -Binary $binaryPath -SourceDb Test_conf.db -RunDb (Join-Path $run 'recheck.db') -Port $Port -ReportsRoot (Join-Path $run 'reports') -K $selection -ExtraPytestArgs $suites -ExtraServerArgs $ExtraServerArgs
$code = $LASTEXITCODE
exit $code
