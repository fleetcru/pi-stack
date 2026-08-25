#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Installs pi-server system-wide as a Windows service (requires admin).

.DESCRIPTION
    Downloads the pi-server binary from GitHub releases, installs it to
    C:\pi-server, and creates a scheduled task that runs at startup as SYSTEM.

.PARAMETER Port
    Port to listen on. Default: 3142

.PARAMETER AuthToken
    Auth token for API authentication. Auto-generated if not provided.

.PARAMETER AllowInsecure
    Allow binding to 0.0.0.0 without auth enforcement. Use only on trusted networks.

.PARAMETER SourceRevision
    Exact Git commit used only when a release binary is unavailable.

.EXAMPLE
    .\install-server.ps1
    .\install-server.ps1 -Port 9000 -AuthToken "my-secret"
    .\install-server.ps1 -AllowInsecure
#>

param(
    [int]$Port = 3142,
    [string]$AuthToken = "",
    [switch]$AllowInsecure,
    [ValidatePattern('^[0-9a-fA-F]{40}$')]
    [string]$SourceRevision = "098d635625f0bdb1edbb2e84f148d093afcfe8da"
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "windows-installer-common.ps1")

# ── Config ────────────────────────────────────────────────
$Repo = "fleetcru/pi-stack"
$InstallDir = "C:\pi-server"
$DataDir = Join-Path $InstallDir "data"
$ConfigDir = Join-Path $InstallDir "config"
$BinaryUrl = "https://github.com/$Repo/releases/latest/download/pi-server-windows-amd64.exe"
$ChecksumUrl = "https://github.com/$Repo/releases/latest/download/SHA256SUMS"
$TaskName = "PiServer"

# ── Helpers ───────────────────────────────────────────────
function Write-Step($msg) { Write-Host "[info] $msg" -ForegroundColor Cyan }
function Write-Ok($msg)   { Write-Host "[ok] $msg" -ForegroundColor Green }
function Write-Warn($msg) { Write-Host "[warn] $msg" -ForegroundColor Yellow }
function Write-Fail($msg) { Write-Host "[error] $msg" -ForegroundColor Red; exit 1 }

# Generate auth token if not provided
if (-not $AuthToken) {
    $bytes = New-Object byte[] 32
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
    $AuthToken = [Convert]::ToBase64String($bytes) -replace '[/+=]', '' | ForEach-Object { $_.Substring(0, [Math]::Min(32, $_.Length)) }
    Write-Step "Generated an auth token"
}

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
    $ChecksumPath = Join-Path $env:TEMP "pi-server-SHA256SUMS"
    Invoke-WebRequest -Uri $BinaryUrl -OutFile $ExePath -UseBasicParsing
    Invoke-WebRequest -Uri $ChecksumUrl -OutFile $ChecksumPath -UseBasicParsing
    $ExpectedHash = Get-ExpectedReleaseHash -ChecksumPath $ChecksumPath -AssetName "pi-server-windows-amd64.exe"
    Assert-ReleaseChecksum -FilePath $ExePath -ExpectedHash $ExpectedHash
    Remove-Item -LiteralPath $ChecksumPath -Force -ErrorAction SilentlyContinue
    Write-Ok "Downloaded and verified pre-built binary"
} catch {
    Write-Warn "No pre-built binary found. Building from source..."
    
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Fail "Go is not installed. Install from https://go.dev/dl/ and retry."
    }

    $TmpDir = Join-Path $env:TEMP "pi-stack-build"
    if (Test-Path $TmpDir) { Remove-Item -Recurse -Force $TmpDir }
    
    Write-Step "Fetching pinned source revision $SourceRevision..."
    New-Item -ItemType Directory -Force -Path $TmpDir | Out-Null
    git -C $TmpDir init
    git -C $TmpDir remote add origin "https://github.com/$Repo.git"
    git -C $TmpDir fetch --depth 1 origin $SourceRevision
    git -C $TmpDir checkout --detach FETCH_HEAD
    if ($LASTEXITCODE -ne 0) { Write-Fail "Could not fetch pinned source revision $SourceRevision" }

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
$PiCommand = Get-Command pi -ErrorAction SilentlyContinue
if (-not $PiCommand) { Write-Fail "Pi CLI is not installed or is not available in PATH." }
$PiBinary = $PiCommand.Source

$EnvContent = @"
# pi-server configuration
# Edit this file, then restart the task:
#   Stop-ScheduledTask -TaskName "$TaskName"
#   Start-ScheduledTask -TaskName "$TaskName"

PI_SERVER_ADDR=0.0.0.0:$Port
PI_SERVER_DATA_DIR=$DataDir
PI_SERVER_ALLOWED_ROOTS=$env:USERPROFILE
PI_SERVER_AUTH_TOKEN=$AuthToken
PI_SERVER_PI_BINARY=$PiBinary
"@

# Only add ALLOW_INSECURE if explicitly requested
if ($AllowInsecure) {
    $EnvContent += "`nPI_SERVER_ALLOW_INSECURE=1"
    Write-Warn "Running in INSECURE mode — auth token will not be enforced"
}

Set-Content -Path $EnvFile -Value $EnvContent -Encoding UTF8
$CurrentUserSid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
& icacls.exe $EnvFile /inheritance:r /grant:r "*$CurrentUserSid`:(R)" "*S-1-5-18`:(F)" "*S-1-5-32-544`:(F)" | Out-Null
if ($LASTEXITCODE -ne 0) { Write-Fail "Could not restrict ACLs on $EnvFile" }
Write-Ok "Config written to $EnvFile with restricted ACLs"

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

# ── Create scheduled task (startup, SYSTEM account) ───────
Write-Step "Creating scheduled task..."

$Action = New-ScheduledTaskAction `
    -Execute "powershell.exe" `
    -Argument "-NoProfile -ExecutionPolicy RemoteSigned -WindowStyle Hidden -File `"$WrapperPath`"" `
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
    Write-Warn "pi-server may have failed to start."
    Write-Warn "Check: Get-ScheduledTaskInfo -TaskName '$TaskName'"
}

# ── Summary ───────────────────────────────────────────────
$ServerIP = (Invoke-WebRequest -Uri "https://ifconfig.me" -UseBasicParsing -ErrorAction SilentlyContinue).Content.Trim()
if (-not $ServerIP) { $ServerIP = "localhost" }

Write-Host ""
Write-Host "==================================================" -ForegroundColor Green
Write-Host "  pi-server installed system-wide!"
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
Write-Host "  Auth token: stored in $EnvFile" -ForegroundColor Cyan
Write-Host "  Save this token — it is required for all API connections." -ForegroundColor Yellow
Write-Host "==================================================" -ForegroundColor Green
