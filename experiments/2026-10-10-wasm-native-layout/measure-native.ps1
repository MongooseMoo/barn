param(
    [string]$Label = 'three-way',
    [string[]]$Versions = @('baseline', 'refactor', 'current'),
    [int]$Rounds = 10,
    [int]$CPU = 1,
    [long]$Affinity = 16
)
$ErrorActionPreference = 'Stop'
$toolsDirectory = $PSScriptRoot
foreach ($version in $Versions) {
    $binary = Join-Path $toolsDirectory "builtins-$version.test.exe"
    if (!(Test-Path -LiteralPath $binary -PathType Leaf)) { throw "Missing $binary" }
    [IO.File]::WriteAllText((Join-Path $toolsDirectory "$Label-$version.txt"), '')
}
for ($round = 0; $round -lt $Rounds; $round++) {
    $order = @($Versions)
    if ($round % 2) { [Array]::Reverse($order) }
    foreach ($version in $order) {
        $start = [Diagnostics.ProcessStartInfo]::new()
        $start.FileName = Join-Path $toolsDirectory "builtins-$version.test.exe"
        $start.Arguments = "-test.run=^$ -test.bench=^BenchmarkBuiltinAdmission$ -test.benchmem -test.benchtime=1s -test.count=1 -test.cpu=$CPU"
        $start.UseShellExecute = $false
        $start.CreateNoWindow = $true
        $start.RedirectStandardOutput = $true
        $start.RedirectStandardError = $true
        $start.Environment['GOMAXPROCS'] = "$CPU"
        $start.Environment['GOGC'] = '100'
        $start.Environment['GOMEMLIMIT'] = 'off'
        $process = [Diagnostics.Process]::Start($start)
        try {
            if ($Affinity) { $process.ProcessorAffinity = [IntPtr]$Affinity }
            $output = $process.StandardOutput.ReadToEnd()
            $errors = $process.StandardError.ReadToEnd()
            $process.WaitForExit()
            if ($process.ExitCode -ne 0) { throw "$version exited $($process.ExitCode): $errors $output" }
            [IO.File]::AppendAllText((Join-Path $toolsDirectory "$Label-$version.txt"), $output)
            Write-Output "round=$($round+1) version=$version affinity=$Affinity cpu=$CPU"
        } finally { $process.Dispose() }
    }
}
