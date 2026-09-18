<#
.SYNOPSIS
Remove or archive the untracked scratch that accumulates at the repository root.

.DESCRIPTION
Dry run by default: prints what it would do and the bytes involved. Nothing is
touched unless -Force (delete scratch) or -ArchiveNotes (move notes-*.md into
notes/archive/<year>/) is given. Only UNTRACKED files are ever considered; the
script refuses to touch anything `git ls-files` knows about, and keeps the
untracked Mongoose fixtures (mongoose.db, mongoose.db.new) the benchmarks read.

Scratch classes handled:
  - root database copies (*.db, *.db.new, *.waifids) except tracked fixtures
  - transcripts and probes (test_*, tmp_*, toast_*, server_*, *_results.txt,
    *.err, *.out, *.log, *.cpu, *.mem, *.stackdump, hs_err_pid*.log)
  - stray editor/OS files (nul.#N#, NUL.#N#, *.exe~, *~)
  - untracked binaries (*.exe, *.dll not tracked)
  - agent notes (notes-*.md) with -ArchiveNotes

.EXAMPLE
.\scripts\clean-scratch.ps1                 # report only
.\scripts\clean-scratch.ps1 -Force          # delete scratch
.\scripts\clean-scratch.ps1 -ArchiveNotes   # move notes-*.md into notes/archive/2026/
#>
[CmdletBinding()]
param(
    [switch]$Force,
    [switch]$ArchiveNotes
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $repoRoot

$tracked = @{}
foreach ($line in (& git ls-files)) {
    if ($line -and -not $line.Contains("/")) { $tracked[$line] = $true }
}

$scratchPatterns = @(
    "*.db", "*.db.new", "*.waifids",
    "test_*", "tmp_*", "toast_*", "server_*", "*_results.txt",
    "*.err", "*.out", "*.log", "*.cpu", "*.mem", "*.stackdump", "hs_err_pid*.log",
    "nul.#*", "NUL.#*", "*.exe~", "*~", "*.exe", "*.dll",
    "hg.html", "main.py", "compare.sh", "stateful_compare.sh", "oracle.sh",
    "*_divergence_test*", "*.moo", "login_test.txt", "manual_*", "conformance_results.txt"
)

# Untracked but load-bearing: the real-workload harness
# (engine/mongoose_real_bench_test.go) and bench_differ read these by default.
$keep = @{ "mongoose.db" = $true; "mongoose.db.new" = $true }

$rootFiles = Get-ChildItem -LiteralPath $repoRoot -File -Force
$scratch = @()
$notes = @()
foreach ($f in $rootFiles) {
    if ($tracked.ContainsKey($f.Name) -or $keep.ContainsKey($f.Name)) { continue }
    if ($f.Name -like "notes-*.md") { $notes += $f; continue }
    foreach ($p in $scratchPatterns) {
        if ($f.Name -like $p) { $scratch += $f; break }
    }
}

$scratchBytes = ($scratch | Measure-Object -Property Length -Sum).Sum
Write-Host ("Untracked scratch: {0} files, {1:N1} MB" -f $scratch.Count, ($scratchBytes / 1MB))
foreach ($f in ($scratch | Sort-Object Length -Descending | Select-Object -First 15)) {
    Write-Host ("  {0,10:N1} MB  {1}" -f ($f.Length / 1MB), $f.Name)
}
if ($scratch.Count -gt 15) { Write-Host ("  ... and {0} more" -f ($scratch.Count - 15)) }
Write-Host ("Untracked agent notes: {0} files" -f $notes.Count)

if ($Force) {
    foreach ($f in $scratch) { Remove-Item -LiteralPath $f.FullName -Force -Confirm:$false }
    Write-Host ("Deleted {0} scratch files." -f $scratch.Count)
} else {
    Write-Host "Dry run: pass -Force to delete the scratch files above."
}

if ($ArchiveNotes -and $notes.Count -gt 0) {
    $year = (Get-Date).Year
    $archive = Join-Path $repoRoot ("notes/archive/{0}" -f $year)
    New-Item -ItemType Directory -Force -Path $archive | Out-Null
    foreach ($f in $notes) {
        Move-Item -LiteralPath $f.FullName -Destination (Join-Path $archive $f.Name) -Force
    }
    Write-Host ("Archived {0} notes into {1}." -f $notes.Count, $archive)
} elseif ($notes.Count -gt 0) {
    Write-Host "Pass -ArchiveNotes to move notes-*.md into notes/archive/<year>/."
}
