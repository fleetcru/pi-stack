[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

$serverDir = Join-Path $PSScriptRoot "pi-server-exp"
$outputPath = Join-Path $PSScriptRoot "pi-server.exe"

if (-not (Test-Path -LiteralPath $serverDir -PathType Container)) {
  throw "pi-server-exp not found at: $serverDir"
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
  throw "Go is not installed or is not available on PATH."
}

$previousGoos = $env:GOOS
$previousGoarch = $env:GOARCH
$previousCgoEnabled = $env:CGO_ENABLED

try {
  $env:GOOS = "windows"
  $env:GOARCH = "amd64"
  $env:CGO_ENABLED = "0"

  Push-Location $serverDir
  try {
    Write-Host "Building pi-server..." -ForegroundColor Cyan
    & go build -trimpath "-ldflags=-s -w" -o $outputPath ./cmd/pi-server
    if ($LASTEXITCODE -ne 0) {
      throw "pi-server build failed with exit code $LASTEXITCODE."
    }
  }
  finally {
    Pop-Location
  }
}
finally {
  $env:GOOS = $previousGoos
  $env:GOARCH = $previousGoarch
  $env:CGO_ENABLED = $previousCgoEnabled
}

Write-Host "Built: $outputPath" -ForegroundColor Green
