[CmdletBinding()]
param(
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA "PiServer"),
    [switch]$NoStartup,
    [switch]$NoLaunch
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'This installer supports Windows only.' }
$InstallDir = [IO.Path]::GetFullPath($InstallDir)

$TrayDir = $PSScriptRoot
$DistDir = Join-Path $TrayDir "dist"
$TrayOutput = Join-Path $DistDir "pi-server-tray-$([guid]::NewGuid().ToString('N')).exe"
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
            Stop-Process -Id $_.ProcessId -Force -ErrorAction Stop
            Wait-Process -Id $_.ProcessId -Timeout 15 -ErrorAction SilentlyContinue
        }
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go was not found on PATH. Install Go 1.23 or newer and try again."
}
Write-Host "Building pi-server tray..."
New-Item -ItemType Directory -Force -Path $DistDir | Out-Null
try {
Invoke-GoBuild -WorkingDirectory $TrayDir -Arguments @(
    "build",
    "-trimpath",
    "-ldflags=-H=windowsgui -s -w",
    "-o", $TrayOutput,
    "."
)

$InstalledTray = Join-Path $InstallDir "pi-server-tray.exe"
$ManagedServer = Join-Path $HOME ".pi\server\bin\pi-server.exe"
Write-Host "Installing to '$InstallDir'..."
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$StagedTray = Join-Path $InstallDir ".tray-$([guid]::NewGuid().ToString('N')).exe"
$BackupTray = "$StagedTray.previous"
Copy-Item -LiteralPath $TrayOutput -Destination $StagedTray
$ShortcutBackup = "$BackupTray.lnk"
if (Test-Path -LiteralPath $ShortcutPath) { Copy-Item -LiteralPath $ShortcutPath -Destination $ShortcutBackup }
$KeepBackup = $true
try {
  Stop-InstalledProcess -ExecutablePath $InstalledTray
  Stop-InstalledProcess -ExecutablePath $ManagedServer
  Start-Sleep -Milliseconds 300
  if (Test-Path -LiteralPath $InstalledTray) { [IO.File]::Replace($StagedTray, $InstalledTray, $BackupTray) }
  else { [IO.File]::Move($StagedTray, $InstalledTray) }

if (-not $NoStartup) {
    Write-Host "Creating startup shortcut..."
    $Shell = New-Object -ComObject WScript.Shell
    $Shortcut = $Shell.CreateShortcut($ShortcutPath)
    $Shortcut.TargetPath = $InstalledTray
    $Shortcut.WorkingDirectory = $InstallDir
    $Shortcut.Description = "Pi Server system tray"
    $Shortcut.Save()
    [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($Shortcut)
    [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($Shell)
} elseif (Test-Path -LiteralPath $ShortcutPath) {
    Remove-Item -LiteralPath $ShortcutPath -Force
}

if (-not $NoLaunch) {
    Write-Host "Starting Pi Server Tray..."
    $process = Start-Process -FilePath $InstalledTray -WorkingDirectory $InstallDir -PassThru
    Start-Sleep -Seconds 1
    if ($process.HasExited) { throw "Tray exited during startup with code $($process.ExitCode)." }
}
$KeepBackup = $false
} catch {
    if (Test-Path -LiteralPath $BackupTray) {
        Stop-InstalledProcess -ExecutablePath $InstalledTray
        Copy-Item -LiteralPath $BackupTray -Destination $InstalledTray -Force
    }
    if (Test-Path -LiteralPath $ShortcutBackup) { Copy-Item -LiteralPath $ShortcutBackup -Destination $ShortcutPath -Force }
    else { Remove-Item -LiteralPath $ShortcutPath -Force -ErrorAction SilentlyContinue }
    $KeepBackup = $false
    throw
} finally {
    Remove-Item -LiteralPath $StagedTray -Force -ErrorAction SilentlyContinue
    if ($KeepBackup) { Write-Warning "Recovery files retained at $BackupTray and $ShortcutBackup." }
    else { Remove-Item -LiteralPath $BackupTray, $ShortcutBackup -Force -ErrorAction SilentlyContinue }
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
} finally {
    Remove-Item -LiteralPath $TrayOutput -Force -ErrorAction SilentlyContinue
}
