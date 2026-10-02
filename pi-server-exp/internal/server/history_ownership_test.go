package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseHistoryOwnerAfterHistoryRemoval(t *testing.T) {
	tests := []struct {
		name         string
		changedPath  bool
		wrongOwner   bool
		wantReleased bool
	}{
		{name: "original path", wantReleased: true},
		{name: "unresolvable path spelling", changedPath: true, wantReleased: true},
		{name: "another session cannot release claim", changedPath: true, wrongOwner: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			s := &Server{cfg: Config{DataDir: directory}, historyOwners: map[string]string{}, historyOwnerLocks: map[string]*os.File{}}
			original := SessionSpec{ID: "history-owner", SessionPath: filepath.Join(directory, "history.jsonl")}
			if err := os.WriteFile(original.SessionPath, []byte("{}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := s.reserveHistoryOwner(original); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { s.releaseHistoryOwner(original) })
			key := historyOwnerKey(original)
			lockPath := s.historyOwnerLocks[key].Name()
			if err := os.Remove(original.SessionPath); err != nil {
				t.Fatal(err)
			}
			release := original
			if test.changedPath {
				// Models a Windows short-name path whose long form cannot be recovered
				// once the target disappears. The original claim still has its owner ID.
				release.SessionPath = filepath.Join(directory, "HISTOR~1.JSONL")
			}
			if test.wrongOwner {
				release.ID = "different-owner"
			}
			s.releaseHistoryOwner(release)
			_, remains := s.historyOwners[key]
			if remains == test.wantReleased {
				t.Fatalf("claim remains=%v, want released=%v", remains, test.wantReleased)
			}
			if test.wantReleased {
				if len(s.historyOwnerLocks) != 0 {
					t.Fatal("open lock handle leaked")
				}
				if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
					t.Fatalf("lock file not removed: %v", err)
				}
			}
		})
	}
}
