$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
$baselineOutput = Join-Path $root 'baseline-paired.txt'
$candidateOutput = Join-Path $root 'candidate-paired.txt'
if ((Test-Path -LiteralPath $baselineOutput) -or (Test-Path -LiteralPath $candidateOutput)) { throw 'Refusing to overwrite paired observations' }
for ($pair = 0; $pair -lt 10; $pair++) {
    $sides = if ($pair % 2 -eq 0) { @('baseline', 'candidate') } else { @('candidate', 'baseline') }
    foreach ($side in $sides) {
        $binary = Join-Path $root "../../.tmp/finalization-$side.test.exe"
        $output = & $binary '-test.run=^$' '-test.bench=^BenchmarkPendingWaifLocals$' '-test.benchtime=1s' '-test.benchmem' '-test.timeout=30s' 2>&1
        $code = $LASTEXITCODE
        $output | Out-File -LiteralPath (Join-Path $root "$side-paired.txt") -Append -Encoding utf8
        Write-Output "pair=$pair side=$side exit=$code"
        if ($code -ne 0) { throw "measurement failed: $side" }
    }
}
