[CmdletBinding()]
param(
  [ValidateRange(1, 65535)][int]$ServerPort = 3142,
  [string]$RelayUrl = "",
  [ValidateScript({ $_ -notmatch '[\r\n\x00]' })][string]$AuthToken = ""
)

$ErrorActionPreference = "Stop"

$source = Join-Path $PSScriptRoot "pi-server-exp" | Join-Path -ChildPath "extensions" | Join-Path -ChildPath "external-session-bridge.ts"
$targetDir = Join-Path $HOME ".pi" | Join-Path -ChildPath "agent" | Join-Path -ChildPath "extensions"

if (-not (Test-Path -LiteralPath $source -PathType Leaf)) {
  throw "Bridge extension not found: $source"
}

# Validate and preserve configuration before changing the installed extension.
$configPath = Join-Path $HOME '.pi\agent\bridge-config.json'
$config = @{}
if (Test-Path -LiteralPath $configPath) {
  $existing = [IO.File]::ReadAllText($configPath) | ConvertFrom-Json
  foreach ($property in $existing.PSObject.Properties) { $config[$property.Name] = $property.Value }
}
$relayUrl = if ($RelayUrl) { $RelayUrl.TrimEnd('/') } elseif (-not $PSBoundParameters.ContainsKey('ServerPort') -and $config.relayUrl) { $config.relayUrl } else { "http://127.0.0.1:$ServerPort" }
$uri = $null
if (-not [Uri]::TryCreate($relayUrl, [UriKind]::Absolute, [ref]$uri) -or $uri.Scheme -notin @('http', 'https') -or $uri.UserInfo -or $uri.Query -or $uri.Fragment -or $uri.AbsolutePath -ne '/') {
  throw 'RelayUrl must be an HTTP or HTTPS server URL without credentials, a path, query, or fragment.'
}
if ($PSBoundParameters.ContainsKey('AuthToken')) { $config.relayToken = $AuthToken }
elseif ($config.relayUrl -and $config.relayUrl -ne $relayUrl) {
  # Never forward a saved credential to a newly selected relay.
  $config.relayToken = ''
  Write-Warning 'Relay changed. Supply -AuthToken to authenticate to the new server.'
}
$config.relayUrl = $relayUrl

# Pi automatically discovers files in ~/.pi/agent/extensions. No pi install
# registration is needed, and a second package declaration can load it twice.
# Copy the extension to the global Pi extensions directory
New-Item -ItemType Directory -Force -Path $targetDir | Out-Null
$installed = Join-Path $targetDir "external-session-bridge.ts"
$staged = Join-Path $targetDir "bridge-$([guid]::NewGuid().ToString('N')).tmp"
try {
  Copy-Item -LiteralPath $source -Destination $staged
  if (Test-Path -LiteralPath $installed) { [IO.File]::Replace($staged, $installed, [NullString]::Value) }
  else { [IO.File]::Move($staged, $installed) }
} finally { Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue }
Write-Host "Copied bridge extension to $installed" -ForegroundColor Green

# Write private UTF-8 without BOM. Node's JSON.parse rejects the BOM emitted
# by Windows PowerShell 5.1 Set-Content -Encoding utf8.
$stagedConfig = "$configPath.$([guid]::NewGuid().ToString('N')).tmp"
$sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
try {
  [IO.File]::WriteAllText($stagedConfig, ($config | ConvertTo-Json -Depth 20), (New-Object Text.UTF8Encoding $false))
  foreach ($path in @($stagedConfig, $configPath)) {
    if (Test-Path -LiteralPath $path) {
      & icacls.exe $path /inheritance:r /grant:r "*$sid`:(F)" '*S-1-5-18:(F)' '*S-1-5-32-544:(F)' | Out-Null
      if ($LASTEXITCODE -ne 0) { throw "Could not restrict permissions on $path" }
    }
  }
  if (Test-Path -LiteralPath $configPath) { [IO.File]::Replace($stagedConfig, $configPath, [NullString]::Value) }
  else { [IO.File]::Move($stagedConfig, $configPath) }
} finally { Remove-Item -LiteralPath $stagedConfig -Force -ErrorAction SilentlyContinue }
# Configuration is authoritative. Do not expose the credential to every future
# process through the persistent user environment.
[Environment]::SetEnvironmentVariable('PI_EXTERNAL_RELAY_URL', $relayUrl, 'User')
[Environment]::SetEnvironmentVariable('PI_EXTERNAL_RELAY_TOKEN', $null, 'User')
if (-not (Get-Command pi -ErrorAction SilentlyContinue)) { Write-Warning 'Pi CLI is not in PATH. The extension will load once Pi is installed.' }

Write-Host ""
Write-Host "Installed external-session bridge." -ForegroundColor Green
Write-Host "  Relay URL: $relayUrl"
Write-Host "  Config:    $configPath"
Write-Host ""
Write-Host "Run 'pi' to start a relay session, or /reload in an existing Pi terminal." -ForegroundColor Cyan
