package server

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type requestMetric struct {
	Count      uint64
	ErrorCount uint64
	Total      time.Duration
	Max        time.Duration
}

type requestMetrics struct {
	mu     sync.Mutex
	routes map[string]requestMetric
}

func newRequestMetrics() *requestMetrics {
	return &requestMetrics{routes: make(map[string]requestMetric)}
}

func metricsMiddleware(metrics *requestMetrics, next http.Handler, resolvers ...*http.ServeMux) http.Handler {
	var resolver *http.ServeMux
	if len(resolvers) > 0 {
		resolver = resolvers[0]
	} else if mux, ok := next.(*http.ServeMux); ok {
		resolver = mux
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		pattern := r.Pattern
		if pattern == "" && resolver != nil {
			_, pattern = resolver.Handler(r)
		}
		writer := &metricResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(writer, r)
		if metrics == nil {
			return
		}
		if pattern == "" {
			pattern = "unmatched"
		}
		key := pattern
		if !strings.HasPrefix(pattern, r.Method+" ") {
			key = r.Method + " " + pattern
		}
		elapsed := time.Since(started)
		metrics.mu.Lock()
		value := metrics.routes[key]
		value.Count++
		if writer.status >= 400 {
			value.ErrorCount++
		}
		value.Total += elapsed
		if elapsed > value.Max {
			value.Max = elapsed
		}
		metrics.routes[key] = value
		metrics.mu.Unlock()
	})
}

// counters returns a copy of the cumulative per-route counters. Used by the
// performance sampler to compute per-interval request/error/latency deltas.
func (m *requestMetrics) counters() map[string]requestMetric {
	if m == nil {
		return map[string]requestMetric{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]requestMetric, len(m.routes))
	for route, value := range m.routes {
		out[route] = value
	}
	return out
}

// performance renders the cumulative counters as numeric route statistics for
// GET /v1/performance.
func (m *requestMetrics) performance() []requestPerformance {
	counters := m.counters()
	out := make([]requestPerformance, 0, len(counters))
	for route, value := range counters {
		averageMs := 0.0
		if value.Count > 0 {
			averageMs = value.Total.Seconds() * 1000 / float64(value.Count)
		}
		out = append(out, requestPerformance{
			Route:     route,
			Count:     value.Count,
			Errors:    value.ErrorCount,
			AverageMs: averageMs,
			MaxMs:     value.Max.Seconds() * 1000,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Route < out[j].Route })
	return out
}

func (m *requestMetrics) snapshot() map[string]map[string]any {
	if m == nil {
		return map[string]map[string]any{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]map[string]any, len(m.routes))
	for route, value := range m.routes {
		average := time.Duration(0)
		if value.Count > 0 {
			average = value.Total / time.Duration(value.Count)
		}
		out[route] = map[string]any{
			"count":   value.Count,
			"errors":  value.ErrorCount,
			"average": average.String(),
			"max":     value.Max.String(),
		}
	}
	return out
}

// metricResponseWriter preserves websocket upgrades and streaming semantics.
type metricResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *metricResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *metricResponseWriter) Write(data []byte) (int, error) {
	return w.ResponseWriter.Write(data)
}

func (w *metricResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *metricResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (w *metricResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
