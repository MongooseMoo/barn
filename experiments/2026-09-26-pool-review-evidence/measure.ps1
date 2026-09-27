$ErrorActionPreference = 'Stop'
$outputDir = $PSScriptRoot
$jobs = @(
    @{Name='unique'; Base='C:/Users/Q/code/barn-pool-evidence/unique-base.exe'; Candidate=(Join-Path $outputDir 'builtins.exe'); Bench='^BenchmarkPoolUniqueHoldout$'; CPU='4'},
    @{Name='fileio'; Base='C:/Users/Q/code/barn-pool-evidence/fileio-base.exe'; Candidate=(Join-Path $outputDir 'builtins.exe'); Bench='^BenchmarkPoolFileReadHoldout$'; CPU='4'},
    @{Name='finalization'; Base='C:/Users/Q/code/barn-pool-finalization/.tmp/finalization-baseline.test.exe'; Candidate=(Join-Path $outputDir 'vm.exe'); Bench='^(BenchmarkPlainFrameFinalization|BenchmarkTemporaryFrameLifecycle)$'; CPU='32'}
)
foreach ($job in $jobs) {
    for ($pair=0; $pair -lt 10; $pair++) {
        $sides = if ($pair % 2 -eq 0) { @('base','candidate') } else { @('candidate','base') }
        foreach ($side in $sides) {
            $binary = if ($side -eq 'base') { $job.Base } else { $job.Candidate }
            $path = Join-Path $outputDir ($job.Name + '-' + $side + '-' + $pair + '.txt')
            if (Test-Path -LiteralPath $path) { throw "Refusing overwrite $path" }
            $arguments = @('-test.run=^$', ('-test.bench='+$job.Bench), '-test.benchtime=300ms', '-test.count=1', '-test.benchmem', '-test.timeout=60s', ('-test.cpu='+$job.CPU))
            $output = & $binary @arguments 2>&1
            $code = $LASTEXITCODE
            $output | Out-File -LiteralPath $path -Encoding utf8
            Write-Output ($job.Name + ' pair=' + $pair + ' side=' + $side + ' exit=' + $code)
            if ($code -ne 0) { throw "Holdout invocation failed: $path" }
        }
    }
}
