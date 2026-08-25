[CmdletBinding()]
# Keep the live development stack on 3142; pi-server standalone defaults to 3141.
# This avoids confusing a separately running server with the stack launcher.
param(
  [int]$ServerPort = 3142,
  [int]$WebPort = 5174,
  [string]$AuthToken = "",
  [switch]$AllowInsecure,
  [switch]$InstallGlobalBridge,
  [switch]$Background,
  [switch]$Minimized,
  [bool]$LaunchPi = $true,
  [int]$MaxSessions = 16,
  [int]$MaxActiveRuns = 8,
  [int]$HealthTimeoutSec = 15
)

$ErrorActionPreference = "Stop"
$root = $PSScriptRoot
$serverDir = Join-Path $root "pi-server-exp"
$webDir = Join-Path $root "pi-webby-exp"
$bridge = Join-Path $serverDir "extensions" | Join-Path -ChildPath "external-session-bridge.ts"
$dataDir = Join-Path $serverDir ".data" | Join-Path -ChildPath "pi-server"

foreach ($path in @($serverDir, $webDir, $bridge)) {
  if (-not (Test-Path -LiteralPath $path)) { throw "Required exp-stack path is missing: $path" }
}

# --- Detect LAN IP for mobile access ---
$lanIp = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
  Where-Object { $_.IPAddress -notlike "127.*" -and $_.IPAddress -notlike "169.254.*" -and $_.AddressState -eq "Preferred" } |
  Select-Object -First 1 -ExpandProperty IPAddress
if (-not $lanIp) {
  Write-Warning "Could not detect a LAN IPv4 address. Mobile devices will not be able to connect. Continuing with localhost only."
  $lanIp = "127.0.0.1"
}

New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
$origins = "http://127.0.0.1:$WebPort,http://localhost:$WebPort,http://${lanIp}:$WebPort"

# --- Install global bridge extension if requested ---
if ($InstallGlobalBridge) {
  $globalExtensions = Join-Path $HOME ".pi" | Join-Path -ChildPath "agent" | Join-Path -ChildPath "extensions"
  New-Item -ItemType Directory -Force -Path $globalExtensions | Out-Null
  Copy-Item $bridge (Join-Path $globalExtensions "external-session-bridge.ts") -Force
  [Environment]::SetEnvironmentVariable("PI_EXTERNAL_RELAY_URL", "http://127.0.0.1:$ServerPort", "User")
  if ($AuthToken) { [Environment]::SetEnvironmentVariable("PI_EXTERNAL_RELAY_TOKEN", $AuthToken, "User") }
  Write-Host "Installed the global Pi bridge extension for future Pi terminals." -ForegroundColor Green
}

# --- Helper: quote a string for safe embedding in a PowerShell command ---
function Quote-PowerShell([string]$value) { "'" + $value.Replace("'", "''") + "'" }

# --- Build server command ---
$bindHost = if ($AuthToken -or $AllowInsecure) { "0.0.0.0" } else { "127.0.0.1" }
$serverCommand = @(
  "`$env:PI_SERVER_ADDR = '${bindHost}:$ServerPort'"
  "`$env:PI_SERVER_CWD = $(Quote-PowerShell $root)"
  "`$env:PI_SERVER_DATA_DIR = $(Quote-PowerShell $dataDir)"
  "`$env:PI_SERVER_ALLOWED_ROOTS = $(Quote-PowerShell $root)"
  "`$env:PI_SERVER_ALLOWED_ORIGINS = $(Quote-PowerShell $origins)"
  "`$env:PI_SERVER_PI_EXTENSIONS = $(Quote-PowerShell (Join-Path $serverDir 'extensions' | Join-Path -ChildPath 'session-title.ts'))"
  "`$env:PI_SERVER_MAX_SESSIONS = '$MaxSessions'"
  "`$env:PI_SERVER_MAX_ACTIVE_RUNS = '$MaxActiveRuns'"
  $(if ($AuthToken) { "`$env:PI_SERVER_AUTH_TOKEN = $(Quote-PowerShell $AuthToken); Remove-Item Env:PI_SERVER_ALLOW_INSECURE -ErrorAction SilentlyContinue" } elseif ($AllowInsecure) { "Remove-Item Env:PI_SERVER_AUTH_TOKEN -ErrorAction SilentlyContinue; `$env:PI_SERVER_ALLOW_INSECURE = '1'" } else { "Remove-Item Env:PI_SERVER_AUTH_TOKEN -ErrorAction SilentlyContinue; Remove-Item Env:PI_SERVER_ALLOW_INSECURE -ErrorAction SilentlyContinue" })
  "Set-Location $(Quote-PowerShell $serverDir)"
  "go run ./cmd/pi-server"
) -join "; "

# --- Build web command ---
$webCommand = "Set-Location $(Quote-PowerShell $webDir); pnpm exec vite --host 0.0.0.0 --port $WebPort --strictPort"

# --- Launch processes ---
$windowStyle = if ($Background) { "Hidden" } elseif ($Minimized) { "Minimized" } else { "Normal" }
$psBaseArgs = @("-NoProfile", "-ExecutionPolicy", "Bypass")
if (-not $Background) { $psBaseArgs += "-NoExit" }
$psBaseArgs += @("-Command")

$serverProc = Start-Process powershell.exe -WindowStyle $windowStyle -ArgumentList ($psBaseArgs + $serverCommand) -PassThru | Out-Null
$webProc = Start-Process powershell.exe -WindowStyle $windowStyle -ArgumentList ($psBaseArgs + $webCommand) -PassThru | Out-Null

if ($LaunchPi) {
  $installedBridge = Join-Path $HOME ".pi" | Join-Path -ChildPath "agent" | Join-Path -ChildPath "extensions" | Join-Path -ChildPath "external-session-bridge.ts"
  $piLaunch = if (Test-Path -LiteralPath $installedBridge -PathType Leaf) { "pi" } else { "pi -e $(Quote-PowerShell $bridge)" }
  $piCommand = @(
    "`$env:PI_EXTERNAL_RELAY_URL = 'http://127.0.0.1:$ServerPort'"
    $(if ($AuthToken) { "`$env:PI_EXTERNAL_RELAY_TOKEN = $(Quote-PowerShell $AuthToken)" } else { "Remove-Item Env:PI_EXTERNAL_RELAY_TOKEN -ErrorAction SilentlyContinue" })
    "Set-Location $(Quote-PowerShell $root)"
    # Load the repository bridge explicitly unless the global copy exists.
    $piLaunch
  ) -join "; "
  $piProc = Start-Process powershell.exe -WindowStyle $windowStyle -ArgumentList ($psBaseArgs + $piCommand) -PassThru | Out-Null
}

# --- Health check: wait for server to respond ---
Write-Host "Waiting for pi-server to become ready..." -ForegroundColor Yellow
$deadline = [DateTime]::UtcNow.AddSeconds($HealthTimeoutSec)
$serverReady = $false
while ([DateTime]::UtcNow -lt $deadline) {
  try {
    $response = Invoke-WebRequest -Uri "http://127.0.0.1:$ServerPort/healthz" -UseBasicParsing -TimeoutSec 2 -ErrorAction Stop
    if ($response.StatusCode -eq 200) { $serverReady = $true; break }
  } catch { }
  Start-Sleep -Milliseconds 500
}

if ($serverReady) {
  Write-Host "pi-server is ready." -ForegroundColor Green
} else {
  Write-Warning "pi-server did not respond within ${HealthTimeoutSec}s. Check the server window for errors."
}

# --- Summary ---
Write-Host ""
Write-Host "Started exp live stack$(if ($Background) { ' in background mode' } else { '' })." -ForegroundColor Green
Write-Host "  Webby:      http://127.0.0.1:$WebPort  (phone: http://${lanIp}:$WebPort)"
Write-Host "  pi-server:  http://$(if ($bindHost -eq '127.0.0.1') { '127.0.0.1' } else { $lanIp }):$ServerPort"
Write-Host "  CORS:       $origins"
if ($AuthToken) { Write-Host "  Auth:       token configured" } else { Write-Host "  Auth:       none (trusted LAN only)" -ForegroundColor Yellow }
Write-Host ""
if ($Background) {
  Write-Host "Background mode: use the server shutdown/stop command or terminate the spawned processes to stop the stack." -ForegroundColor DarkGray
} else {
  Write-Host "Close the spawned PowerShell windows to stop each service." -ForegroundColor DarkGray
}
