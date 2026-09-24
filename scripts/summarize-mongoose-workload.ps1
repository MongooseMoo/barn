[CmdletBinding()]
param(
    [string]$LogPath = '.tmp/mongoose-fable-20260918/barn/logs/latest.jsonl',
    [int]$LastSeconds = 600
)
$ErrorActionPreference = 'Stop'
$cutoff = [DateTimeOffset]::Now.AddSeconds(-$LastSeconds)
$rows = @(Get-Content -LiteralPath $LogPath | ForEach-Object {
    try { $row = $_ | ConvertFrom-Json } catch { return } # A live final line may be incomplete.
    if ([DateTimeOffset]$row.time -ge $cutoff) { $row }
})
if ($rows.Count -lt 2) { throw 'Need at least two log records in the requested window.' }
$seconds = ([DateTimeOffset]$rows[-1].time - [DateTimeOffset]$rows[0].time).TotalSeconds
if ($seconds -le 0) { throw 'Log window has no elapsed time.' }
$slices = @($rows | Where-Object { $_.msg -eq 'slow task slice' -and $_.verb -eq 's_run' })
$durations = @($slices | ForEach-Object { $_.elapsed / 1e9 } | Sort-Object)
$resets = @($rows | Where-Object { $_.msg -eq 'irreversible-effect boundary' -and $_.verb -eq 'schedule' }).Count
[ordered]@{
    window_seconds = $seconds
    slow_slices = $slices.Count
    slow_slice_duty = if ($durations.Count) { ($durations | Measure-Object -Sum).Sum / $seconds } else { 0 }
    median_seconds = if ($durations.Count) { $durations[[int][Math]::Floor($durations.Count / 2)] } else { 0 }
    max_seconds = if ($durations.Count) { $durations[-1] } else { 0 }
    schedule_boundaries_per_second = $resets / $seconds
} | ConvertTo-Json
