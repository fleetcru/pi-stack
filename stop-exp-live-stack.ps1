[CmdletBinding(SupportsShouldProcess = $true)]
param([string]$DataDir = (Join-Path $PSScriptRoot 'pi-server-exp\.data\pi-server'))
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'dev-launcher-common.ps1')
$DataDir = [IO.Path]::GetFullPath($DataDir)
$stateFile = Join-Path $DataDir 'dev-stack.json'
if (-not (Test-Path -LiteralPath $DataDir)) { Write-Host 'No recorded development stack.'; return }
$lock = [IO.File]::Open((Join-Path $DataDir 'dev-stack.lock'), [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
try {
  if (-not (Test-Path -LiteralPath $stateFile)) { Write-Host 'No recorded development stack.'; return }
  $state = [IO.File]::ReadAllText($stateFile) | ConvertFrom-Json
  if ($state.buildId -notmatch '^[0-9a-f]{32}$') { throw 'Invalid stack record. No processes or files were changed.' }
  if ($PSCmdlet.ShouldProcess($DataDir, 'Stop the recorded development stack')) {
    foreach ($entry in $state.processes) {
      $process = Get-Process -Id $entry.id -ErrorAction SilentlyContinue
      # Never kill a new process that has reused a stale recorded PID.
      if (Test-DevProcessRecord -Process $process -Record $entry) {
        Stop-DevProcessTree -Process $process
        if (-not $process.WaitForExit(10000)) { throw "Process $($process.Id) did not stop. Stack record retained." }
      }
    }
    $binary = Join-Path (Join-Path (Join-Path $DataDir '.launchers') $state.buildId) 'pi-server.exe'
    Remove-Item -LiteralPath $binary -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $stateFile -Force
    Write-Host 'Development stack stopped. Startup logs were preserved.'
  }
} finally { $lock.Dispose() }
