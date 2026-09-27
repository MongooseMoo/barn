$ErrorActionPreference = 'Stop'
$root = (Get-Location).Path
$env:GOMAXPROCS = '4'
$env:BARN_MONGOOSE_BENCH = '1'
$env:BARN_MONGOOSE_DB = Join-Path $root '.tmp/mongoose.db'
$env:BARN_MONGOOSE_PLAYERS = '1'
$env:BARN_MONGOOSE_WARMUP = '1s'
$env:BARN_MONGOOSE_MEASURE = '5s'
$env:BARN_MONGOOSE_ONLY = ''
$env:BARN_MONGOOSE_REPAIR = '1'
$env:BARN_MONGOOSE_PROMOTE = '1'
foreach ($case in @('candidate-mem','baseline-cpu','candidate-cpu')) {
    $side, $kind = $case.Split('-')
    $runRoot = Join-Path $root ".tmp/profile-$case"
    New-Item -ItemType Directory -Path (Join-Path $runRoot 'engine') | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $runRoot 'files/sqlite') -Force | Out-Null
    $profile = Join-Path $root ".tmp/mongoose-$case.pprof"
    $env:BARN_MONGOOSE_MEMPROFILE = if ($kind -eq 'mem') { $profile } else { '' }
    $env:BARN_MONGOOSE_CPUPROFILE = if ($kind -eq 'cpu') { $profile } else { '' }
    $binary = Join-Path $root ".tmp/engine-$side.exe"
    $log = Join-Path $root ".tmp/mongoose-$case.txt"
    Push-Location (Join-Path $runRoot 'engine')
    try {
        & $binary '-test.run=^TestMongooseRealWorkload$' '-test.v' '-test.timeout=180s' *> $log
        $runExit = $LASTEXITCODE
    } finally { Pop-Location }
    "case=$case exit=$runExit" | Write-Output
    if ($runExit -ne 0) { throw "Profile failed: $log" }
}
