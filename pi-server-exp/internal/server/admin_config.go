package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

const adminConfigFilename = "admin-config.json"

// AdminSettings is the non-secret configuration persisted by /admin. Durations
// are strings so the file and UI remain readable (for example "30s" or "2h").
type AdminSettings struct {
	Addr                  string   `json:"addr"`
	PiBinary              string   `json:"piBinary"`
	Extensions            []string `json:"extensions"`
	CWD                   string   `json:"cwd"`
	DataDir               string   `json:"dataDir"`
	AllowedOrigins        []string `json:"allowedOrigins"`
	AllowedRoots          []string `json:"allowedRoots"`
	AllowedWorkerHosts    []string `json:"allowedWorkerHosts"`
	ShutdownTimeout       string   `json:"shutdownTimeout"`
	RequestTimeout        string   `json:"requestTimeout"`
	ReadTimeout           string   `json:"readTimeout"`
	WriteTimeout          string   `json:"writeTimeout"`
	IdleTimeout           string   `json:"idleTimeout"`
	MaxSessions           int      `json:"maxSessions"`
	MaxActiveRuns         int      `json:"maxActiveRuns"`
	MaxRunsPerSession     int      `json:"maxRunsPerSession"`
	MaxRunsPerWorker      int      `json:"maxRunsPerWorker"`
	MaxQueuedRuns         int      `json:"maxQueuedRuns"`
	DistributedRunTimeout string   `json:"distributedRunTimeout"`
	RestartMax            int      `json:"restartMax"`
	RestartBackoff        string   `json:"restartBackoff"`
	EventHistoryMax       int      `json:"eventHistoryMax"`
	EventHistoryBytes     int      `json:"eventHistoryBytes"`
	MaxWatches            int      `json:"maxWatches"`
	Debug                 bool     `json:"debug"`
}

func settingsFromConfig(cfg Config) AdminSettings {
	return AdminSettings{
		Addr: cfg.Addr, PiBinary: cfg.PiBinary, Extensions: cloneStrings(cfg.Extensions), CWD: cfg.CWD,
		DataDir: cfg.DataDir, AllowedOrigins: cloneStrings(cfg.AllowedOrigins), AllowedRoots: cloneStrings(cfg.AllowedRoots),
		AllowedWorkerHosts: cloneStrings(cfg.AllowedWorkerHosts), ShutdownTimeout: cfg.ShutdownTimeout.String(),
		RequestTimeout: cfg.RequestTimeout.String(), ReadTimeout: cfg.ReadTimeout.String(), WriteTimeout: cfg.WriteTimeout.String(),
		IdleTimeout: cfg.IdleTimeout.String(), MaxSessions: cfg.MaxSessions, MaxActiveRuns: cfg.MaxActiveRuns,
		MaxRunsPerSession: cfg.MaxRunsPerSession, MaxRunsPerWorker: cfg.MaxRunsPerWorker, MaxQueuedRuns: cfg.MaxQueuedRuns,
		DistributedRunTimeout: cfg.DistributedRunTimeout.String(), RestartMax: cfg.RestartMax, RestartBackoff: cfg.RestartBackoff.String(),
		EventHistoryMax: cfg.EventHistoryMax, EventHistoryBytes: cfg.EventHistoryBytes, MaxWatches: cfg.MaxWatches,
		Debug: cfg.LogLevel <= -4,
	}
}

func (a AdminSettings) apply(cfg *Config) error {
	parsed, err := a.parsedDurations()
	if err != nil {
		return err
	}
	if _, _, err := net.SplitHostPort(a.Addr); err != nil {
		return fmt.Errorf("addr must be host:port: %w", err)
	}
	if a.PiBinary == "" || a.CWD == "" || a.DataDir == "" {
		return errors.New("piBinary, cwd, and dataDir must not be empty")
	}
	for name, value := range map[string]int{
		"maxSessions": a.MaxSessions, "maxActiveRuns": a.MaxActiveRuns, "maxRunsPerSession": a.MaxRunsPerSession,
		"maxRunsPerWorker": a.MaxRunsPerWorker, "maxQueuedRuns": a.MaxQueuedRuns, "restartMax": a.RestartMax,
		"eventHistoryMax": a.EventHistoryMax, "eventHistoryBytes": a.EventHistoryBytes, "maxWatches": a.MaxWatches,
	} {
		if value < 0 {
			return fmt.Errorf("%s must be >= 0", name)
		}
	}
	cfg.Addr, cfg.PiBinary, cfg.Extensions, cfg.CWD, cfg.DataDir = a.Addr, a.PiBinary, cloneStrings(a.Extensions), a.CWD, a.DataDir
	cfg.AllowedOrigins, cfg.AllowedRoots, cfg.AllowedWorkerHosts = cloneStrings(a.AllowedOrigins), cloneStrings(a.AllowedRoots), cloneStrings(a.AllowedWorkerHosts)
	cfg.ShutdownTimeout, cfg.RequestTimeout, cfg.ReadTimeout, cfg.WriteTimeout, cfg.IdleTimeout = parsed[0], parsed[1], parsed[2], parsed[3], parsed[4]
	cfg.MaxSessions, cfg.MaxActiveRuns, cfg.MaxRunsPerSession, cfg.MaxRunsPerWorker, cfg.MaxQueuedRuns = a.MaxSessions, a.MaxActiveRuns, a.MaxRunsPerSession, a.MaxRunsPerWorker, a.MaxQueuedRuns
	cfg.DistributedRunTimeout, cfg.RestartMax, cfg.RestartBackoff = parsed[5], a.RestartMax, parsed[6]
	cfg.EventHistoryMax, cfg.EventHistoryBytes, cfg.MaxWatches = a.EventHistoryMax, a.EventHistoryBytes, a.MaxWatches
	if a.Debug {
		cfg.LogLevel = -4
	} else {
		cfg.LogLevel = 0
	}
	return nil
}

func (a AdminSettings) parsedDurations() ([7]time.Duration, error) {
	var out [7]time.Duration
	values := []struct{ name, value string }{
		{"shutdownTimeout", a.ShutdownTimeout}, {"requestTimeout", a.RequestTimeout}, {"readTimeout", a.ReadTimeout},
		{"writeTimeout", a.WriteTimeout}, {"idleTimeout", a.IdleTimeout}, {"distributedRunTimeout", a.DistributedRunTimeout},
		{"restartBackoff", a.RestartBackoff},
	}
	for i, value := range values {
		d, err := time.ParseDuration(value.value)
		if err != nil || d < 0 {
			return out, fmt.Errorf("%s must be a non-negative duration", value.name)
		}
		out[i] = d
	}
	return out, nil
}

func loadAdminSettings(cfg *Config) {
	path := filepath.Join(cfg.DataDir, adminConfigFilename)
	cfg.AdminConfigPath = path
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		cfg.AdminConfigError = err.Error()
		return
	}
	var settings AdminSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		cfg.AdminConfigError = err.Error()
		return
	}
	if err := settings.apply(cfg); err != nil {
		cfg.AdminConfigError = err.Error()
		return
	}
	cfg.AdminConfigLoaded = true
	for _, key := range adminSettingKeys {
		cfg.ConfigSources[key] = "admin"
	}
}

var adminSettingKeys = []string{
	"addr", "piBinary", "extensions", "cwd", "dataDir", "allowedOrigins", "allowedRoots", "allowedWorkerHosts",
	"shutdownTimeout", "requestTimeout", "readTimeout", "writeTimeout", "idleTimeout", "maxSessions", "maxActiveRuns",
	"maxRunsPerSession", "maxRunsPerWorker", "maxQueuedRuns", "distributedRunTimeout", "restartMax", "restartBackoff",
	"eventHistoryMax", "eventHistoryBytes", "maxWatches", "debug",
}

func cloneStrings(values []string) []string { return append([]string(nil), values...) }
