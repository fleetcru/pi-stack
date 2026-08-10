# Pi Server Tray

A small cross-platform system-tray wrapper that downloads, starts, and manages `pi-server`.

At startup, the tray checks the latest stable GitHub release from `fleetcru/pi-stack`, downloads the matching server binary to `~/.pi/server/bin/`, and runs it with the user's home directory as its working directory. If the update check fails, an already-downloaded server remains usable.

## Menu

- The information section shows download/start status, the downloaded server version, executable path, and stable-release source.
- **Open Admin** opens the server's `/admin` page.
- **Open Logs** opens the combined server log.
- **Open Server Folder** opens `~/.pi/server/bin/`.
- **Download / Update Server** explicitly downloads and installs the latest stable server while it is stopped, with success or failure shown in the menu.
- **Start Server** also downloads or updates the server when needed before launching it.
- **Start / Stop / Restart Server** controls the process started by this tray.
- **Quit** stops the managed server and closes the tray.

If another server is already healthy at the configured URL, the tray reports it as **Running (external)** and does not attempt to stop or restart it.

## Prerequisites

- Go 1.23+
- Internet access on first launch so the latest stable `pi-server` can be downloaded
- Linux only: the AppIndicator/GTK development libraries required by `fyne.io/systray`

Typical Debian/Ubuntu prerequisites:

```bash
sudo apt-get install gcc libgtk-3-dev libayatana-appindicator3-dev
```

## Dependency setup

From this directory, install the reviewed tray dependency:

```bash
go get fyne.io/systray@v1.12.2
```

## Windows install

From PowerShell in this directory:

```powershell
.\install.ps1
```

The script builds and installs `pi-server-tray.exe` to `%LOCALAPPDATA%\PiServer`, creates a startup shortcut, and launches it. On first launch, the tray downloads the latest stable `pi-server.exe` to `%USERPROFILE%\.pi\server\bin`.

If script execution is restricted, run:

```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

Optional install flags:

```powershell
.\install.ps1 -NoStartup       # Do not start automatically at sign-in
.\install.ps1 -NoLaunch        # Install without launching
.\install.ps1 -InstallDir C:\Tools\PiServer
```

To uninstall while preserving server data:

```powershell
.\uninstall.ps1
```

To also remove the server data directory:

```powershell
.\uninstall.ps1 -RemoveData
```

A custom `-InstallDir` used during installation must also be supplied during uninstall.

## GitHub releases

The `Build Pi Server Tray` GitHub Actions workflow publishes Windows and Linux amd64 binaries plus `SHA256SUMS`. It can be started manually from the Actions tab or by pushing a `tray-v*` tag.

## Manual build

```bash
go build -o dist/pi-server-tray .
```

On Windows, suppress the console window:

```powershell
go build -ldflags="-H=windowsgui -s -w" -o dist/pi-server-tray.exe .
```

By default, no server executable needs to be placed beside the tray. The latest stable server is downloaded automatically.

## Run

```bash
./pi-server-tray --server /path/to/pi-server
```

Options:

```text
--server PATH         custom pi-server path; disables automatic downloads
--no-download         disable automatic server downloads
--release-repo REPO   stable release repository (default fleetcru/pi-stack)
--cwd PATH            server working directory (default: user home)
--url URL             server base URL (default http://127.0.0.1:3141)
--log-file PATH       combined server output log
```

Arguments after the tray options are passed directly to `pi-server`:

```bash
./pi-server-tray \
  --url http://127.0.0.1:4141 \
  -- \
  --addr 127.0.0.1:4141 \
  --cwd /path/to/projects
```

Environment variables such as `PI_SERVER_AUTH_TOKEN`, `PI_SERVER_DATA_DIR`, and `PI_SERVER_ALLOWED_ROOTS` are inherited by the server process.

## Platform notes

- **Windows amd64 and Linux amd64:** automatic downloads select the matching release asset.
- **macOS and ARM systems:** automatic server downloads are not available until matching assets are added to the release workflow; use `--server PATH` with a locally-built server.
- **Windows:** the server is launched without a console window. Windows cannot deliver Unix-style `SIGTERM` to the hidden child, so Stop/Quit terminates it directly.
- **macOS/Linux:** Stop/Quit sends `SIGTERM` to the server process group, allowing graceful shutdown.
- Tray icons are embedded at build time: `assets/icon.ico` on Windows and `assets/icon.png` on macOS/Linux. No asset file is required beside the executable.
