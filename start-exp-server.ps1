[CmdletBinding()]
param(
  [int]$Port = 3142,
  [string]$AuthToken = "",
  [string]$DataDir = (Join-Path $PSScriptRoot ".data" | Join-Path -ChildPath "pi-server"),
  [switch]$AllowInsecure
)

$ErrorActionPreference = "Stop"
$serverDir = Join-Path $PSScriptRoot "pi-server-exp"

if (-not (Test-Path -LiteralPath $serverDir -PathType Container)) {
  throw "pi-server-exp not found at: $serverDir"
}

# --- Detect Tailscale IP ---
$tailscaleIp = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
  Where-Object { $_.InterfaceAlias -match "Tailscale" -and $_.AddressState -eq "Preferred" } |
  Select-Object -First 1 -ExpandProperty IPAddress

if (-not $tailscaleIp) {
  Write-Warning "Tailscale interface not found. Falling back to LAN detection."
  $tailscaleIp = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
    Where-Object { $_.IPAddress -notlike "127.*" -and $_.IPAddress -notlike "169.254.*" -and $_.AddressState -eq "Preferred" } |
    Select-Object -First 1 -ExpandProperty IPAddress
}

if (-not $tailscaleIp) {
  Write-Warning "No network IP detected. Binding to 0.0.0.0 but clients may not reach the server."
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

$env:PI_SERVER_ADDR         = "0.0.0.0:$Port"
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
  # Allow binding to 0.0.0.0 without auth. Tailscale encrypts and authenticates
  # the connection, so the pi-server auth token is redundant in that context.
  # Use -AuthToken for defense-in-depth if you prefer.
  $env:PI_SERVER_ALLOW_INSECURE = "1"
}

# --- Launch ---
Write-Host ""
Write-Host "  pi-server-exp" -ForegroundColor Cyan
Write-Host "  ────────────────────────────────────" -ForegroundColor DarkGray
Write-Host "  Bind:      0.0.0.0:$Port"
Write-Host "  Tailscale: http://${tailscaleIp}:$Port"
Write-Host "  Data:      $DataDir"
Write-Host "  Origins:   $origins"
if ($AuthToken) { Write-Host "  Auth:      configured" }
else { Write-Host "  Auth:      none (Tailscale/trusted LAN)" -ForegroundColor Yellow }
Write-Host ""

Set-Location $serverDir
go run ./cmd/pi-server
