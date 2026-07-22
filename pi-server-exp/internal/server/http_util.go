package server

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Debug("writeJSON encode failed", "error", err)
	}
}

func corsMiddleware(allowed []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// When no origins are configured, reject browser cross-origin requests
		// (those with an Origin header) to prevent malicious webpages from
		// accessing pi-server via fetch(). Non-browser clients (curl, SDKs)
		// don't send Origin and are always allowed.
		if origin != "" && len(allowed) == 0 {
			writeErrorCode(w, r, http.StatusForbidden, CodeOriginNotAllowed, "cross-origin requests require PI_SERVER_ALLOWED_ORIGINS")
			return
		}
		if origin != "" && len(allowed) > 0 && !originAllowed(origin, allowed) {
			writeErrorCode(w, r, http.StatusForbidden, CodeOriginNotAllowed, "origin not allowed")
			return
		}
		if origin != "" && originAllowed(origin, allowed) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func originAllowed(origin string, allowed []string) bool {
	for _, value := range allowed {
		value = strings.TrimSpace(value)
		if value == origin || equivalentLoopbackOrigin(origin, value) {
			return true
		}
	}
	return false
}

// Treat localhost, 127.0.0.1, and ::1 as equivalent only when scheme and port match.
// This makes local test sites work regardless of the loopback hostname used in the URL.
func equivalentLoopbackOrigin(a, b string) bool {
	left, err := url.Parse(a)
	if err != nil {
		return false
	}
	right, err := url.Parse(b)
	if err != nil {
		return false
	}
	if left.Scheme != right.Scheme || left.Port() != right.Port() {
		return false
	}
	return isLoopbackHost(left.Hostname()) && isLoopbackHost(right.Hostname())
}
func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func authMiddleware(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Session WebSocket may authenticate via single-use ticket instead of bearer.
		// The relay WebSocket authenticates via a `pi-relay.<token>` subprotocol
		// (keeps the secret out of URLs); the relayToken query param is a
		// deprecated fallback for older bridges.
		if (isSessionWSRequest(r) && r.URL.Query().Get("ticket") != "") || (isExternalRelayWSRequest(r) && relayWSAuthenticated(r, token)) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			writeErrorCode(w, r, http.StatusUnauthorized, CodeUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				writeErrorCode(w, r, http.StatusInternalServerError, CodeInternal, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return h.Hijack()
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := sanitizeRequestID(r.Header.Get("X-Request-ID"))
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := contextWithRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// sanitizeRequestID strips characters that could be used for log injection
// (newlines, control characters) and caps length at 128.
func sanitizeRequestID(id string) string {
	if len(id) > 128 {
		id = id[:128]
	}
	j := 0
	buf := make([]byte, len(id))
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			buf[j] = c
			j++
		}
	}
	return string(buf[:j])
}

func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start).String(),
			"requestId", requestIDFrom(r),
		)
	})
}

// writeJSONAtomic writes data to path atomically using temp+rename so a crash
// mid-write cannot leave a truncated file.
func writeJSONAtomic(path string, data any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
