param([string]$Name,[string]$Bench)
$ErrorActionPreference='Stop'
$env:GOMAXPROCS='4'
for($i=0;$i -lt 10;$i++) {
 $order=@('base','candidate')
 if($i%2){$order=@('candidate','base')}
 foreach($side in $order){
  $exe=Join-Path $PSScriptRoot "$Name-$side.exe"
  $output=& $exe '-test.run=^$' "-test.bench=^$Bench`$" '-test.benchtime=300ms' '-test.count=1' '-test.benchmem'
  $runExit=$LASTEXITCODE
  $output | Set-Content -Encoding utf8 (Join-Path $PSScriptRoot "$Name-$side-$i.txt")
  if($runExit -ne 0){throw "$Name $side pair $i failed: $runExit"}
 }
 Write-Output "Completed $Name pair $i"
}
