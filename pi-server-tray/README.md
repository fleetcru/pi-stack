# Pi Server Tray

A small cross-platform system-tray wrapper that starts and manages `pi-server`.

## Menu

- **Open Admin** opens the server's `/admin` page.
- **Open Logs** opens the combined server log.
- **Start / Stop / Restart Server** controls the process started by this tray.
- **Quit** stops the managed server and closes the tray.

If another server is already healthy at the configured URL, the tray reports it as **Running (external)** and does not attempt to stop or restart it.

## Prerequisites

- Go 1.23+
- A built `pi-server` executable
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

The script builds both `pi-server.exe` and `pi-server-tray.exe`, installs them to `%LOCALAPPDATA%\PiServer`, creates a startup shortcut, and launches the tray.

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

## Manual build

```bash
go build -o dist/pi-server-tray .
```

On Windows, suppress the console window:

```powershell
go build -ldflags="-H=windowsgui -s -w" -o dist/pi-server-tray.exe .
```

Place the tray executable beside `pi-server`/`pi-server.exe`. It will discover the adjacent server automatically. Otherwise provide its path explicitly.

## Run

```bash
./pi-server-tray --server /path/to/pi-server
```

Options:

```text
--server PATH    pi-server executable path or command name
--url URL        server base URL (default http://127.0.0.1:3141)
--log-file PATH  combined server output log
```

Arguments after the tray options are passed directly to `pi-server`:

```bash
./pi-server-tray \
  --server ./pi-server \
  --url http://127.0.0.1:4141 \
  -- \
  --addr 127.0.0.1:4141 \
  --cwd /path/to/projects
```

Environment variables such as `PI_SERVER_AUTH_TOKEN`, `PI_SERVER_DATA_DIR`, and `PI_SERVER_ALLOWED_ROOTS` are inherited by the server process.

## Platform notes

- **Windows:** the server is launched without a console window. Windows cannot deliver Unix-style `SIGTERM` to the hidden child, so Stop/Quit terminates it directly.
- **macOS/Linux:** Stop/Quit sends `SIGTERM` to the server process group, allowing graceful shutdown.
- Tray icons are embedded at build time: `assets/icon.ico` on Windows and `assets/icon.png` on macOS/Linux. No asset file is required beside the executable.
