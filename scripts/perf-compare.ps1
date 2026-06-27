<#
.SYNOPSIS
    Compare a VM micro-benchmark run against the locked C0 baseline using benchstat.

.DESCRIPTION
    Runs `go test ./vm -run='^$' -bench=<Filter> -benchmem -count=10` into a temp
    file, then runs benchstat against the canonical baseline so you can see the
    ns/op | B/op | allocs/op delta WITH statistical significance (benchstat prints
    "~" when a change is indistinguishable from noise at p < 0.05).

    The baseline file (experiments/perf-baseline-vm-20260627.txt) already contains
    ALL nine workloads at -count=10. benchstat matches rows by benchmark name, so a
    FILTERED new run (e.g. just BenchmarkVM/int_arith_1M) still compares correctly:
    benchstat simply reports the one row both files share and ignores the rest.

.PARAMETER Filter
    Benchmark filter passed to `-bench`, e.g. 'BenchmarkVM/int_arith_1M' or
    'BenchmarkVM' to re-run everything.

.PARAMETER Label
    Short tag for this run, used in the temp output filename, e.g. 'c1-after'.

.EXAMPLE
    pwsh scripts/perf-compare.ps1 BenchmarkVM/int_arith_1M c1-after

.EXAMPLE
    pwsh scripts/perf-compare.ps1 BenchmarkVM full-rerun
#>
param(
    [Parameter(Mandatory = $true, Position = 0)]
    [string]$Filter,

    [Parameter(Mandatory = $true, Position = 1)]
    [string]$Label
)

$ErrorActionPreference = 'Stop'

# Repo root = parent of this script's directory.
$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

$baseline = Join-Path $repoRoot 'experiments/perf-baseline-vm-20260627.txt'
if (-not (Test-Path $baseline)) {
    throw "Baseline file not found: $baseline (run the C0 baseline capture first)"
}

# Locate benchstat (installed via `go install golang.org/x/perf/cmd/benchstat@latest`).
$benchstat = (Get-Command benchstat -ErrorAction SilentlyContinue)?.Source
if (-not $benchstat) {
    $gopath = (& go env GOPATH).Trim()
    $candidate = Join-Path $gopath 'bin/benchstat.exe'
    if (Test-Path $candidate) {
        $benchstat = $candidate
    } else {
        throw "benchstat not on PATH and not at $candidate. Run: go install golang.org/x/perf/cmd/benchstat@latest"
    }
}

$newFile = Join-Path $repoRoot ("experiments/perf-{0}-vm.txt" -f $Label)

Write-Host "==> Running: go test ./vm -run='^$' -bench=$Filter -benchmem -count=10"
Write-Host "==> Output:  $newFile"
& go test ./vm "-run=^$" "-bench=$Filter" -benchmem -count=10 | Tee-Object -FilePath $newFile
if ($LASTEXITCODE -ne 0) {
    throw "go test failed with exit code $LASTEXITCODE"
}

Write-Host ""
Write-Host "==> benchstat baseline vs $Label"
& $benchstat $baseline $newFile
