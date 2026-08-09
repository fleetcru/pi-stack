package main

import (
	"context"
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

	// Binding a non-loopback address without an auth token exposes full session
	// control (prompt execution, file reads, relay commands) to the LAN — and,
	// via the permissive WebSocket origin check, to any website the operator
	// visits. Refuse to start unless explicitly overridden.
	if cfg.AuthToken == "" && !loopbackAddr(cfg.Addr) && os.Getenv("PI_SERVER_ALLOW_INSECURE") == "" {
		logger.Error("refusing to bind a non-loopback address without PI_SERVER_AUTH_TOKEN",
			"addr", cfg.Addr,
			"hint", "set PI_SERVER_AUTH_TOKEN, or set PI_SERVER_ALLOW_INSECURE=1 to override")
		os.Exit(1)
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
	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting pi-server", "addr", cfg.Addr, "pi", cfg.PiBinary, "cwd", cfg.CWD)
		errCh <- srv.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

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
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// openLogFile opens (or creates) a file for appending log output.
func openLogFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
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
		return false // ":3141" listens on all interfaces
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
