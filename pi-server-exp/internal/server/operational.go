package server

import (
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxHTTPBodyBytes int64 = 10 << 20

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// requestBodyLimitMiddleware rejects declared oversized requests before they
// allocate work and bounds streaming bodies that omit Content-Length.
func requestBodyLimitMiddleware(limit int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > limit {
			writeErrorText(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) prometheusMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	var retainedEvents, retainedEventBytes int
	var droppedEvents uint64
	for _, id := range s.sessions.List() {
		if process, ok := s.sessions.Get(id); ok {
			retained, bytes, dropped := process.EventMetrics()
			retainedEvents += retained
			retainedEventBytes += bytes
			droppedEvents += dropped
		}
	}
	workers := s.workers.List()
	fmt.Fprintln(w, "# HELP pi_server_http_requests_total Total HTTP requests by route.")
	fmt.Fprintln(w, "# TYPE pi_server_http_requests_total counter")
	s.metrics.writePrometheus(w)
	writePrometheusGauge(w, "pi_server_start_time_seconds", "Server process start time.", float64(s.startedAt.Unix()))
	writePrometheusGauge(w, "pi_server_sessions", "Persisted session count.", float64(len(s.sessions.List())))
	writePrometheusGauge(w, "pi_server_active_sessions", "Running session count.", float64(s.sessions.ActiveCount()))
	writePrometheusGauge(w, "pi_server_workers", "Registered worker count.", float64(len(workers)))
	writePrometheusGauge(w, "pi_server_healthy_workers", "Healthy worker count.", float64(countHealthyWorkers(workers)))
	writePrometheusGauge(w, "pi_server_event_replay_retained", "Retained replay event count.", float64(retainedEvents))
	writePrometheusGauge(w, "pi_server_event_replay_retained_bytes", "Retained replay event bytes.", float64(retainedEventBytes))
	writePrometheusGauge(w, "pi_server_event_replay_dropped_total", "Dropped replay event count.", float64(droppedEvents))
	writePrometheusGauge(w, "pi_server_goroutines", "Current Go goroutine count.", float64(runtime.NumGoroutine()))
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	writePrometheusGauge(w, "pi_server_heap_alloc_bytes", "Allocated heap bytes.", float64(memory.HeapAlloc))
}

func (m *requestMetrics) writePrometheus(w http.ResponseWriter) {
	if m == nil {
		return
	}
	m.mu.Lock()
	routes := make([]string, 0, len(m.routes))
	values := make(map[string]requestMetric, len(m.routes))
	for route, value := range m.routes {
		routes = append(routes, route)
		values[route] = value
	}
	m.mu.Unlock()
	sort.Strings(routes)
	fmt.Fprintln(w, "# HELP pi_server_http_request_errors_total HTTP requests completed with a 4xx or 5xx status.")
	fmt.Fprintln(w, "# TYPE pi_server_http_request_errors_total counter")
	fmt.Fprintln(w, "# HELP pi_server_http_request_duration_seconds Cumulative HTTP request duration by route.")
	fmt.Fprintln(w, "# TYPE pi_server_http_request_duration_seconds counter")
	for _, route := range routes {
		method, pattern, _ := strings.Cut(route, " ")
		labels := `{method="` + prometheusLabel(method) + `",route="` + prometheusLabel(pattern) + `"}`
		value := values[route]
		fmt.Fprintf(w, "pi_server_http_requests_total%s %d\n", labels, value.Count)
		fmt.Fprintf(w, "pi_server_http_request_errors_total%s %d\n", labels, value.ErrorCount)
		fmt.Fprintf(w, "pi_server_http_request_duration_seconds%s %s\n", labels, strconv.FormatFloat(value.Total.Seconds(), 'f', -1, 64))
	}
}

func writePrometheusGauge(w http.ResponseWriter, name, help string, value float64) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %s\n", name, help, name, name, strconv.FormatFloat(value, 'f', -1, 64))
}

func prometheusLabel(value string) string {
	return strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\"", "\\\"").Replace(value)
}

func metricsCacheControlMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" || r.URL.Path == "/v1/diagnostics" {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

var _ = time.Second
