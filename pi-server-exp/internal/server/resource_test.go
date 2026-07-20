package server

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestShouldWatchDirSkipsExpensiveTrees(t *testing.T) {
	for _, path := range []string{"project/node_modules", "project/.git", "project/.next", "project/dist"} {
		if shouldWatchDir(path) {
			t.Fatalf("expected %q to be skipped", path)
		}
	}
	if !shouldWatchDir("project/internal") {
		t.Fatal("source directory should be watched")
	}
}

func TestListAllSessionsFetchesWorkerInventoryOnce(t *testing.T) {
	var calls atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sessions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{"sessions": []SessionSpec{{ID: "one"}, {ID: "two"}}})
	}))
	defer remote.Close()

	s := New(Config{CWD: t.TempDir(), DataDir: t.TempDir()}, testLogger())
	if err := s.workers.Add(Worker{ID: "remote", URL: remote.URL}); err != nil {
		t.Fatal(err)
	}
	for _, rec := range []RemoteSession{{ID: "global-one", WorkerID: "remote", WorkerSessionID: "one"}, {ID: "global-two", WorkerID: "remote", WorkerSessionID: "two"}} {
		if err := s.remoteSessions.Add(rec); err != nil {
			t.Fatal(err)
		}
	}
	w := httptest.NewRecorder()
	s.listSessions(w, httptest.NewRequest(http.MethodGet, "/v1/sessions?scope=all", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected one worker inventory request, got %d", got)
	}
}
