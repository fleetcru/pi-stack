package server

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"
)

type requestMetric struct {
	Count       uint64
	ErrorCount  uint64
	Total       time.Duration
	Max         time.Duration
}

type requestMetrics struct {
	mu     sync.Mutex
	routes map[string]requestMetric
}

func newRequestMetrics() *requestMetrics {
	return &requestMetrics{routes: make(map[string]requestMetric)}
}

func metricsMiddleware(metrics *requestMetrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		writer := &metricResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(writer, r)
		if metrics == nil {
			return
		}
		pattern := r.Pattern
		if pattern == "" {
			pattern = "unmatched"
		}
		key := r.Method + " " + pattern
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
			"count": value.Count,
			"errors": value.ErrorCount,
			"average": average.String(),
			"max": value.Max.String(),
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
