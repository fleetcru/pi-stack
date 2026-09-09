package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// adminSettingsJSON marshals a full AdminSettings payload based on the server
// config, overriding the given fields.
func adminSettingsJSON(t *testing.T, s *Server, overrides func(*AdminSettings)) string {
	t.Helper()
	if s.cfg.PiBinary == "" {
		s.cfg.PiBinary = "pi"
	}
	settings := settingsFromConfig(s.cfg)
	if overrides != nil {
		overrides(&settings)
	}
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	return string(data)
}

func TestAdminV1StateAuth(t *testing.T) {
	s := newTestServer(t, "secret")
	handler := serve(s)

	// Without a token: unauthorized.
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/state", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: want 401 got %d", w.Code)
	}

	// Paired-device token must not be able to read admin state.
	_, deviceToken, err := s.devices.create("phone")
	if err != nil {
		t.Fatalf("create device: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/v1/admin/state", nil)
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("device token: want 401 got %d", w.Code)
	}

	// Bootstrap token is accepted.
	req = httptest.NewRequest(http.MethodGet, "/v1/admin/state", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("bootstrap token: want 200 got %d body=%s", w.Code, w.Body.String())
	}
	state := decodeJSON(t, w)
	if state["authenticationEnabled"] != true {
		t.Fatalf("authenticationEnabled = %v", state["authenticationEnabled"])
	}
	if state["csrf"] != "" {
		t.Fatalf("bearer state should not expose csrf, got %v", state["csrf"])
	}
}

func TestAdminV1StateAuthDisabled(t *testing.T) {
	s := newTestServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/state", nil)
	w := httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", w.Code, w.Body.String())
	}
	state := decodeJSON(t, w)
	if state["authenticationEnabled"] != false {
		t.Fatalf("authenticationEnabled = %v", state["authenticationEnabled"])
	}
}

func TestAdminV1DeviceTokenCannotChangeSettings(t *testing.T) {
	s := newTestServer(t, "secret")
	handler := serve(s)
	_, deviceToken, err := s.devices.create("phone")
	if err != nil {
		t.Fatalf("create device: %v", err)
	}
	body := adminSettingsJSON(t, s, func(settings *AdminSettings) { settings.MaxSessions = 9 })
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/settings", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("device token settings: want 401 got %d body=%s", w.Code, w.Body.String())
	}
	if got := s.sessions.maxSessions; got == 9 {
		t.Fatalf("device token must not change runtime settings")
	}
}

func TestAdminV1PutSettingsRuntimeAndRestartRequired(t *testing.T) {
	s := newTestServer(t, "secret")
	handler := serve(s)
	put := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/v1/admin/settings", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer secret")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}

	// Runtime-only change: applied immediately, no restart required.
	body := adminSettingsJSON(t, s, func(settings *AdminSettings) { settings.MaxSessions = 7 })
	w := put(body)
	if w.Code != http.StatusOK {
		t.Fatalf("runtime put: want 200 got %d body=%s", w.Code, w.Body.String())
	}
	resp := decodeJSON(t, w)
	if resp["restartRequired"] != false {
		t.Fatalf("restartRequired = %v, want false", resp["restartRequired"])
	}
	if got := s.sessions.maxSessions; got != 7 {
		t.Fatalf("maxSessions not applied at runtime: %d", got)
	}

	// Structural change (addr): persisted, but restart required.
	body = adminSettingsJSON(t, s, func(settings *AdminSettings) { settings.Addr = "127.0.0.1:3999" })
	w = put(body)
	if w.Code != http.StatusOK {
		t.Fatalf("structural put: want 200 got %d body=%s", w.Code, w.Body.String())
	}
	resp = decodeJSON(t, w)
	if resp["restartRequired"] != true {
		t.Fatalf("restartRequired = %v, want true", resp["restartRequired"])
	}

	// Invalid payload is rejected without applying.
	w = put(`{"addr":"not-a-host-port"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid put: want 400 got %d", w.Code)
	}
}

func TestAdminV1DevicePurge(t *testing.T) {
	s := newTestServer(t, "secret")
	handler := serve(s)
	record, _, err := s.devices.create("tablet")
	if err != nil {
		t.Fatalf("create device: %v", err)
	}

	// Paired-device tokens cannot purge devices.
	_, deviceToken, err := s.devices.create("phone")
	if err != nil {
		t.Fatalf("create device: %v", err)
	}
	req := httptest.NewRequest(http.MethodDelete, "/v1/devices/"+record.ID+"/purge", nil)
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("device purge with device token: want 403 got %d", w.Code)
	}

	// Bootstrap token purges the device.
	req = httptest.NewRequest(http.MethodDelete, "/v1/devices/"+record.ID+"/purge", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("device purge: want 200 got %d body=%s", w.Code, w.Body.String())
	}
	if got := s.devices.delete(record.ID); got {
		t.Fatalf("device should already be deleted")
	}

	// Purging again reports not found.
	req = httptest.NewRequest(http.MethodDelete, "/v1/devices/"+record.ID+"/purge", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("second purge: want 404 got %d", w.Code)
	}
}
