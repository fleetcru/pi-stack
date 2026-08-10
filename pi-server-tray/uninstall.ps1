[CmdletBinding(SupportsShouldProcess = $true, ConfirmImpact = "Medium")]
param(
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA "PiServer"),
    [switch]$RemoveData
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$ShortcutPath = Join-Path ([Environment]::GetFolderPath("Startup")) "Pi Server Tray.lnk"
$DataDir = if ($env:PI_SERVER_DATA_DIR) {
    $env:PI_SERVER_DATA_DIR
}
else {
    Join-Path $HOME ".pi\server"
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

    if (Test-Path $InstallDir) {
        Remove-Item -Recurse -Force $InstallDir
        Write-Host "Removed '$InstallDir'."
    }

    if ($RemoveData -and (Test-Path $DataDir)) {
        Remove-Item -Recurse -Force $DataDir
        Write-Host "Removed server data '$DataDir'."
    }

    Write-Host "Pi Server Tray uninstalled successfully." -ForegroundColor Green
    if (-not $RemoveData) {
        Write-Host "Server data was preserved at '$DataDir'. Use -RemoveData to delete it."
    }
}
