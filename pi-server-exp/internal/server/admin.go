package server

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type adminState struct {
	mu     sync.Mutex
	target AdminSettings
}

func newAdminState(cfg Config) *adminState {
	return &adminState{target: settingsFromConfig(cfg)}
}

type pairingEndpoint struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

func pairingEndpoints(addr string) []pairingEndpoint {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		port = "3142"
	}
	var lan, tailscale []pairingEndpoint
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range interfaces {
		if !isPhysicalAdapter(iface) {
			continue
		}
		for _, ip := range interfaceIPv4Addrs(iface) {
			url := "http://" + ip.String() + ":" + port
			if isTailscaleIP(ip) {
				tailscale = append(tailscale, pairingEndpoint{Label: "Tailscale (" + iface.Name + ")", URL: url})
			} else if isPrivateLANIPv4(ip) {
				lan = append(lan, pairingEndpoint{Label: "Home network (" + iface.Name + ")", URL: url})
			}
		}
	}
	sort.Slice(lan, func(i, j int) bool { return lan[i].URL < lan[j].URL })
	sort.Slice(tailscale, func(i, j int) bool { return tailscale[i].URL < tailscale[j].URL })
	return append(lan, tailscale...)
}

// adminStateV1 serves GET /v1/admin/state for Webby, Desktop, and other
// bootstrap bearer-token clients.
func (s *Server) adminStateV1(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearerAdmin(w, r) {
		return
	}
	s.adminGetState(w)
}

// adminSettingsV1 serves PUT /v1/admin/settings for bootstrap bearer-token clients.
func (s *Server) adminSettingsV1(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearerAdmin(w, r) {
		return
	}
	s.adminPutSettings(w, r)
}

// requireBearerAdmin allows the request only when authentication is disabled or
// the exact bootstrap bearer token is presented. Paired-device tokens never
// satisfy this check, so they cannot read admin state or change settings.
func (s *Server) requireBearerAdmin(w http.ResponseWriter, r *http.Request) bool {
	if s.cfg.AuthToken == "" {
		return true
	}
	provided := r.Header.Get("Authorization")
	ok := len(provided) == len("Bearer "+s.cfg.AuthToken) &&
		subtle.ConstantTimeCompare([]byte(provided), []byte("Bearer "+s.cfg.AuthToken)) == 1
	if !ok {
		writeErrorCode(w, r, http.StatusUnauthorized, CodeUnauthorized, "admin bearer token required")
		return false
	}
	return true
}

func (s *Server) adminGetState(w http.ResponseWriter) {
	snapshot := s.admission.Snapshot()
	effective := settingsFromConfig(s.cfg)
	effective.MaxSessions = int(atomic.LoadInt64(&s.maxSessionsAtomic))
	effective.MaxActiveRuns, effective.MaxRunsPerSession = snapshot.GlobalLimit, snapshot.PerSessionLimit
	effective.MaxRunsPerWorker, effective.MaxQueuedRuns = snapshot.PerWorkerLimit, snapshot.QueueLimit
	s.admin.mu.Lock()
	target := s.admin.target
	sources := make(map[string]string, len(s.cfg.ConfigSources))
	for key, value := range s.cfg.ConfigSources {
		sources[key] = value
	}
	s.admin.mu.Unlock()
	target.MaxSessions = int(atomic.LoadInt64(&s.maxSessionsAtomic))
	target.MaxActiveRuns, target.MaxRunsPerSession = snapshot.GlobalLimit, snapshot.PerSessionLimit
	target.MaxRunsPerWorker, target.MaxQueuedRuns = snapshot.PerWorkerLimit, snapshot.QueueLimit
	workers := s.workers.List()
	warnings := make([]string, 0)
	if s.cfg.AuthToken == "" {
		warnings = append(warnings, "Authentication is disabled; all devices on the home LAN or Tailscale network are trusted.")
	}
	if s.cfg.AdminConfigError != "" {
		warnings = append(warnings, "Persisted admin configuration could not be loaded: "+s.cfg.AdminConfigError)
	}
	if restartSettingsDiffer(target, settingsFromConfig(s.cfg)) {
		warnings = append(warnings, "Saved settings are pending a manual server restart.")
	}
	unhealthy := len(workers) - countHealthyWorkers(workers)
	if unhealthy > 0 {
		warnings = append(warnings, "One or more workers are not online.")
	}
	transports := map[string]int{}
	for _, spec := range s.sessions.ListSpecs() {
		transports[spec.Transport]++
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticationEnabled": s.cfg.AuthToken != "",
		"overview": map[string]any{
			"apiVersion": APIVersion, "uptimeSeconds": int64(time.Since(s.startedAt).Seconds()),
			"sessions": map[string]any{"active": s.sessions.ActiveCount(), "registered": len(s.sessions.ListSpecs()), "byTransport": transports, "max": atomic.LoadInt64(&s.maxSessionsAtomic)},
			"workers":  map[string]any{"total": len(workers), "unhealthy": unhealthy}, "scheduler": snapshot,
			"warnings": warnings, "configPath": s.cfg.AdminConfigPath,
		},
		"settings": target, "effectiveSettings": effective, "sources": sources,
		"pairingEndpoints": pairingEndpoints(s.cfg.Addr),
		"runtimeFields":    []string{"maxSessions", "maxActiveRuns", "maxRunsPerSession", "maxRunsPerWorker", "maxQueuedRuns"},
		"restartRequired":  restartSettingsDiffer(target, settingsFromConfig(s.cfg)),
	})
}

func (s *Server) adminPutSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var settings AdminSettings
	if json.NewDecoder(r.Body).Decode(&settings) != nil {
		writeErrorText(w, http.StatusBadRequest, "invalid request body")
		return
	}
	validated := s.cfg
	if err := settings.apply(&validated); err != nil {
		writeErrorText(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := writeJSONAtomic(s.cfg.AdminConfigPath, settings); err != nil {
		writeErrorText(w, http.StatusInternalServerError, "could not persist settings: "+err.Error())
		return
	}
	s.applyRuntimeSettings(settings)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restartRequired": restartSettingsDiffer(settings, settingsFromConfig(s.cfg))})
}

// applyRuntimeSettings applies the safe, hot-reloadable subset of AdminSettings
// to live server state. Structural fields (addr, cwd, dataDir, piBinary, and
// server timeouts) are intentionally not mutated here because they are fixed at
// process start; the caller reports those via restartSettingsDiffer.
func (s *Server) applyRuntimeSettings(settings AdminSettings) {
	s.sessions.mu.Lock()
	s.sessions.maxSessions = settings.MaxSessions
	s.sessions.mu.Unlock()
	atomic.StoreInt64(&s.maxSessionsAtomic, int64(settings.MaxSessions))
	s.admission.Reconfigure(settings.MaxActiveRuns, settings.MaxRunsPerSession, settings.MaxRunsPerWorker, settings.MaxQueuedRuns)
	s.admin.mu.Lock()
	s.admin.target = settings
	for _, key := range adminSettingKeys {
		if s.cfg.ConfigSources[key] != "cli" {
			s.cfg.ConfigSources[key] = "admin"
		}
	}
	s.admin.mu.Unlock()
}

func restartSettingsDiffer(a, b AdminSettings) bool {
	for _, settings := range []*AdminSettings{&a, &b} {
		if len(settings.Extensions) == 0 {
			settings.Extensions = nil
		}
		if len(settings.AllowedOrigins) == 0 {
			settings.AllowedOrigins = nil
		}
		if len(settings.AllowedRoots) == 0 {
			settings.AllowedRoots = nil
		}
		if len(settings.AllowedWorkerHosts) == 0 {
			settings.AllowedWorkerHosts = nil
		}
	}
	a.MaxSessions, b.MaxSessions = 0, 0
	a.MaxActiveRuns, b.MaxActiveRuns = 0, 0
	a.MaxRunsPerSession, b.MaxRunsPerSession = 0, 0
	a.MaxRunsPerWorker, b.MaxRunsPerWorker = 0, 0
	a.MaxQueuedRuns, b.MaxQueuedRuns = 0, 0
	return !reflect.DeepEqual(a, b)
}
