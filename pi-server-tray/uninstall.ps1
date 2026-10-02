[CmdletBinding(SupportsShouldProcess = $true, ConfirmImpact = "Medium")]
param(
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA "PiServer"),
    [switch]$RemoveData
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'This uninstaller supports Windows only.' }
$InstallDir = [IO.Path]::GetFullPath($InstallDir)

$ShortcutPath = Join-Path ([Environment]::GetFolderPath("Startup")) "Pi Server Tray.lnk"
$DataDir = if ($env:PI_SERVER_DATA_DIR) {
    $env:PI_SERVER_DATA_DIR
}
else {
    Join-Path $HOME ".pi\server"
}

$DataDir = [IO.Path]::GetFullPath($DataDir)
if ($RemoveData) {
    foreach ($protected in @([IO.Path]::GetPathRoot($DataDir), $HOME, (Join-Path $HOME '.pi'), $env:USERPROFILE, $env:LOCALAPPDATA)) {
        if ([string]::Equals($DataDir.TrimEnd('\'), $protected.TrimEnd('\'), [StringComparison]::OrdinalIgnoreCase)) {
            throw "Refusing to recursively delete protected directory: $DataDir"
        }
    }
    if ((Test-Path -LiteralPath $DataDir) -and ((Get-Item -LiteralPath $DataDir).Attributes -band [IO.FileAttributes]::ReparsePoint)) {
        throw 'Refusing to delete server data through a junction or symbolic link.'
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

if ($PSCmdlet.ShouldProcess($InstallDir, "Uninstall Pi Server Tray")) {
    Stop-InstalledProcess -ExecutablePath (Join-Path $InstallDir "pi-server-tray.exe")
    Stop-InstalledProcess -ExecutablePath (Join-Path $DataDir "bin\pi-server.exe")
    Start-Sleep -Milliseconds 300

    if (Test-Path $ShortcutPath) {
        Remove-Item -Force $ShortcutPath
        Write-Host "Removed startup shortcut."
    }

    $tray = Join-Path $InstallDir 'pi-server-tray.exe'
    if (Test-Path -LiteralPath $tray) { Remove-Item -LiteralPath $tray -Force }
    if ((Test-Path -LiteralPath $InstallDir) -and -not (Get-ChildItem -LiteralPath $InstallDir -Force | Select-Object -First 1)) {
        Remove-Item -LiteralPath $InstallDir
    }
    Write-Host 'Removed the tray executable. Other files in the install directory were preserved.'

    if ($RemoveData -and (Test-Path $DataDir)) {
        Remove-Item -LiteralPath $DataDir -Recurse -Force
        Write-Host "Removed server data '$DataDir'."
    }

    Write-Host "Pi Server Tray uninstalled successfully." -ForegroundColor Green
    if (-not $RemoveData) {
        Write-Host "Server data was preserved at '$DataDir'. Use -RemoveData to delete it."
    }
}
