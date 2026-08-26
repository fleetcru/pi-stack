package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEventJournalRestoresAndCompacts(t *testing.T) {
	dataDir := t.TempDir()
	journal, restored, lastID, err := openEventJournal(dataDir, "session/one")
	if err != nil {
		t.Fatal(err)
	}
	if len(restored) != 0 || lastID != 0 {
		t.Fatalf("unexpected initial journal state: %d records, id %d", len(restored), lastID)
	}
	records := []EventRecord{
		{ID: 1, Timestamp: time.Now().UTC(), Event: RPCEvent{"type": "message_start"}, size: 24},
		{ID: 2, Timestamp: time.Now().UTC(), Event: RPCEvent{"type": "message_end"}, size: 22},
	}
	for _, record := range records {
		if err := journal.append(record); err != nil {
			t.Fatal(err)
		}
	}
	if err := journal.compact(records[1:]); err != nil {
		t.Fatal(err)
	}
	if err := journal.close(); err != nil {
		t.Fatal(err)
	}

	reopened, restored, lastID, err := openEventJournal(dataDir, "session/one")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.close()
	if len(restored) != 1 || restored[0].ID != 2 || lastID != 2 {
		t.Fatalf("restored journal mismatch: %+v, last id %d", restored, lastID)
	}
	if got := filepath.Base(reopened.path); got != "session_one.jsonl" {
		t.Fatalf("unexpected journal path %q", got)
	}
}

func TestEventJournalIgnoresTornFinalRecord(t *testing.T) {
	dataDir := t.TempDir()
	dir := filepath.Join(dataDir, "events")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(path, []byte("{\"id\":1,\"event\":{\"type\":\"ok\"}}\n{"), 0o640); err != nil {
		t.Fatal(err)
	}
	records, lastID, err := readEventJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || lastID != 1 {
		t.Fatalf("expected valid prefix to survive, got %+v, last id %d", records, lastID)
	}
}
