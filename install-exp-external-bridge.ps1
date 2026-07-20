[CmdletBinding()]
param(
  [int]$ServerPort = 3142,
  [string]$AuthToken = ""
)

$ErrorActionPreference = "Stop"

$source = Join-Path $PSScriptRoot "pi-server-exp" | Join-Path -ChildPath "extensions" | Join-Path -ChildPath "external-session-bridge.ts"
$targetDir = Join-Path $HOME ".pi" | Join-Path -ChildPath "agent" | Join-Path -ChildPath "extensions"

if (-not (Test-Path -LiteralPath $source -PathType Leaf)) {
  throw "Bridge extension not found: $source"
}

# Copy the extension to the global Pi extensions directory
New-Item -ItemType Directory -Force -Path $targetDir | Out-Null
$installed = Join-Path $targetDir "external-session-bridge.ts"
Copy-Item $source $installed -Force
Write-Host "Copied bridge extension to $installed" -ForegroundColor Green

# Register the extension with Pi if not already registered
$piCommand = Get-Command pi -ErrorAction SilentlyContinue
if ($piCommand) {
  $listed = & pi list 2>$null | Out-String
  if ($listed -notmatch [regex]::Escape("external-session-bridge.ts")) {
    Write-Host "Registering bridge extension with Pi..." -ForegroundColor Yellow
    Push-Location $targetDir
    try {
      & pi install ./external-session-bridge.ts
      if ($LASTEXITCODE -ne 0) { Write-Warning "pi install exited with code $LASTEXITCODE. The extension may not load automatically." }
    } catch {
      Write-Warning "Failed to register extension with Pi: $_. The extension file is copied but may not load on startup."
    } finally {
      Pop-Location
    }
  } else {
    Write-Host "Bridge extension already registered with Pi." -ForegroundColor Green
  }
} else {
  Write-Warning "Pi CLI not found in PATH. The extension is copied to $targetDir but cannot be registered. Install Pi CLI first, then re-run this script."
}

# Set environment variables for relay connection
$relayUrl = "http://127.0.0.1:$ServerPort"
[Environment]::SetEnvironmentVariable("PI_EXTERNAL_RELAY_URL", $relayUrl, "User")
if ($AuthToken) {
  [Environment]::SetEnvironmentVariable("PI_EXTERNAL_RELAY_TOKEN", $AuthToken, "User")
} else {
  [Environment]::SetEnvironmentVariable("PI_EXTERNAL_RELAY_TOKEN", $null, "User")
}

# Write config file for reference
$configPath = Join-Path $HOME ".pi" | Join-Path -ChildPath "agent" | Join-Path -ChildPath "bridge-config.json"
@{ relayUrl = $relayUrl; relayToken = if ($AuthToken) { "***" } else { "" } } | ConvertTo-Json | Set-Content $configPath -Encoding utf8

Write-Host ""
Write-Host "Installed external-session bridge." -ForegroundColor Green
Write-Host "  Relay URL: $relayUrl"
Write-Host "  Config:    $configPath"
Write-Host ""
Write-Host "Open a new terminal, then run 'pi' to start a relay session." -ForegroundColor Cyan
