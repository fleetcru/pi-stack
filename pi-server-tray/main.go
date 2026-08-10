package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/systray"
)

type config struct {
	serverPath   string
	serverURL    string
	logPath      string
	workDir      string
	releaseRepo  string
	autoDownload bool
	serverArgs   []string
}

type app struct {
	cfg config

	mu       sync.Mutex
	startMu  sync.Mutex
	cmd      *exec.Cmd
	cmdDone  chan struct{}
	logFile  *os.File
	stopping bool
	done     chan struct{}
	doneOnce sync.Once
	exiting  atomic.Bool
	busy     atomic.Bool
	ctx      context.Context
	cancel   context.CancelFunc

	statusItem    *systray.MenuItem
	versionItem   *systray.MenuItem
	pathItem      *systray.MenuItem
	downloadInfo  *systray.MenuItem
	downloadItem  *systray.MenuItem
	openAdminItem *systray.MenuItem
	startItem     *systray.MenuItem
	stopItem      *systray.MenuItem
	restartItem   *systray.MenuItem
}

func main() {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	ctx, cancel := context.WithCancel(context.Background())
	a := &app{cfg: cfg, done: make(chan struct{}), ctx: ctx, cancel: cancel}
	systray.Run(a.onReady, a.onExit)
}

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("pi-server-tray", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	defaultLog, err := defaultLogPath()
	if err != nil {
		return config{}, err
	}
	defaultServer, err := managedServerPath()
	if err != nil {
		return config{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return config{}, fmt.Errorf("find home directory: %w", err)
	}
	cfg := config{autoDownload: true}
	noDownload := false
	fs.StringVar(&cfg.serverPath, "server", defaultServer, "custom pi-server executable path (disables automatic downloads)")
	fs.StringVar(&cfg.serverURL, "url", "http://127.0.0.1:3141", "pi-server base URL")
	fs.StringVar(&cfg.logPath, "log-file", defaultLog, "combined pi-server stdout/stderr log")
	fs.StringVar(&cfg.workDir, "cwd", home, "working directory used to run pi-server")
	fs.StringVar(&cfg.releaseRepo, "release-repo", defaultReleaseRepo, "GitHub owner/repository used for stable server releases")
	fs.BoolVar(&noDownload, "no-download", false, "disable automatic server downloads")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	cfg.serverURL = strings.TrimRight(cfg.serverURL, "/")
	if cfg.serverURL == "" {
		return config{}, errors.New("--url cannot be empty")
	}
	if cfg.workDir == "" {
		return config{}, errors.New("--cwd cannot be empty")
	}
	serverOverridden := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "server" {
			serverOverridden = true
		}
	})
	cfg.autoDownload = !noDownload && !serverOverridden
	cfg.serverArgs = fs.Args()
	return cfg, nil
}

func defaultLogPath() (string, error) {
	if dir := os.Getenv("PI_SERVER_DATA_DIR"); dir != "" {
		return filepath.Join(dir, "tray-server.log"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".pi", "server", "tray-server.log"), nil
}

func (a *app) onReady() {
	systray.SetIcon(trayIcon)
	systray.SetTitle("Pi Server")
	systray.SetTooltip("Pi Server")

	a.statusItem = systray.AddMenuItem("Status: checking…", "Current pi-server status")
	a.statusItem.Disable()
	a.versionItem = systray.AddMenuItem("Server version: checking…", "Downloaded pi-server release")
	a.versionItem.Disable()
	a.pathItem = systray.AddMenuItem("Server: "+a.cfg.serverPath, "Downloaded server executable")
	a.pathItem.Disable()
	updateSource := systray.AddMenuItem("Updates: latest stable release", "Source: github.com/"+a.cfg.releaseRepo)
	updateSource.Disable()
	a.downloadInfo = systray.AddMenuItem("Download: ready", "Server download result")
	a.downloadInfo.Disable()
	a.updateServerInfo()
	systray.AddSeparator()

	a.openAdminItem = systray.AddMenuItem("Open Admin", "Open the pi-server admin page")
	openLogs := systray.AddMenuItem("Open Logs", "Open the pi-server log file")
	openServerFolder := systray.AddMenuItem("Open Server Folder", "Open the downloaded server location")
	a.downloadItem = systray.AddMenuItem("Download / Update Server", "Download and install the latest stable pi-server release")
	systray.AddSeparator()

	a.startItem = systray.AddMenuItem("Start Server", "Start the installed pi-server")
	a.stopItem = systray.AddMenuItem("Stop Server", "Stop the pi-server process started by this tray")
	a.restartItem = systray.AddMenuItem("Restart Server", "Restart the pi-server process")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "Stop the managed server and exit")

	go a.handleMenu(openLogs, openServerFolder, quit)
	go a.monitor()
	go func() {
		if fileExists(a.cfg.serverPath) && !a.healthy() {
			if err := a.start(); err != nil {
				a.setStatus("Start failed: " + err.Error())
			}
		}
	}()
}

func (a *app) handleMenu(openLogs, openServerFolder, quit *systray.MenuItem) {
	for {
		select {
		case <-a.openAdminItem.ClickedCh:
			_ = openTarget(a.cfg.serverURL + "/admin")
		case <-openLogs.ClickedCh:
			_ = ensureFile(a.cfg.logPath)
			_ = openTarget(a.cfg.logPath)
		case <-openServerFolder.ClickedCh:
			_ = os.MkdirAll(filepath.Dir(a.cfg.serverPath), 0o755)
			_ = openTarget(filepath.Dir(a.cfg.serverPath))
		case <-a.downloadItem.ClickedCh:
			go func() {
				if err := a.downloadServer(); err != nil {
					a.setDownloadInfo("failed: " + err.Error())
					a.setStatus("Server download failed")
				}
			}()
		case <-a.startItem.ClickedCh:
			if err := a.start(); err != nil {
				a.setStatus("Start failed: " + err.Error())
			}
		case <-a.stopItem.ClickedCh:
			if err := a.stop(); err != nil {
				a.setStatus("Stop failed: " + err.Error())
			}
		case <-a.restartItem.ClickedCh:
			if err := a.restart(); err != nil {
				a.setStatus("Restart failed: " + err.Error())
			}
		case <-quit.ClickedCh:
			systray.Quit()
		case <-a.done:
			return
		}
	}
}

func (a *app) start() error {
	a.startMu.Lock()
	defer a.startMu.Unlock()
	a.busy.Store(true)
	defer a.busy.Store(false)

	a.mu.Lock()
	alreadyRunning := a.cmd != nil
	a.mu.Unlock()
	if alreadyRunning {
		return nil
	}

	if !fileExists(a.cfg.serverPath) {
		return errors.New("server is not installed; use Download / Update Server first")
	}
	a.setStatus("Starting server…")
	if a.exiting.Load() {
		return errors.New("tray is exiting")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cmd != nil {
		return nil
	}
	if a.healthy() {
		return errors.New("another pi-server is already listening; it is not managed by this tray")
	}
	if err := os.MkdirAll(filepath.Dir(a.cfg.logPath), 0o755); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}
	logFile, err := os.OpenFile(a.cfg.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	cmd := exec.Command(a.cfg.serverPath, a.cfg.serverArgs...)
	cmd.Dir = a.cfg.workDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	configureProcess(cmd)
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start %q: %w", a.cfg.serverPath, err)
	}
	done := make(chan struct{})
	a.cmd = cmd
	a.cmdDone = done
	a.logFile = logFile
	a.stopping = false
	go a.wait(cmd, logFile, done)
	return nil
}

func (a *app) downloadServer() error {
	a.startMu.Lock()
	defer a.startMu.Unlock()
	a.busy.Store(true)
	defer a.busy.Store(false)

	a.mu.Lock()
	running := a.cmd != nil
	a.mu.Unlock()
	if running || a.healthy() {
		return errors.New("stop the server before downloading an update")
	}
	if !a.cfg.autoDownload {
		return errors.New("automatic downloads are disabled by the tray configuration")
	}
	version, err := a.ensureServerDownloaded()
	if err != nil {
		return err
	}
	a.setDownloadInfo("installed " + version)
	a.setStatus("Server " + version + " downloaded")
	return nil
}

func (a *app) ensureServerDownloaded() (string, error) {
	a.setStatus("Downloading latest stable server release…")
	a.setDownloadInfo("checking GitHub releases…")
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	version, err := newServerDownloader(a.cfg.releaseRepo).ensureLatest(ctx, a.cfg.serverPath)
	cancel()
	if err != nil {
		return "", err
	}
	a.updateServerInfo()
	a.setDownloadInfo("installed " + version)
	return version, nil
}

func (a *app) wait(cmd *exec.Cmd, logFile *os.File, done chan struct{}) {
	err := cmd.Wait()
	_ = logFile.Close()
	close(done)
	a.mu.Lock()
	if a.cmd == cmd {
		a.cmd = nil
		a.cmdDone = nil
		a.logFile = nil
	}
	stopping := a.stopping
	a.stopping = false
	a.mu.Unlock()
	if err != nil && !stopping {
		a.setStatus("Server exited: " + err.Error())
	}
}

func (a *app) stop() error {
	a.mu.Lock()
	cmd := a.cmd
	done := a.cmdDone
	if cmd == nil || cmd.Process == nil {
		a.mu.Unlock()
		return nil
	}
	a.stopping = true
	a.mu.Unlock()

	if err := stopProcess(cmd.Process); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	select {
	case <-done:
		return nil
	case <-time.After(10 * time.Second):
	}
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	select {
	case <-done:
		return nil
	case <-time.After(2 * time.Second):
		return errors.New("server process did not exit after being killed")
	}
}

func (a *app) restart() error {
	if err := a.stop(); err != nil {
		return err
	}
	return a.start()
}

func (a *app) healthy() bool {
	client := http.Client{Timeout: 750 * time.Millisecond}
	resp, err := client.Get(a.cfg.serverURL + "/healthz")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func (a *app) monitor() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		a.updateStatus()
		select {
		case <-ticker.C:
		case <-a.done:
			return
		}
	}
}

func (a *app) updateStatus() {
	if a.busy.Load() {
		return
	}
	healthy := a.healthy()
	a.mu.Lock()
	managed := a.cmd != nil
	a.mu.Unlock()

	switch {
	case healthy && managed:
		a.setStatus("Running (managed)")
		a.openAdminItem.Enable()
		a.downloadItem.Disable()
		a.startItem.Disable()
		a.stopItem.Enable()
		a.restartItem.Enable()
	case healthy:
		a.setStatus("Running (external)")
		a.openAdminItem.Enable()
		a.downloadItem.Disable()
		a.startItem.Disable()
		a.stopItem.Disable()
		a.restartItem.Disable()
	default:
		a.openAdminItem.Disable()
		if a.cfg.autoDownload {
			a.downloadItem.Enable()
		} else {
			a.downloadItem.Disable()
		}
		if fileExists(a.cfg.serverPath) {
			a.setStatus("Stopped")
			a.startItem.Enable()
		} else {
			a.setStatus("Server not installed")
			a.startItem.Disable()
		}
		a.stopItem.Disable()
		a.restartItem.Disable()
	}
}

func (a *app) updateServerInfo() {
	if a.versionItem == nil {
		return
	}
	version := "not downloaded"
	if cached, err := cachedServerVersion(a.cfg.serverPath); err == nil && cached != "" {
		version = cached
	} else if fileExists(a.cfg.serverPath) {
		version = "custom or unknown"
	}
	a.versionItem.SetTitle("Server version: " + version)
}

func (a *app) setDownloadInfo(info string) {
	if a.exiting.Load() || a.downloadInfo == nil {
		return
	}
	a.downloadInfo.SetTitle("Download: " + info)
}

func (a *app) setStatus(status string) {
	if a.exiting.Load() {
		return
	}
	if a.statusItem != nil {
		a.statusItem.SetTitle("Status: " + status)
	}
	systray.SetTooltip("Pi Server — " + status)
}

func (a *app) onExit() {
	a.exiting.Store(true)
	a.cancel()
	a.doneOnce.Do(func() { close(a.done) })
	ctx, cancel := context.WithTimeout(context.Background(), 11*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		// Wait for any canceled download/start attempt before checking for a child.
		a.startMu.Lock()
		_ = a.stop()
		a.startMu.Unlock()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func ensureFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}
