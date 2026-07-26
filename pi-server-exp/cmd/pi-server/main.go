package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pi-server/internal/server"
)

func main() {
	cfg := server.ConfigFromEnv()
	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "HTTP listen address")
	flag.StringVar(&cfg.PiBinary, "pi", cfg.PiBinary, "pi executable path")
	flag.StringVar(&cfg.CWD, "cwd", cfg.CWD, "default working directory for pi child processes")
	flag.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "daemon data directory for persisted session registry")
	flag.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", cfg.ShutdownTimeout, "graceful shutdown timeout")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

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

	srv := server.New(cfg, logger)
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
