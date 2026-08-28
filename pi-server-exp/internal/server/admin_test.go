package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// chdir changes the process working directory and restores it when the test
// completes. (Avoids t.Chdir, which requires Go 1.24; the module targets 1.23.)
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %q: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Errorf("restore cwd: %v", err)
		}
	})
}

func TestAdminEmptyCWDFallsBackToLaunchDir(t *testing.T) {
	dataDir := t.TempDir()
	launchDir := t.TempDir()
	t.Setenv("PI_SERVER_DATA_DIR", dataDir)
	// Simulate launching from a specific directory (os.Getwd() fallback).
	chdir(t, launchDir)
	base := ConfigFromEnv()
	settings := settingsFromConfig(base)
	if settings.CWD != launchDir {
		t.Fatalf("launch dir should be the cwd default, got %q", settings.CWD)
	}
	// Persist with an EMPTY cwd to signal "use the launch directory".
	settings.CWD = ""
	if err := writeJSONAtomic(filepath.Join(dataDir, adminConfigFilename), settings); err != nil {
		t.Fatal(err)
	}

	// Reload: now launch from a DIFFERENT directory.
	other := t.TempDir()
	chdir(t, other)
	cfg := ConfigFromEnv()
	if cfg.CWD != other {
		t.Fatalf("empty persisted cwd should fall back to launch dir %q, got %q", other, cfg.CWD)
	}
	if cfg.AdminConfigError != "" {
		t.Fatalf("unexpected admin config error: %s", cfg.AdminConfigError)
	}
}

func TestAdminSettingsOverrideEnvironment(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("PI_SERVER_DATA_DIR", dataDir)
	t.Setenv("PI_SERVER_MAX_SESSIONS", "3")
	base := ConfigFromEnv()
	settings := settingsFromConfig(base)
	settings.MaxSessions = 11
	settings.MaxActiveRuns = 6
	if err := writeJSONAtomic(filepath.Join(dataDir, adminConfigFilename), settings); err != nil {
		t.Fatal(err)
	}

	cfg := ConfigFromEnv()
	if cfg.MaxSessions != 11 || cfg.MaxActiveRuns != 6 {
		t.Fatalf("admin settings not applied: %+v", cfg)
	}
	if cfg.ConfigSources["maxSessions"] != "admin" {
		t.Fatalf("source=%q", cfg.ConfigSources["maxSessions"])
	}
}

func TestIsTailscaleIP(t *testing.T) {
	for _, tc := range []struct {
		ip   string
		want bool
	}{
		{"100.64.0.1", true},
		{"100.127.255.254", true},
		{"100.128.0.1", false},
		{"192.168.1.10", false},
	} {
		if got := isTailscaleIP(net.ParseIP(tc.ip)); got != tc.want {
			t.Errorf("isTailscaleIP(%q) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestAdminPageAllowsSameOriginScript(t *testing.T) {
	cfg := ConfigFromEnv()
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer close(s.stopHeartbeat)

	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "script-src 'self' 'unsafe-inline'") {
		t.Fatalf("CSP does not permit the embedded QR script: %q", got)
	}
}

func TestAdminLoginAndRuntimeSettings(t *testing.T) {
	dataDir, cwd := t.TempDir(), t.TempDir()
	t.Setenv("PI_SERVER_DATA_DIR", dataDir)
	t.Setenv("PI_SERVER_CWD", cwd)
	t.Setenv("PI_SERVER_AUTH_TOKEN", "admin-secret")
	cfg := ConfigFromEnv()
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer close(s.stopHeartbeat)
	h := s.httpSrv.Handler

	unauthorized := httptest.NewRecorder()
	h.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/admin/api/state", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", unauthorized.Code)
	}

	login := httptest.NewRecorder()
	h.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/admin/login", bytes.NewBufferString(`{"token":"admin-secret"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected cookie: %+v", cookies)
	}

	stateRequest := httptest.NewRequest(http.MethodGet, "/admin/api/state", nil)
	stateRequest.AddCookie(cookies[0])
	stateResponse := httptest.NewRecorder()
	h.ServeHTTP(stateResponse, stateRequest)
	if stateResponse.Code != http.StatusOK {
		t.Fatalf("state status=%d body=%s", stateResponse.Code, stateResponse.Body.String())
	}
	var state struct {
		CSRF     string        `json:"csrf"`
		Settings AdminSettings `json:"settings"`
	}
	if err := json.Unmarshal(stateResponse.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.CSRF == "" {
		t.Fatal("missing csrf token")
	}

	state.Settings.MaxSessions = 13
	state.Settings.MaxActiveRuns = 7
	body, _ := json.Marshal(state.Settings)
	updateRequest := httptest.NewRequest(http.MethodPut, "/admin/api/settings", bytes.NewReader(body))
	updateRequest.AddCookie(cookies[0])
	updateRequest.Header.Set("X-Admin-CSRF", state.CSRF)
	updateResponse := httptest.NewRecorder()
	h.ServeHTTP(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updateResponse.Code, updateResponse.Body.String())
	}
	var updated struct {
		RestartRequired bool `json:"restartRequired"`
	}
	if err := json.Unmarshal(updateResponse.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.RestartRequired {
		t.Fatal("runtime-only settings unexpectedly require restart")
	}
	if got := s.workerCapacity()["maxSessions"]; got != int64(13) {
		t.Fatalf("maxSessions=%v", got)
	}
	if got := s.admission.Snapshot().GlobalLimit; got != 7 {
		t.Fatalf("global limit=%d", got)
	}
	if _, err := os.Stat(filepath.Join(dataDir, adminConfigFilename)); err != nil {
		t.Fatal(err)
	}
}

func TestAdminWorksWithoutToken(t *testing.T) {
	dataDir, cwd := t.TempDir(), t.TempDir()
	t.Setenv("PI_SERVER_DATA_DIR", dataDir)
	t.Setenv("PI_SERVER_CWD", cwd)
	t.Setenv("PI_SERVER_AUTH_TOKEN", "")
	cfg := ConfigFromEnv()
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer close(s.stopHeartbeat)

	state := httptest.NewRecorder()
	s.httpSrv.Handler.ServeHTTP(state, httptest.NewRequest(http.MethodGet, "/admin/api/state", nil))
	if state.Code != http.StatusOK {
		t.Fatalf("state status=%d body=%s", state.Code, state.Body.String())
	}
	var response struct {
		AuthenticationEnabled bool          `json:"authenticationEnabled"`
		CSRF                  string        `json:"csrf"`
		Settings              AdminSettings `json:"settings"`
	}
	if err := json.Unmarshal(state.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.AuthenticationEnabled {
		t.Fatal("authentication should be disabled without PI_SERVER_AUTH_TOKEN")
	}
	if response.CSRF != "" {
		t.Fatalf("csrf=%q, want empty", response.CSRF)
	}

	response.Settings.MaxSessions = 9
	body, err := json.Marshal(response.Settings)
	if err != nil {
		t.Fatal(err)
	}
	settings := httptest.NewRecorder()
	s.httpSrv.Handler.ServeHTTP(settings, httptest.NewRequest(http.MethodPut, "/admin/api/settings", bytes.NewReader(body)))
	if settings.Code != http.StatusOK {
		t.Fatalf("settings status=%d body=%s", settings.Code, settings.Body.String())
	}
}

func TestAdminRejectsMutationWithoutCSRF(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("PI_SERVER_DATA_DIR", dataDir)
	cfg := ConfigFromEnv()
	cfg.AdminConfigPath = filepath.Join(dataDir, adminConfigFilename)
	cfg.AuthToken = "secret"
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer close(s.stopHeartbeat)
	s.admin.sessions["session"] = adminSession{Expires: s.startedAt.Add(adminSessionLifetime), CSRF: "csrf"}
	req := httptest.NewRequest(http.MethodPut, "/admin/api/settings", bytes.NewBufferString(`{}`))
	req.AddCookie(&http.Cookie{Name: adminCookieName, Value: "session"})
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d", rec.Code)
	}
}
