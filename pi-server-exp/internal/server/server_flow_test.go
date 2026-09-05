package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuthenticatedSessionInventoryAndMetricsFlow(t *testing.T) {
	root := t.TempDir()
	server := New(Config{
		Addr:           "127.0.0.1:0",
		PiBinary:       "pi",
		CWD:            root,
		DataDir:        t.TempDir(),
		AllowedRoots:   []string{root},
		AuthToken:      "test-token",
		RequestTimeout: time.Second,
		ReadTimeout:    time.Second,
		WriteTimeout:   time.Second,
		IdleTimeout:    time.Second,
		MaxSessions:    2,
	}, slog.Default())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})
	handler := server.httpSrv.Handler

	unauthorized := httptest.NewRequest(http.MethodGet, "/v1/sessions", nil)
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", unauthorizedResponse.Code)
	}
	if unauthorizedResponse.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security headers missing from auth rejection: %#v", unauthorizedResponse.Header())
	}

	body, err := json.Marshal(map[string]any{
		"id": "e2e-session",
		"cwd": root,
		"start": false,
		"title": "HTTP flow",
	})
	if err != nil {
		t.Fatal(err)
	}
	create := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewReader(body))
	create.Header.Set("Authorization", "Bearer test-token")
	create.Header.Set("Content-Type", "application/json")
	createResponse := httptest.NewRecorder()
	handler.ServeHTTP(createResponse, create)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createResponse.Code, createResponse.Body.String())
	}

	inventory := httptest.NewRequest(http.MethodGet, "/v1/sessions?limit=10", nil)
	inventory.Header.Set("Authorization", "Bearer test-token")
	inventoryResponse := httptest.NewRecorder()
	handler.ServeHTTP(inventoryResponse, inventory)
	if inventoryResponse.Code != http.StatusOK || !strings.Contains(inventoryResponse.Body.String(), "e2e-session") {
		t.Fatalf("inventory status=%d body=%s", inventoryResponse.Code, inventoryResponse.Body.String())
	}

	metrics := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metrics.Header.Set("Authorization", "Bearer test-token")
	metricsResponse := httptest.NewRecorder()
	handler.ServeHTTP(metricsResponse, metrics)
	if metricsResponse.Code != http.StatusOK || !strings.Contains(metricsResponse.Body.String(), "pi_server_sessions 1") {
		t.Fatalf("metrics status=%d body=%s", metricsResponse.Code, metricsResponse.Body.String())
	}
	if metricsResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("metrics cache control=%q", metricsResponse.Header().Get("Cache-Control"))
	}
}
