param(
    [Parameter(Mandatory)][string]$Baseline,
    [Parameter(Mandatory)][string]$Candidate,
    [Parameter(Mandatory)][string]$Database,
    [Parameter(Mandatory)][string]$OutputDirectory
)
$ErrorActionPreference = 'Stop'
$Baseline = (Resolve-Path -LiteralPath $Baseline).Path
$Candidate = (Resolve-Path -LiteralPath $Candidate).Path
$Database = (Resolve-Path -LiteralPath $Database).Path
if (Test-Path -LiteralPath $OutputDirectory) { throw 'Use a fresh output directory.' }
$outputRoot = (New-Item -ItemType Directory -Path $OutputDirectory).FullName
$env:GOMAXPROCS = '4'
$env:BARN_MONGOOSE_BENCH = '1'
$env:BARN_MONGOOSE_DB = $Database
$env:BARN_MONGOOSE_PLAYERS = '16'
$env:BARN_MONGOOSE_WARMUP = '2s'
$env:BARN_MONGOOSE_MEASURE = '15s'
$env:BARN_MONGOOSE_MEMPROFILE = ''
$env:BARN_MONGOOSE_CPUPROFILE = ''
$env:BARN_MONGOOSE_ONLY = ''
$env:BARN_MONGOOSE_REPAIR = '1'
$env:BARN_MONGOOSE_PROMOTE = '1'
for ($pair = 0; $pair -lt 10; $pair++) {
    $order = if ($pair % 2 -eq 0) { @('baseline', 'candidate') } else { @('candidate', 'baseline') }
    foreach ($side in $order) {
        $runRoot = Join-Path $outputRoot "$pair-$side"
        New-Item -ItemType Directory -Path (Join-Path $runRoot 'engine') | Out-Null
        New-Item -ItemType Directory -Path (Join-Path $runRoot 'files/sqlite') -Force | Out-Null
        $binary = if ($side -eq 'baseline') { $Baseline } else { $Candidate }
        $log = Join-Path $outputRoot "$pair-$side.txt"
        Push-Location (Join-Path $runRoot 'engine')
        try {
            & $binary '-test.run=^TestMongooseRealWorkload$' '-test.v' '-test.timeout=180s' *> $log
            $runExit = $LASTEXITCODE
        } finally { Pop-Location }
        "exit=$runExit" | Add-Content -LiteralPath $log
        Write-Output "pair=$pair side=$side exit=$runExit"
        if ($runExit -ne 0) { throw "$side pair $pair failed; see $log" }
        $rows = Select-String -LiteralPath $log -Pattern 'players=16 goodput='
        if ($rows.Count -ne 1) { throw "Missing measurement row: $log" }
        $rows.Line | Write-Output
    }
}
