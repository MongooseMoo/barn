[CmdletBinding()]
param(
    [int[]]$Limits = @(1,16),
    [switch]$FullConformance,
    [switch]$SkipGo,
    [switch]$SkipBuild,
	[int]$Port = 11495,
    [string]$RunDir = '.tmp/shared-admission'
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
Set-Location (Split-Path $PSScriptRoot -Parent)
$run = (New-Item -ItemType Directory -Force -Path $RunDir).FullName
$binary = Join-Path $run 'barn.exe'
if (-not $SkipGo) {
    & go test ./... -timeout 180s
    $code = $LASTEXITCODE
    if ($code -ne 0) { throw "Go tests failed: $code" }
    & go test -race ./internal/admission ./internal/commitgate ./engine ./server ./task ./db/store -timeout 300s
    $code = $LASTEXITCODE
    if ($code -ne 0) { throw "Race tests failed: $code" }
}
if (-not $SkipBuild) {
    $main = & go list -f '{{.ImportPath}} {{.Name}}' ./cmd/barn
    $code = $LASTEXITCODE
    if ($code -ne 0 -or $main -ne 'github.com/MongooseMoo/barn/cmd/barn main') { throw "Unexpected main package: $main" }
    & go build -o $binary ./cmd/barn
    $code = $LASTEXITCODE
    if ($code -ne 0) { throw "Build failed: $code" }
}
$selectors = @()
if (-not $FullConformance) {
    $selectors = @('server/dump_database.yaml','server/exec_recent_regressions.yaml','audit/task_scheduling_toast_oracle.yaml','generated_builtins/force_input.yaml') | ForEach-Object { "--moo-suite-path=$_" }
}
$failures = @()
foreach ($limit in $Limits) {
    if ($limit -lt 1 -or $limit -gt 1000000) { throw "Invalid admission limit: $limit" }
    $configuration = Join-Path $run "admission-$limit.conf"
    Set-Content -LiteralPath $configuration -Value "ADMISSION_LIMIT = $limit" -Encoding ascii
    & "$PSScriptRoot/run-conformance.ps1" -Binary $binary -SourceDb Test_conf.db -RunDb (Join-Path $run "admission-$limit.db") -Port $Port -ReportsRoot (Join-Path $run "conformance-$limit") -ExtraServerArgs @('-config', ('"' + $configuration + '"')) -ExtraPytestArgs $selectors
    $code = $LASTEXITCODE
    if ($code -ne 0) { $failures += $limit }
}
if ($failures.Count) { throw "Conformance failed for admission limits: $($failures -join ', ')" }
