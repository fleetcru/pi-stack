[CmdletBinding()]
param(
  [ValidateRange(1, 65535)][int]$Port = 3142,
  [string]$AuthToken = $env:PI_SERVER_AUTH_TOKEN,
  [string]$DataDir = (Join-Path $PSScriptRoot ".data" | Join-Path -ChildPath "pi-server"),
  [switch]$OpenAdmin,
  [switch]$InstallExternalBridge,
  [string]$BridgeRelayUrl = ""
)

$ErrorActionPreference = "Stop"
$serverDir = Join-Path $PSScriptRoot "pi-server-exp"
. (Join-Path $PSScriptRoot 'dev-launcher-common.ps1')
foreach ($command in @('go', 'pi')) {
  if (-not (Get-Command $command -ErrorAction SilentlyContinue)) { throw "$command is not available in PATH." }
}
Assert-DevPortAvailable -Port $Port

if (-not (Test-Path -LiteralPath $serverDir -PathType Container)) {
  throw "pi-server-exp not found at: $serverDir"
}

# --- Detect a reachable home-LAN IP ---
# Do not use Tailscale, link-local (169.254.x.x), or carrier-grade NAT
# (100.64.x.x) addresses. Those are not reachable by a phone on home Wi-Fi.
$lanAddresses = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
  Where-Object {
    $_.AddressState -eq "Preferred" -and
    $_.IPAddress -match "^(10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[0-1])\.)"
  }
$preferredLan = $lanAddresses |
  Where-Object { $_.InterfaceAlias -match "Wi-Fi|WiFi|Wireless|Ethernet" } |
  Select-Object -First 1 -ExpandProperty IPAddress
$homeLanIp = if ($preferredLan) {
  $preferredLan
} else {
  $lanAddresses | Select-Object -First 1 -ExpandProperty IPAddress
}
$tailscaleIp = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
  Where-Object {
    $_.AddressState -eq "Preferred" -and
    ($_.InterfaceAlias -match "Tailscale" -or $_.IPAddress -match "^100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.")
  } |
  Select-Object -First 1 -ExpandProperty IPAddress

if (-not $homeLanIp -and -not $tailscaleIp) {
  Write-Warning "No home-LAN or Tailscale address detected. Clients may not reach the server."
}

# Restore the caller's environment even when build or startup fails.
$savedEnvironment = @{}
foreach ($name in @('PI_SERVER_ADDR', 'PI_SERVER_CWD', 'PI_SERVER_DATA_DIR', 'PI_SERVER_ALLOWED_ROOTS', 'PI_SERVER_PI_EXTENSIONS', 'PI_SERVER_AUTH_TOKEN', 'PI_SERVER_ALLOW_INSECURE')) {
  $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
$buildDir = Join-Path ([IO.Path]::GetTempPath()) "pi-dev-$([guid]::NewGuid().ToString('N'))"
try {
New-Item -ItemType Directory -Path $buildDir | Out-Null
$binary = Join-Path $buildDir 'pi-server.exe'
Build-DevServer -ServerDir $serverDir -Output $binary
# --- Setup ---
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
$DataDir = [IO.Path]::GetFullPath($DataDir)

# The server's built-in CORS policy accepts loopback, home-LAN, and Tailscale
# browser origins when no explicit allowlist is configured.
$bindHost = "0.0.0.0"
$env:PI_SERVER_ADDR         = "${bindHost}:$Port"
$env:PI_SERVER_CWD          = $PSScriptRoot
$env:PI_SERVER_DATA_DIR     = $DataDir
if (-not $env:PI_SERVER_ALLOWED_ROOTS) { $env:PI_SERVER_ALLOWED_ROOTS = $PSScriptRoot }
# Honor an explicit CORS allowlist rather than silently clearing it.

$extension = Join-Path $serverDir "extensions" | Join-Path -ChildPath "session-title.ts"
if (Test-Path -LiteralPath $extension -PathType Leaf) {
  $env:PI_SERVER_PI_EXTENSIONS = $extension
} else {
  Remove-Item Env:PI_SERVER_PI_EXTENSIONS -ErrorAction SilentlyContinue
}

if ($AuthToken) {
  $env:PI_SERVER_AUTH_TOKEN = $AuthToken
} else {
  Remove-Item Env:PI_SERVER_AUTH_TOKEN -ErrorAction SilentlyContinue
}
Remove-Item Env:PI_SERVER_ALLOW_INSECURE -ErrorAction SilentlyContinue

# The bridge belongs in interactive Pi TUI processes, not server-managed RPC
# processes. Install it globally only when explicitly requested, so future TUI
# sessions can register with this server without creating duplicate owners.
if ($InstallExternalBridge) {
  $installer = Join-Path $PSScriptRoot "install-exp-external-bridge.ps1"
  if (-not (Test-Path -LiteralPath $installer -PathType Leaf)) {
    throw "External bridge installer not found: $installer"
  }
  $relayUrl = if ($BridgeRelayUrl) { $BridgeRelayUrl.TrimEnd('/') } else { "http://127.0.0.1:$Port" }
  & $installer -ServerPort $Port -RelayUrl $relayUrl -AuthToken $AuthToken
}

# --- Launch ---
Write-Host ""
Write-Host "  pi-server-exp" -ForegroundColor Cyan
Write-Host "  ────────────────────────────────────" -ForegroundColor DarkGray
Write-Host "  Bind:      ${bindHost}:$Port"
if ($homeLanIp) { Write-Host "  Home LAN:  http://${homeLanIp}:$Port" }
if ($tailscaleIp) { Write-Host "  Tailscale: http://${tailscaleIp}:$Port" }
Write-Host "  Local:     http://127.0.0.1:$Port"
Write-Host "  Data:      $DataDir"
Write-Host "  Browser:   $(if ($env:PI_SERVER_ALLOWED_ORIGINS) { $env:PI_SERVER_ALLOWED_ORIGINS } else { 'automatic private-network policy' })"
if ($AuthToken) { Write-Host "  Auth:      configured" }
else { Write-Host "  Auth:      none (trusting home LAN/Tailscale)" -ForegroundColor Yellow }
Write-Host ""

if ($OpenAdmin) {
  Write-Warning '-OpenAdmin is obsolete. The standalone /admin/ page was removed. Use Webby or Desktop for administration, or scan the terminal pairing QR in Companion.'
}

# Run the built executable directly so Ctrl+C reaches the actual server,
# rather than leaving a go run child behind.
& $binary
if ($LASTEXITCODE -ne 0) { throw "pi-server exited with code $LASTEXITCODE. Check $DataDir\pi-server.log." }
} finally {
  foreach ($entry in $savedEnvironment.GetEnumerator()) { [Environment]::SetEnvironmentVariable($entry.Key, $entry.Value, 'Process') }
  Remove-Item -LiteralPath $buildDir -Recurse -Force -ErrorAction SilentlyContinue
}
