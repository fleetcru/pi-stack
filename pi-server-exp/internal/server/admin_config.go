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
	if a.PiBinary == "" || a.DataDir == "" {
		return errors.New("piBinary and dataDir must not be empty")
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
	cfg.Addr = a.Addr
	cfg.PiBinary = a.PiBinary
	cfg.Extensions = cloneStrings(a.Extensions)
	if a.CWD != "" {
		// An empty persisted cwd means "use the launch directory" (whatever
		// PI_SERVER_CWD or os.Getwd resolved to), so a portable admin config
		// does not pin the server to a single checkout.
		cfg.CWD = a.CWD
	}
	cfg.DataDir = a.DataDir
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

// PersistAdminConfig writes the current effective configuration back to
// admin-config.json. Called after CLI parsing so the persisted file reflects
// the values the server is actually about to use, preventing a stale file from
// pinning a previous listen address or data directory across restarts.
func PersistAdminConfig(cfg Config) error {
	if cfg.AdminConfigPath == "" {
		return errors.New("admin config path is not set")
	}
	return writeJSONAtomic(cfg.AdminConfigPath, settingsFromConfig(cfg))
}

var adminSettingKeys = []string{
	"addr", "piBinary", "extensions", "cwd", "dataDir", "allowedOrigins", "allowedRoots", "allowedWorkerHosts",
	"shutdownTimeout", "requestTimeout", "readTimeout", "writeTimeout", "idleTimeout", "maxSessions", "maxActiveRuns",
	"maxRunsPerSession", "maxRunsPerWorker", "maxQueuedRuns", "distributedRunTimeout", "restartMax", "restartBackoff",
	"eventHistoryMax", "eventHistoryBytes", "maxWatches", "debug",
}

func cloneStrings(values []string) []string { return append([]string(nil), values...) }

// startAdminConfigWatch polls admin-config.json and hot-applies the safe
// subset when its content changes. This lets an operator or the server itself
// revise capacity, origins, roots, worker allowlists, and durations while the
// process is live, matching the behavior of the Admin settings endpoint.
func (s *Server) startAdminConfigWatch() {
	s.adminConfigMu.Lock()
	if s.adminConfigStop != nil {
		s.adminConfigMu.Unlock()
		return
	}
	stop := make(chan struct{})
	s.adminConfigStop = stop
	s.adminConfigMu.Unlock()
	path := s.cfg.AdminConfigPath
	var lastMtime time.Time
	if info, err := os.Stat(path); err == nil {
		lastMtime = info.ModTime()
	}
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				info, err := os.Stat(path)
				if err != nil || info.ModTime().Equal(lastMtime) {
					continue
				}
				lastMtime = info.ModTime()
				data, err := os.ReadFile(path)
				if err != nil {
					s.logger.Warn("could not read admin config", "error", err)
					continue
				}
				var settings AdminSettings
				if err := json.Unmarshal(data, &settings); err != nil {
					s.logger.Warn("could not parse admin config", "error", err)
					continue
				}
				var validated Config
				validated = s.cfg
				if err := settings.apply(&validated); err != nil {
					s.logger.Warn("ignoring invalid admin config", "error", err)
					continue
				}
				// Only apply the hot-reloadable subset to live state.
				s.applyRuntimeSettings(settings)
				s.logger.Info("admin config hot-applied")
			}
		}
	}()
}

func (s *Server) stopAdminConfigWatch() {
	s.adminConfigMu.Lock()
	stop := s.adminConfigStop
	s.adminConfigStop = nil
	s.adminConfigMu.Unlock()
	if stop != nil {
		close(stop)
	}
}
