[CmdletBinding()]
param(
  [string]$Source = "$HOME\.pi\agent",
  # Keep generated bundles with pi-server rather than in whichever folder
  # invoked the script. This remains stable if the whole stack is moved.
  [string]$Output = (Join-Path $PSScriptRoot "..\dist\pi-agent-vps-bundle.zip"),
  [switch]$IncludeAuth
)

$ErrorActionPreference = "Stop"
$Output = [System.IO.Path]::GetFullPath($Output)
$null = New-Item -ItemType Directory -Path (Split-Path -Parent $Output) -Force

if (-not (Test-Path -LiteralPath $Source -PathType Container)) {
  throw "Pi agent directory not found: $Source"
}
if (-not $IncludeAuth) {
  throw "Refusing to create a bundle without an explicit choice. Re-run with -IncludeAuth to include auth.json."
}

$stage = Join-Path ([System.IO.Path]::GetTempPath()) ("pi-agent-bundle-" + [guid]::NewGuid())
$agentStage = Join-Path $stage "agent"
New-Item -ItemType Directory -Path $agentStage -Force | Out-Null

try {
  # Portable Pi resources. Deliberately exclude sessions, Windows-only binaries,
  # caches, and npm package caches; the VPS gets its own session history/runtime.
  $files = @("auth.json", "settings.json", "trust.json", "pi-permissions.jsonc", "agent-manager-registry.json")
  foreach ($file in $files) {
    $sourceFile = Join-Path $Source $file
    if (Test-Path -LiteralPath $sourceFile -PathType Leaf) {
      Copy-Item -LiteralPath $sourceFile -Destination (Join-Path $agentStage $file) -Force
    }
  }
  # MCP server definitions live outside .pi. Preserve the same relative
  # location in the bundle so the Linux installer can place them in ~/.config.
  $mcpConfig = Join-Path $HOME ".config\mcp\mcp.json"
  if (Test-Path -LiteralPath $mcpConfig -PathType Leaf) {
    $mcpStage = Join-Path $stage "config\mcp"
    New-Item -ItemType Directory -Path $mcpStage -Force | Out-Null
    Copy-Item -LiteralPath $mcpConfig -Destination (Join-Path $mcpStage "mcp.json") -Force
  } else {
    Write-Warning "No MCP config found at $mcpConfig; it will not be included."
  }

  foreach ($directory in @("agents", "extensions", "skills", "prompts", "themes")) {
    $sourceDirectory = Join-Path $Source $directory
    if (Test-Path -LiteralPath $sourceDirectory -PathType Container) {
      Copy-Item -LiteralPath $sourceDirectory -Destination (Join-Path $agentStage $directory) -Recurse -Force
    }
  }

  # Convert configured resource paths to forward slashes for the Linux VPS.
  $settingsPath = Join-Path $agentStage "settings.json"
  if (Test-Path -LiteralPath $settingsPath) {
    $settings = Get-Content -LiteralPath $settingsPath -Raw | ConvertFrom-Json
    foreach ($property in @("packages", "extensions", "skills", "prompts", "themes")) {
      if ($null -ne $settings.$property) {
        $settings.$property = @($settings.$property | ForEach-Object {
          if ($_ -is [string]) { $_ -replace "\\", "/" } else { $_ }
        })
      }
    }
    $settings | ConvertTo-Json -Depth 100 | Set-Content -LiteralPath $settingsPath -Encoding utf8
  }

  [pscustomobject]@{
    createdAt = (Get-Date).ToUniversalTime().ToString("o")
    source = $Source
    includesAuth = $true
    includesMcpConfig = (Test-Path -LiteralPath $mcpConfig)
    excludes = @("sessions", "bin", "npm", "caches")
  } | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $stage "manifest.json") -Encoding utf8

  Remove-Item -LiteralPath $Output -Force -ErrorAction SilentlyContinue
  Compress-Archive -Path (Join-Path $stage "*") -DestinationPath $Output -Force
  Write-Host "Created $Output" -ForegroundColor Green
  Write-Warning "This archive includes auth.json. Transfer only over SSH/Tailscale and delete it after installing on the VPS."
}
finally {
  Remove-Item -LiteralPath $stage -Recurse -Force -ErrorAction SilentlyContinue
}
