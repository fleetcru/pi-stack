package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func continuationFixture(t *testing.T) (*Server, func()) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	s := newTestServer(t, "")
	s.cfg.PiBinary = fakePiBinary()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
		for _, spec := range s.sessions.ListSpecs() {
			s.releaseHistoryOwner(spec)
		}
	})
	id := "continue-test"
	path := filepath.Join(s.cfg.DataDir, "pi-sessions", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	header, _ := json.Marshal(map[string]any{"type": "session", "version": 3, "id": id, "cwd": s.cfg.CWD})
	if err := os.WriteFile(path, append(header, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(fakePiBinary())
	cmd.Env = append(os.Environ(), fakePiChildEnv+"=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	identity, started, err := bridgeProcessStart(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Skipf("process identity unsupported: %v", err)
	}
	if identity == "" {
		t.Fatal("empty kernel process identity")
	}
	var once sync.Once
	stop := func() { once.Do(func() { _ = stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }) }
	t.Cleanup(stop)
	proof := bridgeProcessProof{BridgeID: "proof-instance", PID: cmd.Process.Pid, StartedAt: started.UnixMilli(), SessionPath: path}
	proofPath := filepath.Join(home, ".pi", "agent", "bridge-processes", proof.BridgeID+".json")
	if err := os.MkdirAll(filepath.Dir(proofPath), 0700); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(proof)
	if err := os.WriteFile(proofPath, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	// Seed fields that registration must preserve and the fake child's test mode.
	_, err = s.sessions.RegisterSpec(SessionSpec{ID: id, CWD: s.cfg.CWD, SessionPath: path, Transport: "relay", Title: "original", Project: "project", Env: map[string]string{fakePiChildEnv: "1", "PI_TEST_CONTINUE_RPC": "1"}, Metadata: map[string]string{"custom": "kept"}})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"id": id, "cwd": s.cfg.CWD, "title": "original", "sessionPath": path, "bridgeId": proof.BridgeID, "pid": proof.PID, "startedAt": proof.StartedAt})
	req := httptest.NewRequest(http.MethodPost, "/v1/external-sessions/register", strings.NewReader(string(body)))
	req.RemoteAddr = "127.0.0.1:45678"
	response := httptest.NewRecorder()
	s.externalRegister(response, req)
	if response.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", response.Code, response.Body.String())
	}
	if s.external.stateSnapshot(id)["continueAvailable"] != false {
		t.Fatal("live TUI offered continuation")
	}
	s.external.mu.RLock()
	verified := s.external.sessions[id].bridgePID == proof.PID && s.external.sessions[id].bridgeStartedAt == identity
	s.external.mu.RUnlock()
	if !verified {
		t.Fatal("local process proof was not verified")
	}
	return s, stop
}

func continueRequest(s *Server) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/v1/external-sessions/continue-test/continue", strings.NewReader("{}"))
	response := httptest.NewRecorder()
	serve(s).ServeHTTP(response, request)
	return response
}

func TestContinueExternalSessionGuards(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Server, func())
	}{
		{"disconnected but alive", func(s *Server, stop func()) {}},
		{"still connected", func(s *Server, stop func()) { stop(); s.external.sessions["continue-test"].RelayConnected = true }},
		{"unknown legacy identity", func(s *Server, stop func()) { stop(); s.external.sessions["continue-test"].bridgePID = 0 }},
		{"pending commands", func(s *Server, stop func()) {
			s.external.enqueue("continue-test", ExternalCommand{ID: "pending", Type: "prompt", Message: "hello"})
			stop()
		}},
		{"missing history", func(s *Server, stop func()) { stop(); _ = os.Remove(s.external.sessions["continue-test"].SessionPath) }},
		{"unavailable directory", func(s *Server, stop func()) {
			stop()
			s.external.sessions["continue-test"].CWD = filepath.Join(s.cfg.CWD, "missing")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, stop := continuationFixture(t)
			test.setup(s, stop)
			result := continueRequest(s)
			if result.Code != http.StatusConflict {
				t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
			}
			if _, exists := s.sessions.Get("continue-test"); exists {
				t.Fatal("blocked handoff attached an RPC process")
			}
			spec, _ := s.sessions.GetSpec("continue-test")
			if spec.Transport != "relay" {
				t.Fatal("blocked handoff changed ownership")
			}
		})
	}
}

func TestContinueExternalSessionPreservesIdentity(t *testing.T) {
	s, stop := continuationFixture(t)
	stop()
	if s.external.enqueue("continue-test", ExternalCommand{ID: "too-late", Type: "prompt", Message: "hello"}) {
		t.Fatal("exited TUI accepted a new prompt")
	}
	ch, _, unsubscribe, _ := s.external.subscribe("continue-test", 0)
	defer unsubscribe()
	oldSpec, _ := s.sessions.GetSpec("continue-test")
	result := continueRequest(s)
	if result.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
	}
	spec, _ := s.sessions.GetSpec("continue-test")
	if spec.ID != oldSpec.ID || canonicalPath(spec.SessionPath) != canonicalPath(oldSpec.SessionPath) || canonicalPath(spec.CWD) != canonicalPath(oldSpec.CWD) || spec.Title != oldSpec.Title || spec.Project != oldSpec.Project || spec.Metadata["custom"] != "kept" {
		t.Fatalf("metadata/history lost: %#v", spec)
	}
	if spec.Transport != "rpc" || len(spec.Args) != 2 || spec.Args[0] != "--session" || canonicalPath(spec.Args[1]) != canonicalPath(oldSpec.SessionPath) {
		t.Fatalf("incorrect resume args: %#v", spec)
	}
	if _, exists := s.external.get(spec.ID); exists {
		t.Fatal("relay still routes requests after handoff")
	}
	if _, open := <-ch; open {
		t.Fatal("viewer was not disconnected for new transport")
	}
	p, exists := s.sessions.Get(spec.ID)
	if !exists {
		t.Fatal("missing RPC process")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := p.Request(ctx, RPCCommand{"type": "prompt", "message": "continue"}); err != nil {
		t.Fatalf("resumed RPC unusable: %v", err)
	}
	if retry := continueRequest(s); retry.Code != http.StatusOK {
		t.Fatalf("idempotent retry failed: %s", retry.Body.String())
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/external-sessions/register", strings.NewReader(`{"id":"continue-test","cwd":".","bridgeId":"old"}`))
	response := httptest.NewRecorder()
	s.externalRegister(response, req)
	if response.Code != http.StatusConflict {
		t.Fatal("late bridge reclaimed RPC ownership")
	}
	// The durable registry must use the RPC owner after restart.
	registry := NewSessionRegistry(s.sessions.path, 4)
	if err := registry.Load(); err != nil {
		t.Fatal(err)
	}
	restored, _ := registry.GetSpec(spec.ID)
	if restored.Transport != "rpc" || restored.SessionPath != spec.SessionPath {
		t.Fatal("handoff was not persisted")
	}
}

func TestContinueExternalSessionRollbackAndRetry(t *testing.T) {
	s, stop := continuationFixture(t)
	stop()
	binary := s.cfg.PiBinary
	s.cfg.PiBinary = filepath.Join(t.TempDir(), "missing-pi")
	result := continueRequest(s)
	if result.Code != http.StatusBadGateway {
		t.Fatalf("startup failure status=%d body=%s", result.Code, result.Body.String())
	}
	spec, _ := s.sessions.GetSpec("continue-test")
	if spec.Transport != "relay" || s.external.stateSnapshot(spec.ID)["continueAvailable"] != true {
		t.Fatal("failed startup did not restore relay")
	}
	s.cfg.PiBinary = binary
	if result := continueRequest(s); result.Code != http.StatusOK {
		t.Fatalf("retry failed: %s", result.Body.String())
	}
}

func TestContinueWithInstalledPi(t *testing.T) {
	binary := os.Getenv("PI_TEST_REAL_PI")
	if binary == "" {
		t.Skip("set PI_TEST_REAL_PI to exercise the installed CLI without a model prompt")
	}
	s, stop := continuationFixture(t)
	stop()
	specBefore, _ := s.sessions.GetSpec("continue-test")
	file, err := os.OpenFile(specBefore.SessionPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	entry := map[string]any{"type": "message", "id": "previous-user-entry", "parentId": nil, "timestamp": time.Now().UTC().Format(time.RFC3339), "message": map[string]any{"role": "user", "content": []map[string]any{{"type": "text", "text": "A previous conversation message"}}, "timestamp": time.Now().UnixMilli()}}
	err = json.NewEncoder(file).Encode(entry)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.PiBinary = binary
	s.cfg.RequestTimeout = 45 * time.Second
	result := continueRequest(s)
	if result.Code != http.StatusOK {
		t.Fatalf("installed Pi continuation failed: %d %s", result.Code, result.Body.String())
	}
	spec, _ := s.sessions.GetSpec("continue-test")
	process, _ := s.sessions.Get(spec.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	state, err := process.Request(ctx, RPCCommand{"type": "get_state"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := state["data"].(map[string]any)
	if data["sessionId"] != spec.ID || canonicalPath(data["sessionFile"].(string)) != canonicalPath(spec.SessionPath) {
		t.Fatalf("wrong installed Pi history: %#v", data)
	}
	messages, err := process.Request(ctx, RPCCommand{"type": "get_messages"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(messages["data"])
	if !strings.Contains(string(encoded), "A previous conversation message") {
		t.Fatalf("conversation was not resumed: %s", encoded)
	}
}

func TestContinueRejectsWrongRPCSession(t *testing.T) {
	s, stop := continuationFixture(t)
	stop()
	s.sessions.mu.Lock()
	spec := s.sessions.specs["continue-test"]
	spec.Env["PI_TEST_WRONG_SESSION"] = "1"
	s.sessions.specs[spec.ID] = spec
	s.sessions.mu.Unlock()
	result := continueRequest(s)
	if result.Code != http.StatusBadGateway {
		t.Fatalf("wrong session accepted: %d %s", result.Code, result.Body.String())
	}
	restored, _ := s.sessions.GetSpec(spec.ID)
	if restored.Transport != "relay" {
		t.Fatal("wrong history published as owner")
	}
}

func TestContinueExternalSessionConcurrent(t *testing.T) {
	s, stop := continuationFixture(t)
	stop()
	results := make(chan int, 2)
	for range 2 {
		go func() { results <- continueRequest(s).Code }()
	}
	for range 2 {
		if code := <-results; code != http.StatusOK {
			t.Fatalf("concurrent continuation status=%d", code)
		}
	}
	if len(s.sessions.sessions) != 1 {
		t.Fatal("multiple processes attached")
	}
}

func TestContinueWhilePublishing(t *testing.T) {
	s, stop := continuationFixture(t)
	stop()
	_, _, unsubscribe, _ := s.external.subscribe("continue-test", 0)
	defer unsubscribe()
	done := make(chan struct{})
	var writers sync.WaitGroup
	writers.Add(1)
	go func() {
		defer writers.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			s.external.publish("continue-test", RPCEvent{"type": "heartbeat"})
		}
	}()
	result := continueRequest(s)
	close(done)
	writers.Wait()
	if result.Code != http.StatusOK {
		t.Fatalf("concurrent publish handoff failed: %s", result.Body.String())
	}
}

func TestContinueRegistryPersistenceFailure(t *testing.T) {
	s, stop := continuationFixture(t)
	stop()
	original := s.sessions.path
	s.sessions.path = t.TempDir() // A directory cannot be replaced by a registry file.
	result := continueRequest(s)
	if result.Code != http.StatusInternalServerError {
		t.Fatalf("persistence failure status=%d body=%s", result.Code, result.Body.String())
	}
	spec, _ := s.sessions.GetSpec("continue-test")
	if spec.Transport != "relay" {
		t.Fatal("persistence failure published RPC ownership")
	}
	if _, present := s.sessions.Get(spec.ID); present {
		t.Fatal("failed persistence left RPC attached")
	}
	s.sessions.path = original
	if retry := continueRequest(s); retry.Code != http.StatusOK {
		t.Fatalf("persistence retry failed: %s", retry.Body.String())
	}
}

func TestContinueOriginalProjectOutsideServerRoots(t *testing.T) {
	s, stop := continuationFixture(t)
	stop()
	project := t.TempDir()
	s.external.mu.Lock()
	session := s.external.sessions["continue-test"]
	session.CWD = project
	historyPath := session.SessionPath
	s.external.mu.Unlock()
	header, _ := json.Marshal(map[string]any{"type": "session", "version": 3, "id": "continue-test", "cwd": project})
	if err := os.WriteFile(historyPath, append(header, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if result := continueRequest(s); result.Code != http.StatusOK {
		t.Fatalf("original project resume failed: %s", result.Body.String())
	}
	spec, _ := s.sessions.GetSpec("continue-test")
	if canonicalPath(spec.CWD) != canonicalPath(project) {
		t.Fatal("original project changed")
	}
	if s.allowedFilePath(project) == nil {
		t.Fatal("continuation widened file access roots")
	}
}

func TestContinueCapacityGuard(t *testing.T) {
	s, stop := continuationFixture(t)
	stop()
	busy := startFakeProcess(t, "other-process", false)
	if err := s.sessions.Add(busy, busy.spec); err != nil {
		t.Fatal(err)
	}
	s.sessions.mu.Lock()
	s.sessions.maxSessions = 1
	s.sessions.mu.Unlock()
	if result := continueRequest(s); result.Code != http.StatusServiceUnavailable {
		t.Fatalf("capacity status=%d body=%s", result.Code, result.Body.String())
	}
	if s.external.stateSnapshot("continue-test")["continueAvailable"] != true {
		t.Fatal("capacity rejection prevented retry")
	}
}

func TestContinueAfterServerRestart(t *testing.T) {
	s, stop := continuationFixture(t)
	stop()
	old, _ := s.sessions.GetSpec("continue-test")
	// Simulate the old server exiting and releasing its OS file handles.
	s.releaseHistoryOwner(old)
	restored := New(s.cfg, freshTestLogger())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = restored.Shutdown(ctx)
		for _, spec := range restored.sessions.ListSpecs() {
			restored.releaseHistoryOwner(spec)
		}
	})
	if restored.external.stateSnapshot(old.ID)["continueAvailable"] != true {
		t.Fatal("verified exit not restored")
	}
	if result := continueRequest(restored); result.Code != http.StatusOK {
		t.Fatalf("restored continuation failed: %s", result.Body.String())
	}
}

func TestRemoteRegistrationCannotAuthorizeContinuation(t *testing.T) {
	s, stop := continuationFixture(t)
	external, _ := s.external.get("continue-test")
	home, _ := os.UserHomeDir()
	raw, err := os.ReadFile(filepath.Join(home, ".pi", "agent", "bridge-processes", "proof-instance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	body["id"], body["cwd"] = external.ID, external.CWD
	encoded, _ := json.Marshal(body)
	request := httptest.NewRequest(http.MethodPost, "/v1/external-sessions/register", strings.NewReader(string(encoded)))
	request.RemoteAddr = "192.0.2.1:4567"
	response := httptest.NewRecorder()
	s.externalRegister(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("normal remote relay registration failed: %s", response.Body.String())
	}
	stop()
	if result := continueRequest(s); result.Code != http.StatusConflict {
		t.Fatal("remote PID was treated as local proof")
	}
}

func TestContinuationFencesDelivery(t *testing.T) {
	s, stop := continuationFixture(t)
	defer stop()
	s.external.mu.Lock()
	session := s.external.sessions["continue-test"]
	session.transferring = true
	lease := session.leaseToken
	s.external.mu.Unlock()
	if s.external.enqueue(session.ID, ExternalCommand{ID: "late", Type: "prompt"}) {
		t.Fatal("command entered a transferring relay")
	}
	if s.external.publish(session.ID, RPCEvent{"type": "agent_start"}) {
		t.Fatal("event entered a transferring relay")
	}
	if _, _, _, _, _, _, authorized := s.external.attachRelay(session.ID, lease); authorized {
		t.Fatal("relay attached during handoff")
	}
	if _, _, authorized := s.external.commandsFor(session.ID, lease); authorized {
		t.Fatal("relay polled during handoff")
	}
}

func TestLocalBridgeProofRejectsRemoteOrMismatchedIdentity(t *testing.T) {
	s, stop := continuationFixture(t)
	defer stop()
	session, _ := s.external.get("continue-test")
	proof := bridgeProcessProof{BridgeID: "proof-instance", PID: os.Getpid(), StartedAt: time.Now().UnixMilli(), SessionPath: session.SessionPath}
	if pid, _ := localBridgeProcess(proof); pid != 0 {
		t.Fatal("mismatched on-disk proof accepted")
	}
	for _, address := range []string{"192.0.2.1:1234", "not-an-address"} {
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.RemoteAddr = address
		if isLocalBridgeRequest(request) {
			t.Fatalf("remote peer %s accepted", address)
		}
	}
}
