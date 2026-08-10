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
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/systray"
)

type config struct {
	serverPath string
	serverURL  string
	logPath    string
	serverArgs []string
}

type app struct {
	cfg config

	mu       sync.Mutex
	cmd      *exec.Cmd
	cmdDone  chan struct{}
	logFile  *os.File
	stopping bool
	done     chan struct{}
	doneOnce sync.Once
	exiting  atomic.Bool

	statusItem  *systray.MenuItem
	startItem   *systray.MenuItem
	stopItem    *systray.MenuItem
	restartItem *systray.MenuItem
}

func main() {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	a := &app{cfg: cfg, done: make(chan struct{})}
	systray.Run(a.onReady, a.onExit)
}

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("pi-server-tray", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	defaultLog, err := defaultLogPath()
	if err != nil {
		return config{}, err
	}
	cfg := config{}
	fs.StringVar(&cfg.serverPath, "server", defaultServerPath(), "path or command name for the pi-server executable")
	fs.StringVar(&cfg.serverURL, "url", "http://127.0.0.1:3141", "pi-server base URL")
	fs.StringVar(&cfg.logPath, "log-file", defaultLog, "combined pi-server stdout/stderr log")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	cfg.serverURL = strings.TrimRight(cfg.serverURL, "/")
	if cfg.serverURL == "" {
		return config{}, errors.New("--url cannot be empty")
	}
	cfg.serverArgs = fs.Args()
	return cfg, nil
}

func defaultServerPath() string {
	name := "pi-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return name
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
	systray.AddSeparator()

	openAdmin := systray.AddMenuItem("Open Admin", "Open the pi-server admin page")
	openLogs := systray.AddMenuItem("Open Logs", "Open the pi-server log file")
	systray.AddSeparator()

	a.startItem = systray.AddMenuItem("Start Server", "Start pi-server")
	a.stopItem = systray.AddMenuItem("Stop Server", "Stop the pi-server process started by this tray")
	a.restartItem = systray.AddMenuItem("Restart Server", "Restart the pi-server process")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "Stop the managed server and exit")

	go a.handleMenu(openAdmin, openLogs, quit)
	go a.monitor()
	go func() {
		if !a.healthy() {
			if err := a.start(); err != nil {
				a.setStatus("Start failed: " + err.Error())
			}
		}
	}()
}

func (a *app) handleMenu(openAdmin, openLogs, quit *systray.MenuItem) {
	for {
		select {
		case <-openAdmin.ClickedCh:
			_ = openTarget(a.cfg.serverURL + "/admin")
		case <-openLogs.ClickedCh:
			_ = ensureFile(a.cfg.logPath)
			_ = openTarget(a.cfg.logPath)
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
	healthy := a.healthy()
	a.mu.Lock()
	managed := a.cmd != nil
	a.mu.Unlock()

	switch {
	case healthy && managed:
		a.setStatus("Running (managed)")
		a.startItem.Disable()
		a.stopItem.Enable()
		a.restartItem.Enable()
	case healthy:
		a.setStatus("Running (external)")
		a.startItem.Disable()
		a.stopItem.Disable()
		a.restartItem.Disable()
	default:
		a.setStatus("Stopped")
		a.startItem.Enable()
		a.stopItem.Disable()
		a.restartItem.Disable()
	}
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
	a.doneOnce.Do(func() { close(a.done) })
	ctx, cancel := context.WithTimeout(context.Background(), 11*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		_ = a.stop()
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
