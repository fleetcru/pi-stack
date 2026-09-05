package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestBodyLimitMiddlewareRejectsOversizeBody(t *testing.T) {
	handler := requestBodyLimitMiddleware(4, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("12345"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSecurityHeadersMiddlewareSetsBrowserProtections(t *testing.T) {
	handler := securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Header().Get("X-Content-Type-Options") != "nosniff" ||
		response.Header().Get("X-Frame-Options") != "DENY" ||
		response.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("missing security headers: %#v", response.Header())
	}
}

func TestPrometheusMetricsExportsRequestCounters(t *testing.T) {
	server := &Server{
		metrics:   newRequestMetrics(),
		sessions:  NewSessionRegistry("", 0),
		workers:   NewWorkerRegistry(""),
		startedAt: testNow(),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	metricsMiddleware(server.metrics, mux).ServeHTTP(httptest.NewRecorder(), request)

	response := httptest.NewRecorder()
	server.prometheusMetrics(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := response.Body.String()
	if response.Header().Get("Content-Type") != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("content type=%q", response.Header().Get("Content-Type"))
	}
	if !strings.Contains(body, `pi_server_http_requests_total{method="GET",route="GET /healthz"} 1`) {
		t.Fatalf("missing request metric: %s", body)
	}
	if !strings.Contains(body, "pi_server_goroutines ") {
		t.Fatalf("missing runtime metric: %s", body)
	}
}
