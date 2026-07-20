package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuthMiddleware(t *testing.T) {
	h := authMiddleware("secret", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("want 401 got %d", w.Code)
	}
	r = httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("want 204 got %d", w.Code)
	}
}

func TestCommandMappings(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"level":"high"}`))
	cmd, _, err := commandFromBody("thinking-level", r)
	if err != nil || cmd["type"] != "set_thinking_level" || cmd["level"] != "high" {
		t.Fatalf("bad cmd %#v err %v", cmd, err)
	}
	r = httptest.NewRequest("POST", "/", strings.NewReader(`{"enabled":true}`))
	cmd, _, _ = commandFromBody("auto-retry", r)
	if cmd["type"] != "set_auto_retry" || cmd["enabled"] != true {
		t.Fatalf("bad bool cmd %#v", cmd)
	}
}

func TestOriginAllowlist(t *testing.T) {
	h := corsMiddleware([]string{"https://app.example"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("want 403 got %d", w.Code)
	}
	r = httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "https://app.example")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("want 204 got %d", w.Code)
	}
	loopback := corsMiddleware([]string{"http://127.0.0.1:8080"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	r = httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "http://localhost:8080")
	w = httptest.NewRecorder()
	loopback.ServeHTTP(w, r)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:8080" {
		t.Fatalf("loopback CORS failed: %d %q", w.Code, w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestConvenienceCommandValidation(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{}`))
	if _, _, err := commandFromBody("prompt", r); err == nil {
		t.Fatal("expected missing prompt validation")
	}
	r = httptest.NewRequest("POST", "/", strings.NewReader(`{"message":"hello","streamingBehavior":"bad"}`))
	if _, _, err := commandFromBody("prompt", r); err == nil {
		t.Fatal("expected streaming validation")
	}
	r = httptest.NewRequest("POST", "/", strings.NewReader(`{"command":"pwd"}`))
	if cmd, _, err := commandFromBody("bash", r); err != nil || cmd["type"] != "bash" {
		t.Fatalf("bad bash validation: %#v %v", cmd, err)
	}
}

func TestWorkerRegistryPersistence(t *testing.T) {
	path := t.TempDir() + "/workers.json"
	r := NewWorkerRegistry(path)
	if err := r.Add(Worker{ID: "w1", URL: "http://x"}); err != nil {
		t.Fatal(err)
	}
	r2 := NewWorkerRegistry(path)
	if err := r2.Load(); err != nil {
		t.Fatal(err)
	}
	if _, ok := r2.Get("w1"); !ok {
		t.Fatal("worker not loaded")
	}
}

func TestCORSEmptyOriginsRejectsCrossOrigin(t *testing.T) {
	// When no origins are configured, cross-origin browser requests should be
	// rejected to prevent malicious webpages from accessing pi-server.
	h := corsMiddleware(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "http://anything.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("want 403 got %d", w.Code)
	}
}

func TestCORSEmptyOriginsPermitsNonBrowser(t *testing.T) {
	// Non-browser clients (no Origin header) should always be permitted.
	h := corsMiddleware(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("want 204 got %d", w.Code)
	}
}

func TestWSTicketConsumeRejectsReuse(t *testing.T) {
	st := newWSTicketStore()
	ticket, _, _ := st.issue("s1", "fp")
	if code := st.consume(ticket, "s1", "fp"); code != "" {
		t.Fatalf("first consume should succeed, got %q", code)
	}
	// Ticket is deleted after successful use, so reuse returns invalid_ticket.
	if code := st.consume(ticket, "s1", "fp"); code != CodeInvalidTicket {
		t.Fatalf("reuse should return %q, got %q", CodeInvalidTicket, code)
	}
}

func TestWSTicketRejectsSessionMismatch(t *testing.T) {
	st := newWSTicketStore()
	ticket, _, _ := st.issue("s1", "fp")
	if code := st.consume(ticket, "s2", "fp"); code != CodeTicketSessionMismatch {
		t.Fatalf("want %q, got %q", CodeTicketSessionMismatch, code)
	}
}

func TestWSTicketRejectsFingerprintMismatch(t *testing.T) {
	st := newWSTicketStore()
	ticket, _, _ := st.issue("s1", "fp")
	if code := st.consume(ticket, "s1", "wrong"); code != CodeInvalidTicket {
		t.Fatalf("want %q, got %q", CodeInvalidTicket, code)
	}
}

func TestWSTicketRejectsExpired(t *testing.T) {
	st := newWSTicketStore()
	ticket, _, _ := st.issue("s1", "fp")
	// Manually expire the ticket.
	key := hashTicket(ticket)
	st.mu.Lock()
	st.tickets[key].expiresAt = time.Now().UTC().Add(-time.Second)
	st.mu.Unlock()
	if code := st.consume(ticket, "s1", "fp"); code != CodeTicketExpired {
		t.Fatalf("want %q, got %q", CodeTicketExpired, code)
	}
}

func TestExternalRegistryConcurrentPublish(t *testing.T) {
	r := newExternalRegistry(t.TempDir() + "/relay-commands.json")
	r.register("ext", ".", "", "", "bridge")
	// Concurrent publishes must not panic or corrupt state.
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				r.publish("ext", RPCEvent{"type": "test", "j": j})
			}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	_, ok := r.get("ext")
	if !ok {
		fatal(t, "session lost")
	}
	if count := r.eventCount("ext"); count != 1000 {
		t.Fatalf("expected 1000 events, got %d", count)
	}
}

func fatal(t *testing.T, msg string) { t.Fatal(msg) }
