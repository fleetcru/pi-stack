# Legacy filename retained. Install a current-user logon task, not an SCM service.
[CmdletBinding(SupportsShouldProcess = $true)]
param(
  [string]$Binary = "$PSScriptRoot\..\pi-server.exe",
  [ValidatePattern('^[A-Za-z0-9_.-]+$')][string]$Name = 'pi-server',
  [ValidatePattern('^(\[[0-9a-fA-F:]+\]|[A-Za-z0-9_.-]+):[0-9]{1,5}$')][string]$Addr = '127.0.0.1:3142'
)
$ErrorActionPreference = 'Stop'
$port = [int]($Addr -replace '^.*:', '')
if ($port -lt 1 -or $port -gt 65535) { throw 'Port must be between 1 and 65535.' }
$Binary = [System.IO.Path]::GetFullPath($Binary)
if (-not (Test-Path -LiteralPath $Binary -PathType Leaf)) {
  throw "pi-server binary not found: $Binary. Run scripts/build.sh or pass -Binary explicitly."
}
# pi-server is a console executable, not a Service Control Manager service.
# sc.exe create produces a service that cannot start. Use Task Scheduler.
if (Get-Service -Name $Name -ErrorAction SilentlyContinue) {
  throw "A Windows service named $Name already exists. Stop and remove that legacy service explicitly before installing a task."
}
$existing = Get-ScheduledTask -TaskName $Name -ErrorAction SilentlyContinue
if ($existing -and $existing.State -eq 'Running') { throw "Stop the existing task $Name before changing its definition." }
if ($existing -and -not @($existing.Actions | Where-Object { [string]::Equals($_.Execute, $Binary, [StringComparison]::OrdinalIgnoreCase) }).Count) {
  throw "Task $Name belongs to another executable. Choose a different -Name."
}
$pi = Get-Command pi -CommandType Application -ErrorAction Stop
$user = [Security.Principal.WindowsIdentity]::GetCurrent().Name
if ($PSCmdlet.ShouldProcess($Name, 'Install a current-user logon task for the existing binary')) {
  $action = New-ScheduledTaskAction -Execute $Binary -Argument "--addr $Addr --pi `"$($pi.Source)`" --pairing-qr=false" -WorkingDirectory (Split-Path $Binary)
  $trigger = New-ScheduledTaskTrigger -AtLogOn -User $user
  $settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew
  $principal = New-ScheduledTaskPrincipal -UserId $user -LogonType Interactive -RunLevel Limited
  Register-ScheduledTask -TaskName $Name -Action $action -Trigger $trigger -Settings $settings -Principal $principal -Force | Out-Null
  Write-Host "Installed logon task $Name for $user. Start with: Start-ScheduledTask -TaskName '$Name'"
  Write-Host 'For downloads, verified upgrades, and a managed configuration, use the root install-server-user.ps1 installer.'
}
