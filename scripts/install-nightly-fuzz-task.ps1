# Run on the workstation that owns the Barn self-hosted runners, as Q.
$ErrorActionPreference = 'Stop'
if ((Get-TimeZone).Id -ne 'Mountain Standard Time') {
    throw 'The Barn nightly task requires the workstation to use Mountain time.'
}
$ghPath = (Get-Command gh -CommandType Application).Source
$pwshPath = (Get-Command pwsh -CommandType Application).Source
$taskName = 'Barn - Nightly fuzz'
$arguments = '-NoProfile -NonInteractive -WindowStyle Hidden -Command "& ''{0}'' workflow run fuzz.yml --repo MongooseMoo/barn --ref master; exit $LASTEXITCODE"' -f $ghPath.Replace("'", "''")
$action = New-ScheduledTaskAction -Execute $pwshPath -Argument $arguments
$trigger = New-ScheduledTaskTrigger -Daily -At '03:17'
$trigger.StartBoundary = (Get-Date -Hour 3 -Minute 17 -Second 0).ToString("yyyy-MM-dd'T'HH:mm:ss")
$principal = New-ScheduledTaskPrincipal -UserId ([System.Security.Principal.WindowsIdentity]::GetCurrent().Name) -LogonType Interactive
$settings = New-ScheduledTaskSettingsSet -WakeToRun -MultipleInstances IgnoreNew -ExecutionTimeLimit (New-TimeSpan -Minutes 5)
$settings.StartWhenAvailable = $false
Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Description 'Dispatch Barn nightly fuzz at 03:17 Mountain; do not catch up missed nights during the day.' -Force
Get-ScheduledTaskInfo -TaskName $taskName
