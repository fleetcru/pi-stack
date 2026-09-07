package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"pi-server/internal/server"

	clog "github.com/charmbracelet/log"
	"github.com/mdp/qrterminal/v3"
	"github.com/muesli/termenv"
)

func main() {
	cfg := server.ConfigFromEnv()

	// ── CLI flags ──────────────────────────────────────────────────────
	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "HTTP listen address")
	flag.StringVar(&cfg.PiBinary, "pi", cfg.PiBinary, "pi executable path")
	flag.StringVar(&cfg.CWD, "cwd", cfg.CWD, "default working directory for pi child processes")
	flag.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "daemon data directory for persisted session registry")
	flag.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", cfg.ShutdownTimeout, "graceful shutdown timeout")
	logFile := flag.String("log-file", "", "write log output to a file (appends; default: stdout)")
	logFormat := flag.String("log-format", "text", "log format: text, json, or logfmt")
	logLevel := flag.String("log-level", "info", "log level: debug, info, warn, error")
	bg := flag.Bool("bg", false, "detach and run in the background (not supported under systemd / Task Scheduler)")
	pairingQR := flag.Bool("pairing-qr", true, "print a Companion pairing QR in an interactive terminal")
	flag.Parse()
	flag.Visit(func(f *flag.Flag) {
		key := map[string]string{
			"addr": "addr", "pi": "piBinary", "cwd": "cwd", "data-dir": "dataDir",
			"shutdown-timeout": "shutdownTimeout", "log-level": "debug",
		}[f.Name]
		if key != "" {
			cfg.ConfigSources[key] = "cli"
		}
	})

	// Validate log format early so we fail fast.
	formatter, err := parseLogFormat(*logFormat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --log-format %q: %v\n", *logFormat, err)
		os.Exit(1)
	}
	level, err := parseLogLevel(*logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --log-level %q: %v\n", *logLevel, err)
		os.Exit(1)
	}
	// Honour PI_SERVER_DEBUG=1 even if --log-level was not set to debug.
	if cfg.LogLevel == slog.LevelDebug && level > clog.DebugLevel && cfg.ConfigSources["debug"] != "cli" {
		level = clog.DebugLevel
	}

	// ── Logger setup ───────────────────────────────────────────────────
	var logOutput io.Writer = os.Stdout
	var logCloser io.Closer
	if *logFile != "" {
		f, err := openLogFile(*logFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot open log file %q: %v\n", *logFile, err)
			os.Exit(1)
		}
		logOutput = f
		logCloser = f
	}

	logger := newLogger(logOutput, formatter, level, *logFile == "" && isTerminal(os.Stdout))
	slog.SetDefault(slog.New(logger))

	if logCloser != nil {
		defer logCloser.Close()
	}

	// ── Validation & startup ───────────────────────────────────────────
	if err := server.ValidateConfig(cfg); err != nil {
		logger.Error("configuration validation failed", "error", err)
		os.Exit(1)
	}
	if cfg.AuthToken != "" {
		if err := writeBridgeConfig(cfg); err != nil {
			logger.Warn("could not update external bridge config", "error", err)
		} else {
			logger.Info("external bridge config updated")
		}
	}

	// The standalone default intentionally trusts the home LAN or Tailscale
	// network. Operators can still set PI_SERVER_AUTH_TOKEN when the network
	// contains devices that should not have full Pi access.
	if cfg.AuthToken == "" && !loopbackAddr(cfg.Addr) {
		logger.Warn("authentication disabled; trusting the private network", "addr", cfg.Addr)
	}

	// Validate before detaching so --bg reports configuration errors to the
	// caller instead of silently starting a child that exits immediately.
	if *bg {
		if err := daemonize(); err != nil {
			logger.Error("failed to start in background", "error", err)
			os.Exit(1)
		}
		logger.Info("pi-server started in background")
		return
	}

	srv := server.New(cfg, slog.New(logger))
	logger.Info("starting pi-server", "addr", cfg.Addr, "pi", cfg.PiBinary, "cwd", cfg.CWD)
	logAccessURLs(logger, cfg.Addr)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	if *pairingQR {
		// Print before serving so startup logs cannot split and corrupt the QR.
		printPairingQR(srv, cfg.Addr, logger)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case sig := <-sigCh:
		logger.Info("shutdown signal received", "signal", sig.String())
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("pi-server stopped", "time", time.Now().Format(time.RFC3339))
}

// ── Logger construction ────────────────────────────────────────────────

// newLogger builds a charmbracelet/log Logger backed by slog, writing to w.
func newLogger(w io.Writer, formatter clog.Formatter, level clog.Level, tty bool) *clog.Logger {
	opts := clog.Options{
		Formatter:       formatter,
		Level:           level,
		ReportTimestamp: true,
	}
	l := clog.NewWithOptions(w, opts)
	// Disable ANSI styling when output is not a terminal or when writing to
	// a file — keeps log files machine-parseable.
	if !tty {
		l.SetColorProfile(termenv.Ascii)
	}
	return l
}

func parseLogFormat(s string) (clog.Formatter, error) {
	switch strings.ToLower(s) {
	case "text", "":
		return clog.TextFormatter, nil
	case "json":
		return clog.JSONFormatter, nil
	case "logfmt":
		return clog.LogfmtFormatter, nil
	default:
		return 0, fmt.Errorf("unknown format %q (valid: text, json, logfmt)", s)
	}
}

func parseLogLevel(s string) (clog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return clog.DebugLevel, nil
	case "info", "":
		return clog.InfoLevel, nil
	case "warn", "warning":
		return clog.WarnLevel, nil
	case "error":
		return clog.ErrorLevel, nil
	default:
		return 0, fmt.Errorf("unknown level %q (valid: debug, info, warn, error)", s)
	}
}

// isTerminal reports whether fd is connected to a terminal.
type bridgeConfig struct {
	RelayURL   string `json:"relayUrl"`
	RelayToken string `json:"relayToken"`
}

func writeBridgeConfig(cfg server.Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	addr := bridgeAddress(cfg.Addr)
	if override := strings.TrimSpace(os.Getenv("PI_SERVER_BRIDGE_URL")); override != "" {
		addr = strings.TrimRight(override, "/")
	}
	path := filepath.Join(home, ".pi", "agent", "bridge-config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(bridgeConfig{RelayURL: addr, RelayToken: cfg.AuthToken}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func bridgeAddress(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		port = "3142"
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = firstLANIPv4()
	}
	return "http://" + net.JoinHostPort(host, port)
}

func printPairingQR(srv *server.Server, addr string, logger *clog.Logger) {
	if !isTerminal(os.Stdout) {
		return
	}
	token, err := srv.CreatePairingCredential("Terminal pairing")
	if err != nil {
		logger.Warn("could not create terminal pairing credential", "error", err)
		return
	}
	label, url := preferredPairingURL(addr)
	payload, err := json.Marshal(map[string]string{"url": url, "token": token, "name": "Pi Server"})
	if err != nil {
		logger.Warn("could not encode terminal pairing payload", "error", err)
		return
	}
	fmt.Fprintf(os.Stdout, "\n  Pair Companion over %s\n  %s\n\n", label, url)
	qrterminal.GenerateHalfBlock(string(payload), qrterminal.M, os.Stdout)
	fmt.Fprintln(os.Stdout, "  In Companion, open Settings and tap Scan pairing QR.")
	fmt.Fprintln(os.Stdout)
}

func preferredPairingURL(addr string) (string, string) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		port = "3142"
	}
	if tailscale := firstTailscaleIPv4(); tailscale != "" {
		return "Tailscale", "http://" + net.JoinHostPort(tailscale, port)
	}
	if lan := firstLANIPv4(); lan != "127.0.0.1" {
		return "home network", "http://" + net.JoinHostPort(lan, port)
	}
	return "this computer", "http://127.0.0.1:" + port
}

func logAccessURLs(logger *clog.Logger, addr string) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		port = "3142"
	}
	logger.Info("local access", "url", "http://127.0.0.1:"+port, "admin", "http://127.0.0.1:"+port+"/admin/")
	if lan := firstLANIPv4(); lan != "127.0.0.1" {
		logger.Info("home network access", "url", "http://"+net.JoinHostPort(lan, port))
	}
	if tailscale := firstTailscaleIPv4(); tailscale != "" {
		logger.Info("Tailscale access", "url", "http://"+net.JoinHostPort(tailscale, port))
	}
}

func firstTailscaleIPv4() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err == nil && isTailscaleIPv4(ip) {
				return ip.String()
			}
		}
	}
	return ""
}

func isTailscaleIPv4(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127
}

func firstLANIPv4() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}

	// Only choose RFC1918 addresses. Starlink and other ISPs can expose a
	// carrier-grade NAT address in 100.64.0.0/10, which is not reachable by a
	// phone on the home's Wi-Fi and must never be written into bridge-config.
	// Prefer normal Wi-Fi/Ethernet adapters when several private networks exist.
	bestIP := ""
	bestScore := -1
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		score := 1
		name := strings.ToLower(iface.Name)
		if strings.Contains(name, "wi-fi") || strings.Contains(name, "wifi") ||
			strings.Contains(name, "wireless") || strings.Contains(name, "ethernet") {
			score = 3
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil || !isPrivateLANIPv4(ip) {
				continue
			}
			if score > bestScore {
				bestIP = ip.String()
				bestScore = score
			}
		}
	}
	if bestIP != "" {
		return bestIP
	}
	return "127.0.0.1"
}

func isPrivateLANIPv4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		return false
	}
	return ip[0] == 10 ||
		(ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) ||
		(ip[0] == 192 && ip[1] == 168)
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// openLogFile opens (or creates) a file for appending log output.
func openLogFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// ── Background / daemon mode ───────────────────────────────────────────

// filteredArgs returns a copy of args with --bg and its value (if any) removed.
func filteredArgs(args []string) []string {
	out := make([]string, 0, len(args))
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		if a == "--bg" {
			continue
		}
		if strings.HasPrefix(a, "--bg=") {
			continue
		}
		out = append(out, a)
	}
	return out
}

// stderrLogPath returns the path used for background-mode stderr.
func stderrLogPath() string {
	dataDir := os.Getenv("PI_SERVER_DATA_DIR")
	if dataDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dataDir = filepath.Join(home, ".pi", "server")
		} else {
			dataDir = ".pi-server"
		}
	}
	return filepath.Join(dataDir, "stderr.log")
}

// ── Helpers ────────────────────────────────────────────────────────────

func loopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return false // ":3142" listens on all interfaces
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
