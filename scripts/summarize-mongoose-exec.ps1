[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$LogPath,
    [ValidateRange(1, 10000)][int]$Last = 20
)
$ErrorActionPreference = 'Stop'
$pending = @{}
$pairs = foreach ($line in Get-Content -LiteralPath $LogPath) {
    try { $row = $line | ConvertFrom-Json } catch { continue } # Live final line may be incomplete.
    if ($row.msg -eq 'external command completed') {
        $id = [string]$row.task_id
        if (-not $pending.ContainsKey($id)) { $pending[$id] = [Collections.Generic.Queue[object]]::new() }
        $pending[$id].Enqueue($row)
    } elseif ($row.msg -eq 'external task resumed') {
        $id = [string]$row.task_id
        if (-not $pending.ContainsKey($id) -or $pending[$id].Count -eq 0) { continue }
        $completion = $pending[$id].Dequeue()
        [pscustomobject]@{
            task_id = $row.task_id
            command = $completion.command
            completed = $completion.time
            command_ms = $completion.elapsed / 1e6
            queue_wait_ms = $row.queue_wait / 1e6
            ready_to_vm_ms = $row.ready_to_vm / 1e6
        }
    }
}
ConvertTo-Json -InputObject @($pairs | Select-Object -Last $Last)
