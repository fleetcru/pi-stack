[CmdletBinding()]
param(
  [ValidateRange(1, 65535)][int]$ServerPort = 3142,
  [ValidateRange(1, 65535)][int]$WebPort = 5174,
  [string]$AuthToken = $env:PI_SERVER_AUTH_TOKEN,
  [switch]$InstallGlobalBridge,
  [switch]$Background,
  [switch]$Minimized,
  [bool]$LaunchPi = $true,
  [ValidateRange(0, 100000)][int]$MaxSessions = 16,
  [ValidateRange(0, 100000)][int]$MaxActiveRuns = 8,
  [ValidateRange(1, 600)][int]$HealthTimeoutSec = 30,
  [string]$DataDir = (Join-Path $PSScriptRoot 'pi-server-exp\.data\pi-server')
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'dev-launcher-common.ps1')
$root = $PSScriptRoot
$serverDir = Join-Path $root 'pi-server-exp'
$webDir = Join-Path $root 'pi-webby-exp'
$bridge = Join-Path $serverDir 'extensions\external-session-bridge.ts'
$DataDir = [IO.Path]::GetFullPath($DataDir)
$stateFile = Join-Path $DataDir 'dev-stack.json'
if ($ServerPort -eq $WebPort) { throw 'ServerPort and WebPort must differ.' }
if ($Background) {
  if ($PSBoundParameters.ContainsKey('LaunchPi') -and $LaunchPi) { throw 'A Pi TUI cannot run in a hidden window. Omit -LaunchPi or use -LaunchPi:$false.' }
  $LaunchPi = $false
}
foreach ($path in @($serverDir, $webDir, $bridge)) {
  if (-not (Test-Path -LiteralPath $path)) { throw "Required path is missing: $path" }
}
$commands = @('go', 'pnpm')
if ($LaunchPi) { $commands += 'pi' }
foreach ($command in $commands) {
  if (-not (Get-Command $command -ErrorAction SilentlyContinue)) { throw "$command is not available in PATH." }
}
if (-not (Test-Path -LiteralPath (Join-Path $webDir 'node_modules'))) { throw 'Webby dependencies are missing. Run pnpm install in pi-webby-exp first.' }
Assert-DevPortAvailable -Port $ServerPort
Assert-DevPortAvailable -Port $WebPort
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
$lock = [IO.File]::Open((Join-Path $DataDir 'dev-stack.lock'), [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
$processes = @()
$buildDir = $null
$savedEnvironment = @{}
foreach ($name in @('PI_SERVER_AUTH_TOKEN', 'PI_EXTERNAL_RELAY_URL', 'PI_EXTERNAL_RELAY_TOKEN')) {
  $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
try {
  if (Test-Path -LiteralPath $stateFile) {
    $previous = [IO.File]::ReadAllText($stateFile) | ConvertFrom-Json
    foreach ($entry in $previous.processes) {
      $existing = Get-Process -Id $entry.id -ErrorAction SilentlyContinue
      if (Test-DevProcessRecord -Process $existing -Record $entry) {
        throw "A stack is already running for this data directory. Run stop-exp-live-stack.ps1 -DataDir $(Quote-DevPowerShell $DataDir)."
      }
    }
  }
  $buildId = [guid]::NewGuid().ToString('N')
  $buildDir = Join-Path (Join-Path $DataDir '.launchers') $buildId
  New-Item -ItemType Directory -Path $buildDir -Force | Out-Null
  $binary = Join-Path $buildDir 'pi-server.exe'
  Build-DevServer -ServerDir $serverDir -Output $binary
  if ($InstallGlobalBridge) {
    & (Join-Path $root 'install-exp-external-bridge.ps1') -ServerPort $ServerPort -RelayUrl "http://127.0.0.1:$ServerPort" -AuthToken $AuthToken
  }
  $windowStyle = if ($Background) { 'Hidden' } elseif ($Minimized) { 'Minimized' } else { 'Normal' }
  # The token travels through the child environment, not its command line.
  [Environment]::SetEnvironmentVariable('PI_SERVER_AUTH_TOKEN', $AuthToken, 'Process')
  $serverCommand = @(
    "`$ErrorActionPreference = 'Stop'"
    "`$env:PI_SERVER_ADDR = '0.0.0.0:$ServerPort'"
    "`$env:PI_SERVER_CWD = $(Quote-DevPowerShell $root)"
    "`$env:PI_SERVER_DATA_DIR = $(Quote-DevPowerShell $DataDir)"
    "if (-not `$env:PI_SERVER_ALLOWED_ROOTS) { `$env:PI_SERVER_ALLOWED_ROOTS = $(Quote-DevPowerShell $root) }"
    "`$env:PI_SERVER_PI_EXTENSIONS = $(Quote-DevPowerShell (Join-Path $serverDir 'extensions\session-title.ts'))"
    "`$env:PI_SERVER_MAX_SESSIONS = '$MaxSessions'"
    "`$env:PI_SERVER_MAX_ACTIVE_RUNS = '$MaxActiveRuns'"
    $(if ($Background) { "& $(Quote-DevPowerShell $binary) --pairing-qr=false *>> $(Quote-DevPowerShell (Join-Path $buildDir 'server-startup.log'))" } else { "& $(Quote-DevPowerShell $binary) 2>> $(Quote-DevPowerShell (Join-Path $buildDir 'server-startup.log'))" })
    'exit $LASTEXITCODE'
  ) -join '; '
  $serverProc = Start-DevPowerShell -Command $serverCommand -WindowStyle $windowStyle
  $processes += $serverProc
  Write-Host 'Waiting for pi-server...' -ForegroundColor Yellow
  Wait-DevHttpReady -Url "http://127.0.0.1:$ServerPort/healthz" -Process $serverProc -TimeoutSec $HealthTimeoutSec

  # Do not give the web dev server the server credential.
  [Environment]::SetEnvironmentVariable('PI_SERVER_AUTH_TOKEN', $null, 'Process')
  $webOutput = if ($Background) { "*>> $(Quote-DevPowerShell (Join-Path $buildDir 'web-startup.log'))" } else { "2>> $(Quote-DevPowerShell (Join-Path $buildDir 'web-startup.log'))" }
  $webCommand = "`$ErrorActionPreference = 'Stop'; Set-Location $(Quote-DevPowerShell $webDir); & pnpm exec vite --host 0.0.0.0 --port $WebPort --strictPort $webOutput; exit `$LASTEXITCODE"
  $webProc = Start-DevPowerShell -Command $webCommand -WindowStyle $windowStyle
  $processes += $webProc
  Wait-DevHttpReady -Url "http://127.0.0.1:$WebPort/" -Process $webProc -TimeoutSec $HealthTimeoutSec
  if ($LaunchPi) {
    [Environment]::SetEnvironmentVariable('PI_EXTERNAL_RELAY_URL', "http://127.0.0.1:$ServerPort", 'Process')
    [Environment]::SetEnvironmentVariable('PI_EXTERNAL_RELAY_TOKEN', $AuthToken, 'Process')
    $installedBridge = Join-Path $HOME '.pi\agent\extensions\external-session-bridge.ts'
    $piLaunch = if (Test-Path -LiteralPath $installedBridge) { '& pi' } else { "& pi -e $(Quote-DevPowerShell $bridge)" }
    $piCommand = "Set-Location $(Quote-DevPowerShell $root); $piLaunch; exit `$LASTEXITCODE"
    $piProc = Start-DevPowerShell -Command $piCommand -WindowStyle $windowStyle
    $processes += $piProc
  }
  $state = @{
    buildId = $buildId
    processes = @(Get-DevProcessRecords -Processes $processes)
  }
  [IO.File]::WriteAllText($stateFile, ($state | ConvertTo-Json -Depth 5), (New-Object Text.UTF8Encoding $false))
  Write-Host 'Stack is ready.' -ForegroundColor Green
  Write-Host "  Webby: http://127.0.0.1:$WebPort"
  Write-Host "  Server: http://127.0.0.1:$ServerPort"
  foreach ($address in @(Get-DevNetworkAddresses)) {
    Write-Host "  Network server: http://${address}:$ServerPort"
    Write-Host "  Network Webby: http://${address}:$WebPort"
  }
  Write-Host "  Startup logs: $buildDir"
  Write-Host "  Stop: .\stop-exp-live-stack.ps1 -DataDir $(Quote-DevPowerShell $DataDir)"
  if (-not $AuthToken) { Write-Warning 'No authentication token is configured. Use only on a trusted private network.' }
} catch {
  foreach ($process in $processes) { Stop-DevProcessTree -Process $process }
  if ($buildDir) { Remove-Item -LiteralPath $buildDir -Recurse -Force -ErrorAction SilentlyContinue }
  throw
} finally {
  foreach ($entry in $savedEnvironment.GetEnumerator()) { [Environment]::SetEnvironmentVariable($entry.Key, $entry.Value, 'Process') }
  $lock.Dispose()
}
