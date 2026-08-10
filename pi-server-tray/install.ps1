[CmdletBinding()]
param(
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA "PiServer"),
    [switch]$NoStartup,
    [switch]$NoLaunch
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$TrayDir = $PSScriptRoot
$DistDir = Join-Path $TrayDir "dist"
$TrayOutput = Join-Path $DistDir "pi-server-tray.exe"
$ShortcutPath = Join-Path ([Environment]::GetFolderPath("Startup")) "Pi Server Tray.lnk"

function Invoke-GoBuild {
    param(
        [Parameter(Mandatory = $true)][string]$WorkingDirectory,
        [Parameter(Mandatory = $true)][string[]]$Arguments
    )

    Push-Location $WorkingDirectory
    try {
        & go @Arguments
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed with exit code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }
}

function Stop-InstalledProcess {
    param([Parameter(Mandatory = $true)][string]$ExecutablePath)

    $ExpectedPath = [IO.Path]::GetFullPath($ExecutablePath)
    Get-CimInstance Win32_Process -ErrorAction SilentlyContinue |
        Where-Object {
            $_.ExecutablePath -and
            [string]::Equals(
                [IO.Path]::GetFullPath($_.ExecutablePath),
                $ExpectedPath,
                [StringComparison]::OrdinalIgnoreCase
            )
        } |
        ForEach-Object {
            Write-Host "Stopping $($_.Name) (PID $($_.ProcessId))..."
            Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue
        }
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go was not found on PATH. Install Go 1.23 or newer and try again."
}
Write-Host "Building pi-server tray..."
New-Item -ItemType Directory -Force -Path $DistDir | Out-Null
Invoke-GoBuild -WorkingDirectory $TrayDir -Arguments @(
    "build",
    "-trimpath",
    "-ldflags=-H=windowsgui -s -w",
    "-o", $TrayOutput,
    "."
)

$InstalledTray = Join-Path $InstallDir "pi-server-tray.exe"
$ManagedServer = Join-Path $HOME ".pi\server\bin\pi-server.exe"
Stop-InstalledProcess -ExecutablePath $InstalledTray
Stop-InstalledProcess -ExecutablePath $ManagedServer
Start-Sleep -Milliseconds 300

Write-Host "Installing to '$InstallDir'..."
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item -Force $TrayOutput $InstalledTray

if (-not $NoStartup) {
    Write-Host "Creating startup shortcut..."
    $Shell = New-Object -ComObject WScript.Shell
    $Shortcut = $Shell.CreateShortcut($ShortcutPath)
    $Shortcut.TargetPath = $InstalledTray
    $Shortcut.WorkingDirectory = $InstallDir
    $Shortcut.Description = "Pi Server system tray"
    $Shortcut.Save()
}

if (-not $NoLaunch) {
    Write-Host "Starting Pi Server Tray..."
    Start-Process -FilePath $InstalledTray -WorkingDirectory $InstallDir
}

Write-Host ""
Write-Host "Pi Server Tray installed successfully." -ForegroundColor Green
Write-Host "Install directory: $InstallDir"
if ($NoStartup) {
    Write-Host "Start at sign-in: disabled"
}
else {
    Write-Host "Start at sign-in: enabled"
}
