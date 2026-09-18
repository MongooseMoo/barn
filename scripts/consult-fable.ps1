[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$PromptPath,
    [string]$RunDir = '.tmp/fable-consultations',
    [string]$SessionId = '',
    [switch]$Resume
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
Set-Location (Split-Path $PSScriptRoot -Parent)
$promptFile = (Resolve-Path -LiteralPath $PromptPath).Path
if (-not (Test-Path -LiteralPath $promptFile -PathType Leaf)) { throw 'Prompt must be a file.' }
if ($Resume -and -not $SessionId) { throw 'Resume requires SessionId.' }
if (-not $SessionId) { $SessionId = [guid]::NewGuid().ToString() }
$null = [guid]::Parse($SessionId)
$null = Get-Command claude -ErrorAction Stop
$run = (New-Item -ItemType Directory -Force -Path $RunDir).FullName
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss-fff'
$prefix = Join-Path $run "$stamp-$SessionId"
$cliArgs = @('--model', 'fable', '--print', '--dangerously-skip-permissions', '--output-format', 'json')
if ($Resume) { $cliArgs += @('--resume', $SessionId) }
else { $cliArgs += @('--session-id', $SessionId) }
$cliArgs += "Read the review instructions in $promptFile and carry out that bounded review."
Write-Output "Fable session: $SessionId"
Write-Output "Output prefix: $prefix"
& claude @cliArgs 1> "$prefix.json" 2> "$prefix-stderr.txt"
$code = $LASTEXITCODE
if ($code -ne 0) { throw "Fable exited $code; inspect $prefix-stderr.txt and $prefix.json" }
$events = Get-Content -LiteralPath "$prefix.json" -Raw | ConvertFrom-Json
# Verbose CLI configurations return an event array; plain configurations return
# the result object alone. Select the terminal result explicitly in both cases.
$result = @($events) | Where-Object { $_.type -eq 'result' } | Select-Object -Last 1
if ($null -eq $result) { throw "Fable returned no terminal result; inspect $prefix.json" }
if ($result.is_error) { throw "Fable reported an error; inspect $prefix.json" }
Write-Output $result.result
Write-Output "Fable completed: $SessionId"
