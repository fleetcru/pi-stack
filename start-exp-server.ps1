[CmdletBinding()]
param(
  [int]$Port = 3142,
  [string]$AuthToken = "",
  [string]$DataDir = (Join-Path $PSScriptRoot ".data" | Join-Path -ChildPath "pi-server"),
  [switch]$AllowInsecure,
  [switch]$OpenAdmin,
  [switch]$InstallExternalBridge,
  [string]$BridgeRelayUrl = ""
)

$ErrorActionPreference = "Stop"
$serverDir = Join-Path $PSScriptRoot "pi-server-exp"

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
$tailscaleIp = if ($preferredLan) {
  $preferredLan
} else {
  $lanAddresses | Select-Object -First 1 -ExpandProperty IPAddress
}

if (-not $tailscaleIp) {
  Write-Warning "No private LAN IP detected. Binding to 0.0.0.0 but clients may not reach the server."
  $tailscaleIp = "127.0.0.1"
}

# --- Setup ---
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null

# Allow origins from Tailscale IP, localhost, and common web dev ports
$origins = @(
  "http://127.0.0.1:5173", "http://localhost:5173"
  "http://127.0.0.1:5174", "http://localhost:5174"
  "http://${tailscaleIp}:5173", "http://${tailscaleIp}:5174"
) -join ","

$bindHost = if ($AuthToken -or $AllowInsecure) { "0.0.0.0" } else { "127.0.0.1" }
$env:PI_SERVER_ADDR         = "${bindHost}:$Port"
$env:PI_SERVER_CWD          = $PSScriptRoot
$env:PI_SERVER_DATA_DIR     = $DataDir
$env:PI_SERVER_ALLOWED_ROOTS = $PSScriptRoot
$env:PI_SERVER_ALLOWED_ORIGINS = $origins

$extension = Join-Path $serverDir "extensions" | Join-Path -ChildPath "session-title.ts"
if (Test-Path -LiteralPath $extension -PathType Leaf) {
  $env:PI_SERVER_PI_EXTENSIONS = $extension
} else {
  Remove-Item Env:PI_SERVER_PI_EXTENSIONS -ErrorAction SilentlyContinue
}

if ($AuthToken) {
  $env:PI_SERVER_AUTH_TOKEN = $AuthToken
  Remove-Item Env:PI_SERVER_ALLOW_INSECURE -ErrorAction SilentlyContinue
} else {
  Remove-Item Env:PI_SERVER_AUTH_TOKEN -ErrorAction SilentlyContinue
  if ($AllowInsecure) {
    $env:PI_SERVER_ALLOW_INSECURE = "1"
  } else {
    Remove-Item Env:PI_SERVER_ALLOW_INSECURE -ErrorAction SilentlyContinue
  }
}

# The bridge belongs in interactive Pi TUI processes, not server-managed RPC
# processes. Install it globally only when explicitly requested, so future TUI
# sessions can register with this server without creating duplicate owners.
if ($InstallExternalBridge) {
  $installer = Join-Path $PSScriptRoot "install-exp-external-bridge.ps1"
  if (-not (Test-Path -LiteralPath $installer -PathType Leaf)) {
    throw "External bridge installer not found: $installer"
  }
  $relayUrl = if ($BridgeRelayUrl) { $BridgeRelayUrl.TrimEnd('/') } else { "http://127.0.0.1:$Port" }
  & powershell.exe -NoProfile -ExecutionPolicy RemoteSigned -File $installer `
    -ServerPort $Port `
    -RelayUrl $relayUrl `
    -AuthToken $AuthToken
  if ($LASTEXITCODE -ne 0) { throw "External bridge installation failed with exit code $LASTEXITCODE" }
}

# --- Launch ---
Write-Host ""
Write-Host "  pi-server-exp" -ForegroundColor Cyan
Write-Host "  ────────────────────────────────────" -ForegroundColor DarkGray
Write-Host "  Bind:      ${bindHost}:$Port"
Write-Host "  Tailscale: http://${tailscaleIp}:$Port"
Write-Host "  Data:      $DataDir"
Write-Host "  Origins:   $origins"
if ($AuthToken) { Write-Host "  Auth:      configured" }
else { Write-Host "  Auth:      none (Tailscale/trusted LAN)" -ForegroundColor Yellow }
Write-Host ""

if ($OpenAdmin) {
  Start-Job -ArgumentList "http://127.0.0.1:$Port", $Port -ScriptBlock {
    param($baseUrl, $serverPort)
    for ($attempt = 0; $attempt -lt 60; $attempt++) {
      try {
        $health = Invoke-WebRequest -Uri "$baseUrl/healthz" -UseBasicParsing -TimeoutSec 2 -ErrorAction Stop
        if ($health.StatusCode -eq 200) {
          Start-Process "$baseUrl/admin/"
          return
        }
      } catch { }
      Start-Sleep -Milliseconds 500
    }
  } | Out-Null
  Write-Host "  Opened Pi Server Admin. Create a trusted device there, then scan its QR from Companion." -ForegroundColor Green
}

Set-Location $serverDir
go run ./cmd/pi-server
