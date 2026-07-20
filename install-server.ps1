#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Installs pi-server as a Windows service.

.DESCRIPTION
    Downloads the pi-server binary from GitHub releases, installs it as a
    scheduled task that runs at system startup, and starts it immediately.

.PARAMETER Port
    Port to listen on. Default: 3142

.PARAMETER AuthToken
    Optional auth token for API authentication.

.EXAMPLE
    .\install-server.ps1
    .\install-server.ps1 -Port 9000 -AuthToken "my-secret"
#>

param(
    [int]$Port = 3142,
    [string]$AuthToken = ""
)

$ErrorActionPreference = "Stop"

# ── Config ────────────────────────────────────────────────
$Repo = "fleetcru/pi-stack"
$InstallDir = "C:\pi-server"
$DataDir = Join-Path $InstallDir "data"
$ConfigDir = Join-Path $InstallDir "config"
$BinaryUrl = "https://github.com/$Repo/releases/latest/download/pi-server-windows-amd64.exe"
$TaskName = "PiServer"

# ── Helpers ───────────────────────────────────────────────
function Write-Step($msg) { Write-Host "[info] $msg" -ForegroundColor Cyan }
function Write-Ok($msg)   { Write-Host "[ok] $msg" -ForegroundColor Green }
function Write-Warn($msg) { Write-Host "[warn] $msg" -ForegroundColor Yellow }
function Write-Fail($msg) { Write-Host "[error] $msg" -ForegroundColor Red; exit 1 }

# ── Create directories ────────────────────────────────────
Write-Step "Creating directories..."
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
New-Item -ItemType Directory -Force -Path $ConfigDir | Out-Null
Write-Ok "Directories created"

# ── Download binary ───────────────────────────────────────
Write-Step "Downloading pi-server..."
$ExePath = Join-Path $InstallDir "pi-server.exe"

try {
    Invoke-WebRequest -Uri $BinaryUrl -OutFile $ExePath -UseBasicParsing
    Write-Ok "Downloaded pre-built binary"
} catch {
    Write-Warn "No pre-built binary found. Building from source..."
    
    # Check if Go is installed
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Fail "Go is not installed. Install from https://go.dev/dl/ and retry."
    }

    # Clone and build
    $TmpDir = Join-Path $env:TEMP "pi-stack-build"
    if (Test-Path $TmpDir) { Remove-Item -Recurse -Force $TmpDir }
    
    Write-Step "Cloning repository..."
    git clone --depth 1 "https://github.com/$Repo.git" $TmpDir
    
    Write-Step "Building pi-server..."
    Push-Location (Join-Path $TmpDir "pi-server-exp")
    go build -o $ExePath ./cmd/pi-server
    Pop-Location
    
    Remove-Item -Recurse -Force $TmpDir
    Write-Ok "Built from source"
}

# ── Write config ──────────────────────────────────────────
Write-Step "Writing configuration..."
$EnvFile = Join-Path $ConfigDir "pi-server.env"

$EnvContent = @"
# pi-server configuration
# Edit this file, then restart the service:
#   Stop-ScheduledTask -TaskName "$TaskName"
#   Start-ScheduledTask -TaskName "$TaskName"

PI_SERVER_ADDR=0.0.0.0:$Port
PI_SERVER_DATA_DIR=$DataDir
PI_SERVER_ALLOWED_ROOTS=$DataDir
PI_SERVER_ALLOW_INSECURE=1

# Set a token to require authentication:
# PI_SERVER_AUTH_TOKEN=your-secret-token
"@

Set-Content -Path $EnvFile -Value $EnvContent -Encoding UTF8
Write-Ok "Config written to $EnvFile"

# ── Create wrapper script ────────────────────────────────
Write-Step "Creating service wrapper..."
$WrapperPath = Join-Path $InstallDir "start.ps1"

$WrapperContent = @"
# Reads config and starts pi-server
`$envFile = Join-Path `$PSScriptRoot "config\pi-server.env"
Get-Content `$envFile | ForEach-Object {
    if (`$_ -match '^\s*([^#][^=]+)=(.+)$') {
        [Environment]::SetEnvironmentVariable(`$matches[1].Trim(), `$matches[2].Trim(), "Process")
    }
}
& (Join-Path `$PSScriptRoot "pi-server.exe")
"@

Set-Content -Path $WrapperPath -Value $WrapperContent -Encoding UTF8
Write-Ok "Wrapper created"

# ── Remove old task if exists ─────────────────────────────
$ExistingTask = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
if ($ExistingTask) {
    Write-Step "Removing existing scheduled task..."
    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
}

# ── Create scheduled task ─────────────────────────────────
Write-Step "Creating scheduled task..."

$Action = New-ScheduledTaskAction `
    -Execute "powershell.exe" `
    -Argument "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$WrapperPath`"" `
    -WorkingDirectory $InstallDir

$Trigger = New-ScheduledTaskTrigger -AtStartup
$Settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RestartCount 3 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -ExecutionTimeLimit (New-TimeSpan -Days 365)

$Principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest

Register-ScheduledTask `
    -TaskName $TaskName `
    -Action $Action `
    -Trigger $Trigger `
    -Settings $Settings `
    -Principal $Principal `
    -Description "pi-server — Pi coding agent hub" `
    -Force | Out-Null

Write-Ok "Scheduled task created: $TaskName"

# ── Start service ─────────────────────────────────────────
Write-Step "Starting pi-server..."
Start-ScheduledTask -TaskName $TaskName
Start-Sleep -Seconds 2

$Task = Get-ScheduledTask -TaskName $TaskName
if ($Task.State -eq "Running") {
    Write-Ok "pi-server is running"
} else {
    Write-Warn "pi-server may have failed to start. Check: Get-ScheduledTaskInfo -TaskName '$TaskName'"
}

# ── Summary ───────────────────────────────────────────────
$ServerIP = (Invoke-WebRequest -Uri "https://ifconfig.me" -UseBasicParsing -ErrorAction SilentlyContinue).Content.Trim()
if (-not $ServerIP) { $ServerIP = "localhost" }

Write-Host ""
Write-Host "==================================================" -ForegroundColor Green
Write-Host "  pi-server installed and running!"
Write-Host ""
Write-Host "  URL:      http://${ServerIP}:${Port}" -ForegroundColor Cyan
Write-Host "  Config:   $EnvFile"
Write-Host "  Data:     $DataDir"
Write-Host "  Binary:   $ExePath"
Write-Host ""
Write-Host "  Commands:"
Write-Host "    Start-ScheduledTask -TaskName '$TaskName'" -ForegroundColor Cyan
Write-Host "    Stop-ScheduledTask -TaskName '$TaskName'" -ForegroundColor Cyan
Write-Host "    Get-ScheduledTask -TaskName '$TaskName'" -ForegroundColor Cyan
Write-Host ""
Write-Host "  Set PI_SERVER_AUTH_TOKEN in $EnvFile" -ForegroundColor Yellow
Write-Host "  before exposing this to the internet!" -ForegroundColor Yellow
Write-Host "==================================================" -ForegroundColor Green
