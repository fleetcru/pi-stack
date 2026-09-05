package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListSessionsAppliesLimitToMostRecentlyUpdatedLocalSpecs(t *testing.T) {
	registry := NewSessionRegistry("", 0)
	now := time.Now().UTC()
	for _, spec := range []SessionSpec{
		{ID: "old", UpdatedAt: now.Add(-time.Hour)},
		{ID: "new", UpdatedAt: now},
	} {
		registry.mu.Lock()
		registry.specs[spec.ID] = spec
		registry.mu.Unlock()
	}
	server := &Server{sessions: registry}
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions?limit=1", nil)
	response := httptest.NewRecorder()
	server.listSessions(response, request)
	if response.Code != http.StatusOK { t.Fatalf("status=%d body=%s", response.Code, response.Body.String()) }
	var body struct { Sessions []SessionSpec `json:"sessions"` }
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil { t.Fatal(err) }
	if len(body.Sessions) != 1 || body.Sessions[0].ID != "new" {
		t.Fatalf("unexpected sessions: %#v", body.Sessions)
	}
}
