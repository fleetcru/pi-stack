package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRemoteLifecycleReplayReleasesShortRun(t *testing.T) {
	upgrader := websocket.Upgrader{}
	workerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sessions/remote/daemon-status" {
			writeJSON(w, http.StatusOK, map[string]any{"latestEventId": 41, "running": true, "runtimeStatus": map[string]any{"state": "idle"}})
			return
		}
		if r.URL.Path == "/v1/sessions/remote/ws" {
			if r.URL.Query().Get("since") != "41" {
				t.Errorf("since=%q", r.URL.Query().Get("since"))
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			_ = conn.WriteJSON(map[string]any{"type": "agent_start", "_daemonEventId": 42})
			_ = conn.WriteJSON(map[string]any{"type": "agent_settled", "_daemonEventId": 43})
			return
		}
		http.NotFound(w, r)
	}))
	defer workerServer.Close()

	s := newTestServer(t, "")
	worker := Worker{ID: "worker", URL: workerServer.URL}
	cursor, ok := s.remoteEventCursor(worker, "remote")
	if !ok {
		t.Fatal("cursor unavailable")
	}
	if cursor != 41 {
		t.Fatalf("cursor=%d", cursor)
	}
	if !s.acquireDistributedRun(context.Background(), "hub", worker.ID) {
		t.Fatal("acquire failed")
	}
	s.subscribeRemoteRun(worker, "remote", "hub", cursor, true)
	deadline := time.Now().Add(2 * time.Second)
	for s.admission.Active() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if s.admission.Active() != 0 {
		t.Fatal("short replayed run did not release admission")
	}
}

func TestDistributedReservationsRestoreAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.json")
	newServer := func() *Server {
		return &Server{
			cfg: Config{DistributedRunTimeout: time.Hour}, logger: testLogger(), admission: NewTaskAdmission(4, 1, 2),
			distributedRuns: map[string]distributedReservation{}, distributedPath: path,
			workers: NewWorkerRegistry(""), remoteSessions: NewRemoteSessionRegistry(""),
		}
	}
	first := newServer()
	if !first.acquireDistributedRun(context.Background(), "relay", "relay:relay") {
		t.Fatal("acquire failed")
	}
	first.setDistributedRunMetadata("relay", "relay", "")
	second := newServer()
	second.restoreDistributedRuns()
	if second.admission.Active() != 1 {
		t.Fatalf("restored active=%d", second.admission.Active())
	}
	second.releaseDistributedRun("relay")
	first.releaseDistributedRun("relay")
}

func TestDistributedAdmissionReleasesFromRelayLifecycle(t *testing.T) {
	s := newTestServer(t, "")
	if !s.acquireDistributedRun(context.Background(), "relay-one", "relay:relay-one") {
		t.Fatal("could not acquire relay run")
	}
	if s.acquireDistributedRun(context.Background(), "relay-one", "relay:relay-one") {
		t.Fatal("duplicate relay run admitted")
	}
	s.external.register("relay-one", t.TempDir(), "", "", "lease")
	if !s.external.publish("relay-one", RPCEvent{"type": "agent_settled"}) {
		t.Fatal("could not publish lifecycle")
	}
	if s.admission.Active() != 0 {
		t.Fatalf("active admission = %d", s.admission.Active())
	}
}

func TestDistributedAdmissionUsesConfigurableTimeout(t *testing.T) {
	s := &Server{cfg: Config{DistributedRunTimeout: 20 * time.Millisecond}, logger: testLogger(), admission: NewTaskAdmission(1, 1, 1), distributedRuns: map[string]distributedReservation{}}
	if !s.acquireDistributedRun(context.Background(), "remote", "worker") {
		t.Fatal("acquire failed")
	}
	deadline := time.Now().Add(time.Second)
	for s.admission.Active() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if s.admission.Active() != 0 {
		t.Fatal("configured timeout did not release")
	}
}

func TestDistributedAdmissionReleaseIsIdempotent(t *testing.T) {
	s := newTestServer(t, "")
	if !s.acquireDistributedRun(context.Background(), "remote-one", "worker-one") {
		t.Fatal("could not acquire remote run")
	}
	s.releaseDistributedRun("remote-one")
	s.releaseDistributedRun("remote-one")
	if s.admission.Active() != 0 {
		t.Fatalf("active admission = %d", s.admission.Active())
	}
}
