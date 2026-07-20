package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestWorkerWebSocketURL(t *testing.T) {
	u, err := workerWebSocketURL("http://example.test:3141/", "/v1/sessions/s1/ws")
	if err != nil {
		t.Fatal(err)
	}
	if u != "ws://example.test:3141/v1/sessions/s1/ws" {
		t.Fatalf("bad url %s", u)
	}
	u, err = workerWebSocketURL("https://example.test", "/v1/sessions/s1/ws")
	if err != nil {
		t.Fatal(err)
	}
	if u != "wss://example.test/v1/sessions/s1/ws" {
		t.Fatalf("bad url %s", u)
	}
}

func TestRemoteWorkerHealthProxy(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Fatalf("missing auth")
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	}))
	defer remote.Close()
	s := New(Config{CWD: ".", DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err := s.workers.Add(Worker{ID: "r1", URL: remote.URL, Token: "tok"}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/v1/workers/r1/health", nil)
	w := httptest.NewRecorder()
	s.workerGet(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
}

func TestOpenAPIHasSchemasAndRemoteWS(t *testing.T) {
	s := New(Config{CWD: ".", DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	w := httptest.NewRecorder()
	s.openapi(w, httptest.NewRequest("GET", "/openapi.json", nil))
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(w.Body.String(), "CreateSessionRequest") {
		t.Fatal("missing schema")
	}
	if !strings.Contains(w.Body.String(), "/v1/workers/{id}/sessions/{sessionId}/ws") {
		t.Fatal("missing remote ws path")
	}
}
