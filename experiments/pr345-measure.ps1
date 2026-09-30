param(
    [Parameter(Mandatory)][string]$Control,
    [Parameter(Mandatory)][string]$Candidate,
    [Parameter(Mandatory)][string]$Callbacks,
    [Parameter(Mandatory)][string]$Checkpoint,
    [Parameter(Mandatory)][string]$Database,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [switch]$ApplicationOnly,
    [ValidateRange(1,100)][int]$Pairs = 5
)
$ErrorActionPreference = 'Stop'
$Control = (Resolve-Path -LiteralPath $Control).Path
$Candidate = (Resolve-Path -LiteralPath $Candidate).Path
$Callbacks = (Resolve-Path -LiteralPath $Callbacks).Path
$Checkpoint = (Resolve-Path -LiteralPath $Checkpoint).Path
$Database = (Resolve-Path -LiteralPath $Database).Path
if (Test-Path -LiteralPath $OutputDirectory) { throw 'Use a fresh output directory.' }
$outputRoot = (New-Item -ItemType Directory -Path $OutputDirectory).FullName
$benchmarkMutex = [Threading.Mutex]::new($false, 'Global\BarnBenchmark')
if (!$benchmarkMutex.WaitOne(1000)) { $benchmarkMutex.Dispose(); throw 'Barn benchmark mutex is held.' }
try {
    $env:GOMAXPROCS = '4'
    $env:BARN_MONGOOSE_BENCH = ''
    if (!$ApplicationOnly) { foreach ($sample in 0..9) {
        & $Callbacks '-test.run=^$' '-test.bench=Callbacks' '-test.benchmem' '-test.benchtime=100ms' '-test.count=1' *> (Join-Path $outputRoot "callbacks-$sample.txt")
        $runExit = $LASTEXITCODE
        if ($runExit -ne 0) { throw "Callback sample $sample failed." }
        $order = if ($sample % 2 -eq 0) { @('control','candidate') } else { @('candidate','control') }
        foreach ($side in $order) {
            $binary = if ($side -eq 'control') { $Control } else { $Candidate }
            & $binary '-test.run=^$' '-test.bench=^BenchmarkThreadedContinuations$' '-test.benchmem' '-test.benchtime=200ms' '-test.count=1' *> (Join-Path $outputRoot "continuations-$sample-$side.txt")
            $runExit = $LASTEXITCODE
            if ($runExit -ne 0) { throw "Continuation sample $sample $side failed." }
        }
        Write-Output "callback and continuation sample=$sample complete"
    }
    & $Checkpoint '-test.run=^$' '-test.bench=^BenchmarkContinuationCheckpoint$' '-test.benchmem' '-test.benchtime=100ms' '-test.count=10' *> (Join-Path $outputRoot 'checkpoint.txt')
    $runExit = $LASTEXITCODE
    if ($runExit -ne 0) { throw 'Checkpoint benchmark failed.' }
    Write-Output 'checkpoint samples complete'
    }

    $env:BARN_MONGOOSE_BENCH = '1'
    $env:BARN_MONGOOSE_DB = $Database
    $env:BARN_MONGOOSE_PLAYERS = '1,4,16'
    $env:BARN_MONGOOSE_WARMUP = '1s'
    $env:BARN_MONGOOSE_MEASURE = '3s'
    $env:BARN_MONGOOSE_MEMPROFILE = ''
    $env:BARN_MONGOOSE_CPUPROFILE = ''
    $env:BARN_MONGOOSE_ONLY = ''
    $env:BARN_MONGOOSE_REPAIR = '1'
    $env:BARN_MONGOOSE_PROMOTE = '1'
    $env:BARN_MONGOOSE_COMPLETION_TIMEOUT = '10s'
    foreach ($pair in 0..($Pairs-1)) {
        $order = if ($pair % 2 -eq 0) { @('control','candidate') } else { @('candidate','control') }
        foreach ($side in $order) {
            $runRoot = Join-Path $outputRoot "mongoose-$pair-$side"
            New-Item -ItemType Directory -Path (Join-Path $runRoot 'engine') | Out-Null
            New-Item -ItemType Directory -Path (Join-Path $runRoot 'files/sqlite') -Force | Out-Null
            $binary = if ($side -eq 'control') { $Control } else { $Candidate }
            $log = Join-Path $outputRoot "mongoose-$pair-$side.txt"
            Push-Location (Join-Path $runRoot 'engine')
            try {
                & $binary '-test.run=^TestMongooseRealWorkload$' '-test.v' '-test.timeout=180s' *> $log
                $runExit = $LASTEXITCODE
            } finally { Pop-Location }
            Write-Output "mongoose pair=$pair side=$side exit=$runExit"
            $rows = Select-String -LiteralPath $log -Pattern 'players=\d+ goodput='
            if ($rows.Count -ne 3) { throw "Mongoose pair $pair $side did not produce its terminal inventory; see $log" }
            (Select-String -LiteralPath $log -Pattern 'cohort submitted=|players=\d+ goodput=').Line | Write-Output
        }
    }
} finally {
    $benchmarkMutex.ReleaseMutex()
    $benchmarkMutex.Dispose()
}
