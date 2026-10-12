param([Parameter(Mandatory = $true)][string]$Launcher)

$ErrorActionPreference = 'Stop'
$action = New-ScheduledTaskAction -Execute "$env:SystemRoot\System32\cmd.exe" -Argument ('/d /s /c ""{0}""' -f $Launcher)
$trigger = New-ScheduledTaskTrigger -AtStartup
$principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
$settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit ([TimeSpan]::Zero) -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
Register-ScheduledTask -TaskName 'Portico Media Server' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null
