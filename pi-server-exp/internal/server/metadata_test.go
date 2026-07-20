package server

import "testing"

func TestSessionMetadataPersistence(t *testing.T) {
	path := t.TempDir() + "/sessions.json"
	r := NewSessionRegistry(path, 0)
	spec := SessionSpec{ID: "s", CWD: "."}
	if err := r.Add(NewPiProcess(spec, Config{}, testLogger()), spec); err != nil {
		t.Fatal(err)
	}
	title, project := "Build", "Server"
	labels := []string{"go", "priority"}
	updated, err := r.UpdateMetadata("s", SessionMetadataUpdate{Title: &title, Project: &project, Labels: &labels, Metadata: &map[string]string{"ownerTeam": "personal"}})
	if err != nil || updated.Title != "Build" || len(updated.Labels) != 2 {
		t.Fatalf("bad update %#v %v", updated, err)
	}
	r2 := NewSessionRegistry(path, 0)
	if err := r2.Load(); err != nil {
		t.Fatal(err)
	}
	stored, _ := r2.GetSpec("s")
	if stored.Project != "Server" || stored.Metadata["ownerTeam"] != "personal" {
		t.Fatalf("bad persistence %#v", stored)
	}
}
