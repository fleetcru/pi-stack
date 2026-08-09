package server

import (
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed adminui/index.html
var adminHTML []byte

const adminCookieName = "pi_server_admin"
const adminSessionLifetime = 8 * time.Hour

type adminSession struct {
	Expires time.Time
	CSRF    string
}

type adminState struct {
	mu       sync.Mutex
	sessions map[string]adminSession
	target   AdminSettings
}

func newAdminState(cfg Config) *adminState {
	return &adminState{sessions: make(map[string]adminSession), target: settingsFromConfig(cfg)}
}

func (s *Server) adminRoot(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/admin/":
		s.adminPage(w, r)
	case r.URL.Path == "/admin/login":
		s.adminLogin(w, r)
	case strings.HasPrefix(r.URL.Path, "/admin/api/"):
		s.adminAPI(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) adminPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/admin" && r.URL.Path != "/admin/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(adminHTML)
}

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeErrorText(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var input struct {
		Token string `json:"token"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeErrorText(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if subtle.ConstantTimeCompare([]byte(input.Token), []byte(s.cfg.AuthToken)) != 1 {
		time.Sleep(150 * time.Millisecond)
		writeErrorText(w, http.StatusUnauthorized, "invalid server token")
		return
	}
	id, csrf := randomAdminToken(), randomAdminToken()
	if id == "" || csrf == "" {
		writeErrorText(w, http.StatusInternalServerError, "could not create admin session")
		return
	}
	now := time.Now()
	s.admin.mu.Lock()
	for key, session := range s.admin.sessions {
		if now.After(session.Expires) {
			delete(s.admin.sessions, key)
		}
	}
	s.admin.sessions[id] = adminSession{Expires: now.Add(adminSessionLifetime), CSRF: csrf}
	s.admin.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: adminCookieName, Value: id, Path: "/admin", MaxAge: int(adminSessionLifetime.Seconds()), HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func randomAdminToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func (s *Server) adminAuthorized(r *http.Request) (adminSession, bool) {
	if s.cfg.AuthToken == "" {
		return adminSession{Expires: time.Now().Add(time.Hour), CSRF: ""}, true
	}
	cookie, err := r.Cookie(adminCookieName)
	if err != nil {
		return adminSession{}, false
	}
	s.admin.mu.Lock()
	defer s.admin.mu.Unlock()
	session, ok := s.admin.sessions[cookie.Value]
	if !ok || time.Now().After(session.Expires) {
		delete(s.admin.sessions, cookie.Value)
		return adminSession{}, false
	}
	return session, true
}

func (s *Server) adminAPI(w http.ResponseWriter, r *http.Request) {
	session, ok := s.adminAuthorized(r)
	if !ok {
		writeErrorText(w, http.StatusUnauthorized, "admin login required")
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/admin/api/state":
		s.adminGetState(w, session)
	case r.Method == http.MethodPut && r.URL.Path == "/admin/api/settings":
		if !s.validAdminMutation(r, session) {
			writeErrorText(w, http.StatusForbidden, "invalid admin CSRF token or origin")
			return
		}
		s.adminPutSettings(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/admin/api/logout":
		if !s.validAdminMutation(r, session) {
			writeErrorText(w, http.StatusForbidden, "invalid admin CSRF token or origin")
			return
		}
		s.adminLogout(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) validAdminMutation(r *http.Request, session adminSession) bool {
	if s.cfg.AuthToken != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Admin-CSRF")), []byte(session.CSRF)) != 1 {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return origin == scheme+"://"+r.Host
}

func (s *Server) adminGetState(w http.ResponseWriter, session adminSession) {
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
		warnings = append(warnings, "Authentication is disabled; keep the server bound to loopback only.")
	}
	if s.cfg.AdminConfigError != "" {
		warnings = append(warnings, "Persisted admin configuration could not be loaded: "+s.cfg.AdminConfigError)
	}
	if restartSettingsDiffer(target, settingsFromConfig(s.cfg)) {
		warnings = append(warnings, "Saved settings are pending a manual server restart.")
	}
	unhealthy := 0
	for _, worker := range workers {
		if strings.ToLower(worker.Status) != "online" {
			unhealthy++
		}
	}
	if unhealthy > 0 {
		warnings = append(warnings, "One or more workers are not online.")
	}
	transports := map[string]int{}
	for _, spec := range s.sessions.ListSpecs() {
		transports[spec.Transport]++
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"csrf": session.CSRF, "authenticationEnabled": s.cfg.AuthToken != "",
		"overview": map[string]any{
			"apiVersion": APIVersion, "uptimeSeconds": int64(time.Since(s.startedAt).Seconds()),
			"sessions": map[string]any{"active": s.sessions.ActiveCount(), "registered": len(s.sessions.ListSpecs()), "byTransport": transports, "max": atomic.LoadInt64(&s.maxSessionsAtomic)},
			"workers":  map[string]any{"total": len(workers), "unhealthy": unhealthy}, "scheduler": snapshot,
			"warnings": warnings, "configPath": s.cfg.AdminConfigPath,
		},
		"settings": target, "effectiveSettings": effective, "sources": sources,
		"runtimeFields":   []string{"maxSessions", "maxActiveRuns", "maxRunsPerSession", "maxRunsPerWorker", "maxQueuedRuns"},
		"restartRequired": restartSettingsDiffer(target, settingsFromConfig(s.cfg)),
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
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restartRequired": restartSettingsDiffer(settings, settingsFromConfig(s.cfg))})
}

func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(adminCookieName); err == nil {
		s.admin.mu.Lock()
		delete(s.admin.sessions, cookie.Value)
		s.admin.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: adminCookieName, Value: "", Path: "/admin", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
