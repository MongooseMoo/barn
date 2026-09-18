[CmdletBinding()]
param(
    [ValidateSet('Barn','Toast')][string]$Engine = 'Barn',
    [string]$RunDir = '.tmp/mongoose-fable-20260918',
    [int]$Port = 7777,
    [Parameter(Mandatory)][string]$Username,
    [Parameter(Mandatory)][string]$Password
)
# Keep the statements inside eval: the account's one-semicolon command can
# otherwise return only the first expression instead of executing this loop.
$source = 'r={}; for o in (#4143.objects[1..3]) t=ticks_left(); f=ftime(); o:cycle(); r={@r,{o,t-ticks_left(),ftime()-f}}; endfor return {"cycle-cost",r};'
$command = ';return eval(' + (ConvertTo-Json -InputObject $source -Compress) + ');'
& "$PSScriptRoot/mongoose-login.ps1" -Engine $Engine -RunDir $RunDir -Port $Port -Proxy -Username $Username -Password $Password -Commands $command -InterCommand 12000 -Timeout 35 -MaxDuration 85 | Tee-Object -Variable probeOutput
if (-not ($probeOutput -match '=> \{1, \{"cycle-cost",')) {
    throw 'Cycle-cost eval did not complete successfully; inspect the probe transcript.'
}
