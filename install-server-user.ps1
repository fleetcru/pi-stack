<#
.SYNOPSIS
    Installs pi-server for the current user (no admin required).

.DESCRIPTION
    Downloads the pi-server binary from GitHub releases, installs it to
    %LOCALAPPDATA%\pi-server, and creates a logon task to start automatically.

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
$InstallDir = Join-Path $env:LOCALAPPDATA "pi-server"
$DataDir = Join-Path $InstallDir "data"
$ConfigDir = Join-Path $InstallDir "config"
$BinaryUrl = "https://github.com/$Repo/releases/latest/download/pi-server-windows-amd64.exe"
$TaskName = "PiServer-$env:USERNAME"

# ── Helpers ───────────────────────────────────────────────
function Write-Step($msg) { Write-Host "[info] $msg" -ForegroundColor Cyan }
function Write-Ok($msg)   { Write-Host "[ok] $msg" -ForegroundColor Green }
function Write-Warn($msg) { Write-Host "[warn] $msg" -ForegroundColor Yellow }
function Write-Fail($msg) { Write-Host "[error] $msg" -ForegroundColor Red; exit 1 }

# ── Create directories ────────────────────────────────────
Write-Step "Creating directories in $InstallDir..."
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
    
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Fail "Go is not installed. Install from https://go.dev/dl/ and retry."
    }

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
# Edit this file, then restart the task:
#   Stop-ScheduledTask -TaskName "$TaskName"
#   Start-ScheduledTask -TaskName "$TaskName"

PI_SERVER_ADDR=127.0.0.1:$Port
PI_SERVER_DATA_DIR=$DataDir
PI_SERVER_ALLOWED_ROOTS=$DataDir

# For LAN/Tailscale access, uncomment and set:
# PI_SERVER_ADDR=0.0.0.0:$Port
# PI_SERVER_ALLOW_INSECURE=1

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

# ── Create scheduled task (logon, current user, no admin) ─
Write-Step "Creating logon task..."

$Action = New-ScheduledTaskAction `
    -Execute "powershell.exe" `
    -Argument "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$WrapperPath`"" `
    -WorkingDirectory $InstallDir

$Trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME

$Settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RestartCount 3 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -ExecutionTimeLimit (New-TimeSpan -Days 365)

Register-ScheduledTask `
    -TaskName $TaskName `
    -Action $Action `
    -Trigger $Trigger `
    -Settings $Settings `
    -Description "pi-server — Pi coding agent hub (user install)" `
    -Force | Out-Null

Write-Ok "Logon task created: $TaskName"

# ── Start now ─────────────────────────────────────────────
Write-Step "Starting pi-server..."
Start-ScheduledTask -TaskName $TaskName
Start-Sleep -Seconds 2

$Task = Get-ScheduledTask -TaskName $TaskName
if ($Task.State -eq "Running") {
    Write-Ok "pi-server is running"
} else {
    Write-Warn "pi-server may have failed to start."
    Write-Warn "Check: Get-ScheduledTaskInfo -TaskName '$TaskName'"
}

# ── Summary ───────────────────────────────────────────────
Write-Host ""
Write-Host "==================================================" -ForegroundColor Green
Write-Host "  pi-server installed for current user!"
Write-Host ""
Write-Host "  URL:      http://127.0.0.1:$Port" -ForegroundColor Cyan
Write-Host "  Config:   $EnvFile"
Write-Host "  Data:     $DataDir"
Write-Host "  Binary:   $ExePath"
Write-Host ""
Write-Host "  Commands:"
Write-Host "    Start-ScheduledTask -TaskName '$TaskName'" -ForegroundColor Cyan
Write-Host "    Stop-ScheduledTask -TaskName '$TaskName'" -ForegroundColor Cyan
Write-Host "    Get-ScheduledTask -TaskName '$TaskName'" -ForegroundColor Cyan
Write-Host "    Unregister-ScheduledTask -TaskName '$TaskName'" -ForegroundColor Cyan
Write-Host ""
Write-Host "  To uninstall: run the Unregister command above,"
Write-Host "  then delete $InstallDir"
Write-Host "==================================================" -ForegroundColor Green
