package server

import "testing"

func TestRemoteSessionPersistence(t *testing.T) {
	path := t.TempDir() + "/remote.json"
	r := NewRemoteSessionRegistry(path)
	if err := r.Add(RemoteSession{ID: "global", WorkerID: "worker", WorkerSessionID: "remote"}); err != nil {
		t.Fatal(err)
	}
	r2 := NewRemoteSessionRegistry(path)
	if err := r2.Load(); err != nil {
		t.Fatal(err)
	}
	got, ok := r2.Get("global")
	if !ok || got.WorkerSessionID != "remote" {
		t.Fatalf("bad restored mapping: %#v", got)
	}
}
