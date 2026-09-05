package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetricsMiddlewareRecordsPatternAndFailure(t *testing.T) {
	metrics := newRequestMetrics()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	})
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	metricsMiddleware(metrics, mux).ServeHTTP(response, request)

	snapshot := metrics.snapshot()
	value, ok := snapshot["GET GET /healthz"]
	if !ok {
		t.Fatalf("missing route metric: %#v", snapshot)
	}
	if value["count"] != uint64(1) || value["errors"] != uint64(1) {
		t.Fatalf("unexpected metric: %#v", value)
	}
}
