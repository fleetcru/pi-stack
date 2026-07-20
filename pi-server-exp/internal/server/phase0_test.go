package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T, auth string) *Server {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{
		Addr:            "127.0.0.1:0",
		DataDir:         dir,
		CWD:             dir,
		AllowedRoots:    []string{dir},
		AuthToken:       auth,
		MaxSessions:     4,
		RequestTimeout:  5 * time.Second,
		ShutdownTimeout: 2 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(cfg, logger)
}

func serve(s *Server) http.Handler {
	return s.httpSrv.Handler
}

func decodeJSON(t *testing.T, r *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil {
		t.Fatalf("json: %v body=%s", err, r.Body.String())
	}
	return out
}

func TestRequestIDAndErrorCode(t *testing.T) {
	s := newTestServer(t, "secret")
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	req.Header.Set("X-Request-ID", "test-req-1")
	w := httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", w.Code)
	}
	if w.Header().Get("X-Request-ID") != "test-req-1" {
		t.Fatalf("request id missing: %q", w.Header().Get("X-Request-ID"))
	}
	body := decodeJSON(t, w)
	if body["code"] != CodeUnauthorized {
		t.Fatalf("code=%v", body["code"])
	}
	if body["error"] == nil {
		t.Fatal("error field missing")
	}
}

func TestCapabilitiesAndHealth(t *testing.T) {
	s := newTestServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	w := httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("capabilities %d %s", w.Code, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["apiVersion"] != APIVersion {
		t.Fatalf("apiVersion=%v", body["apiVersion"])
	}
	caps, ok := body["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("capabilities type %T", body["capabilities"])
	}
	for _, key := range []string{"workerCrud", "sessionInventory", "fileContent", "webSocketTickets"} {
		if caps[key] != true {
			t.Fatalf("missing capability %s", key)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w = httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	body = decodeJSON(t, w)
	if body["ok"] != true {
		t.Fatal("health not ok")
	}
	if body["apiVersion"] != APIVersion {
		t.Fatal("health missing apiVersion")
	}
}

func TestWorkerCRUD(t *testing.T) {
	s := newTestServer(t, "")
	// create
	body := `{"id":"w1","url":"http://127.0.0.1:9999","token":"wt","tags":["a"]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/workers", strings.NewReader(body))
	w := httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	if created["token"] != nil {
		t.Fatal("token must not be returned")
	}
	if created["id"] != "w1" {
		t.Fatalf("id=%v", created["id"])
	}

	// get
	req = httptest.NewRequest(http.MethodGet, "/v1/workers/w1", nil)
	w = httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("get %d", w.Code)
	}

	// update preserve token when omitted
	req = httptest.NewRequest(http.MethodPut, "/v1/workers/w1", strings.NewReader(`{"id":"w1","url":"http://127.0.0.1:9998","tags":["b"]}`))
	w = httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("put %d %s", w.Code, w.Body.String())
	}
	worker, ok := s.workers.Get("w1")
	if !ok || worker.Token != "wt" {
		t.Fatalf("token not preserved: %#v", worker)
	}

	// clear token
	req = httptest.NewRequest(http.MethodPut, "/v1/workers/w1", strings.NewReader(`{"id":"w1","url":"http://127.0.0.1:9998","token":null,"tags":[]}`))
	w = httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("clear token %d %s", w.Code, w.Body.String())
	}
	worker, _ = s.workers.Get("w1")
	if worker.Token != "" {
		t.Fatal("token not cleared")
	}

	// local immutable
	req = httptest.NewRequest(http.MethodDelete, "/v1/workers/local", nil)
	w = httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("local delete want 403 got %d", w.Code)
	}
	bodyMap := decodeJSON(t, w)
	if bodyMap["code"] != CodeLocalWorkerImmutable {
		t.Fatalf("code=%v", bodyMap["code"])
	}

	// conflict when mappings exist
	_ = s.remoteSessions.Add(RemoteSession{ID: "g1", WorkerID: "w1", WorkerSessionID: "r1", CreatedAt: time.Now().UTC()})
	req = httptest.NewRequest(http.MethodDelete, "/v1/workers/w1", nil)
	w = httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("want 409 got %d %s", w.Code, w.Body.String())
	}
	if decodeJSON(t, w)["code"] != CodeWorkerHasActive {
		t.Fatal("expected worker_has_active_sessions")
	}

	// force delete
	req = httptest.NewRequest(http.MethodDelete, "/v1/workers/w1?force=true", nil)
	w = httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("force delete %d %s", w.Code, w.Body.String())
	}
	if _, ok := s.workers.Get("w1"); ok {
		t.Fatal("worker still present")
	}
}

func TestWSTicketLifecycle(t *testing.T) {
	store := newWSTicketStore()
	ticket, exp, err := store.issue("sess-a", tokenFingerprint("secret"))
	if err != nil || ticket == "" || exp.IsZero() {
		t.Fatalf("issue: %v", err)
	}
	// Ticket is deleted on any validation failure (mismatch, expired, etc.)
	// to prevent reuse attempts.
	if code := store.consume(ticket, "sess-b", tokenFingerprint("secret")); code != CodeTicketSessionMismatch {
		t.Fatalf("mismatch code=%s", code)
	}
	// Ticket was deleted after mismatch, so it's now invalid.
	if code := store.consume(ticket, "sess-a", tokenFingerprint("secret")); code != CodeInvalidTicket {
		t.Fatalf("expected invalid_ticket after mismatch, got: %s", code)
	}
	// Issue a new ticket for valid consumption.
	ticket2, _, _ := store.issue("sess-a", tokenFingerprint("secret"))
	if code := store.consume(ticket2, "sess-a", tokenFingerprint("secret")); code != "" {
		t.Fatalf("consume valid: %s", code)
	}
	if code := store.consume(ticket2, "sess-a", tokenFingerprint("secret")); code != CodeInvalidTicket && code != CodeTicketUsed {
		t.Fatalf("reuse code=%s", code)
	}

	// expired
	store2 := newWSTicketStore()
	ticket3, _, _ := store2.issue("s", tokenFingerprint("secret"))
	key := hashTicket(ticket3)
	store2.mu.Lock()
	store2.tickets[key].expiresAt = time.Now().UTC().Add(-time.Second)
	store2.mu.Unlock()
	if code := store2.consume(ticket3, "s", tokenFingerprint("secret")); code != CodeTicketExpired {
		t.Fatalf("expired code=%s", code)
	}
}

func TestWSTicketHTTP(t *testing.T) {
	s := newTestServer(t, "secret")
	// seed a local session spec without starting pi
	spec := SessionSpec{ID: "local-sess", CWD: s.cfg.CWD, Status: "created", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	p := NewPiProcess(spec, s.cfg, s.logger)
	if err := s.sessions.Add(p, spec); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/ws-tickets", strings.NewReader(`{"sessionId":"local-sess"}`))
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("ticket %d %s", w.Code, w.Body.String())
	}
	ticketBody := decodeJSON(t, w)
	ticket, _ := ticketBody["ticket"].(string)
	if ticket == "" {
		t.Fatal("empty ticket")
	}

	// wrong session ticket rejected before upgrade
	req = httptest.NewRequest(http.MethodGet, "/v1/sessions/other/ws?ticket="+ticket, nil)
	w = httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden && w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong session want auth fail got %d %s", w.Code, w.Body.String())
	}
}

func TestFileContentBounded(t *testing.T) {
	s := newTestServer(t, "")
	path := filepath.Join(s.cfg.DataDir, "note.txt")
	if err := os.WriteFile(path, []byte("hello pi-desk"), 0o600); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/files/content?path="+path, nil)
	w := httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("content %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store")
	}
	body := decodeJSON(t, w)
	if body["content"] != "hello pi-desk" {
		t.Fatalf("content=%v", body["content"])
	}
	if body["encoding"] != "utf-8" {
		t.Fatalf("encoding=%v", body["encoding"])
	}

	// binary
	bin := filepath.Join(s.cfg.DataDir, "bin.dat")
	if err := os.WriteFile(bin, []byte{0x00, 0x01, 0x02}, 0o600); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/v1/files/content?path="+bin, nil)
	w = httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	body = decodeJSON(t, w)
	if body["binary"] != true {
		t.Fatalf("expected binary %#v", body)
	}
	if body["content"] != nil {
		t.Fatal("binary non-image must not include content")
	}

	// path outside roots
	req = httptest.NewRequest(http.MethodGet, "/v1/files/content?path=C:\\Windows\\System32\\drivers\\etc\\hosts", nil)
	w = httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		// On non-Windows CI this path may not exist; either forbidden or not found after resolve is ok if not 200
		if w.Code == 200 {
			t.Fatal("escape allowed")
		}
	}
}

func TestSessionInventoryLocalDefault(t *testing.T) {
	s := newTestServer(t, "")
	spec := SessionSpec{ID: "s1", CWD: s.cfg.CWD, Status: "created", Project: "p", Title: "t", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	p := NewPiProcess(spec, s.cfg, s.logger)
	if err := s.sessions.Add(p, spec); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions", nil)
	w := httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var body struct {
		Sessions []SessionSpec `json:"sessions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Sessions) != 1 || body.Sessions[0].ID != "s1" {
		t.Fatalf("%+v", body.Sessions)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/sessions?scope=all", nil)
	w = httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("all %d %s", w.Code, w.Body.String())
	}
	out := decodeJSON(t, w)
	if _, ok := out["partialFailures"]; !ok {
		t.Fatal("partialFailures missing")
	}
}

func TestOpenAPIContainsPhase0Routes(t *testing.T) {
	s := newTestServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	w := httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	for _, p := range []string{
		"/v1/capabilities",
		"/v1/ws-tickets",
		"/v1/workers/{id}",
		"/v1/files/content",
		"/v1/sessions/{id}/summary",
		"/v1/sessions/{id}/files/content",
	} {
		if _, ok := paths[p]; !ok {
			t.Fatalf("openapi missing %s", p)
		}
	}
	// Error schema includes code
	comps, _ := doc["components"].(map[string]any)
	schemas, _ := comps["schemas"].(map[string]any)
	errSchema, _ := schemas["Error"].(map[string]any)
	props, _ := errSchema["properties"].(map[string]any)
	if props["code"] == nil {
		t.Fatal("Error schema missing code")
	}
}

func TestAuthErrorCompatible(t *testing.T) {
	// Existing clients still see "error" string
	h := authMiddleware("secret", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	// wrap with request id
	h = requestIDMiddleware(h)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var body map[string]any
	_ = json.NewDecoder(bytes.NewReader(w.Body.Bytes())).Decode(&body)
	if body["error"] != "unauthorized" {
		t.Fatalf("%v", body)
	}
}
