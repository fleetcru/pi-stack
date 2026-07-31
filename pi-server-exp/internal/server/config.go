package server

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr               string
	PiBinary           string
	Extensions         []string
	CWD                string
	DataDir            string
	AllowedOrigins     []string
	AllowedRoots       []string
	AllowedWorkerHosts []string
	AuthToken          string
	ShutdownTimeout    time.Duration
	RequestTimeout     time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
	MaxSessions        int
	RestartMax         int
	RestartBackoff     time.Duration
	EventHistoryMax    int
	EventHistoryBytes  int
	MaxWatches         int
	LogLevel           slog.Level
}

// ValidateConfig checks local prerequisites before the daemon starts accepting
// sessions. It deliberately avoids launching Pi, since Pi may load user-specific
// extensions and provider configuration at session start.
func ValidateConfig(cfg Config) error {
	if _, err := exec.LookPath(cfg.PiBinary); err != nil {
		return fmt.Errorf("pi binary %q is not available in PATH: %w", cfg.PiBinary, err)
	}

	cwdInfo, err := os.Stat(cfg.CWD)
	if err != nil {
		return fmt.Errorf("server cwd %q is unavailable: %w", cfg.CWD, err)
	}
	if !cwdInfo.IsDir() {
		return fmt.Errorf("server cwd %q is not a directory", cfg.CWD)
	}

	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("server data directory %q is unavailable: %w", cfg.DataDir, err)
	}
	testFile, err := os.CreateTemp(filepath.Clean(cfg.DataDir), ".pi-server-write-test-*")
	if err != nil {
		return fmt.Errorf("server data directory %q is not writable: %w", cfg.DataDir, err)
	}
	name := testFile.Name()
	if err := testFile.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("server data directory %q write test failed: %w", cfg.DataDir, err)
	}
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("server data directory %q cleanup failed: %w", cfg.DataDir, err)
	}
	return nil
}

func ConfigFromEnv() Config {
	cwd, _ := os.Getwd()
	cfg := Config{
		Addr:               env("PI_SERVER_ADDR", "127.0.0.1:3141"),
		PiBinary:           env("PI_SERVER_PI_BINARY", "pi"),
		Extensions:         envList("PI_SERVER_PI_EXTENSIONS"),
		CWD:                env("PI_SERVER_CWD", cwd),
		DataDir:            env("PI_SERVER_DATA_DIR", defaultDataDir()),
		AuthToken:          env("PI_SERVER_AUTH_TOKEN", ""),
		AllowedOrigins:     envList("PI_SERVER_ALLOWED_ORIGINS"),
		AllowedRoots:       envList("PI_SERVER_ALLOWED_ROOTS"),
		AllowedWorkerHosts: envList("PI_SERVER_ALLOWED_WORKER_HOSTS"),
		ShutdownTimeout:    envDuration("PI_SERVER_SHUTDOWN_TIMEOUT", 10*time.Second),
		RequestTimeout:     envDuration("PI_SERVER_REQUEST_TIMEOUT", 30*time.Second),
		ReadTimeout:        envDuration("PI_SERVER_READ_TIMEOUT", 30*time.Second),
		WriteTimeout:       envDuration("PI_SERVER_WRITE_TIMEOUT", 60*time.Second),
		IdleTimeout:        envDuration("PI_SERVER_IDLE_TIMEOUT", 120*time.Second),
		MaxSessions:        envInt("PI_SERVER_MAX_SESSIONS", 8),
		RestartMax:         envInt("PI_SERVER_RESTART_MAX", 5),
		RestartBackoff:     envDuration("PI_SERVER_RESTART_BACKOFF", time.Second),
		EventHistoryMax:    envInt("PI_SERVER_EVENT_HISTORY_MAX", 100),
		EventHistoryBytes:  envInt("PI_SERVER_EVENT_HISTORY_BYTES", 2<<20),
		MaxWatches:         envInt("PI_SERVER_MAX_WATCHES", 2048),
		LogLevel:           slog.LevelInfo,
	}
	if os.Getenv("PI_SERVER_DEBUG") == "1" || os.Getenv("PI_SERVER_DEBUG") == "true" {
		cfg.LogLevel = slog.LevelDebug
	}
	return cfg
}

func defaultDataDir() string {
	if v := os.Getenv("PI_CODING_AGENT_DIR"); v != "" {
		return v + string(os.PathSeparator) + "server"
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home + string(os.PathSeparator) + ".pi" + string(os.PathSeparator) + "server"
	}
	return ".pi-server"
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if value, err := strconv.Atoi(os.Getenv(key)); err == nil && value > 0 {
		return value
	}
	return fallback
}

func envList(key string) []string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, s := range parts {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if seconds, err := strconv.Atoi(v); err == nil {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}
