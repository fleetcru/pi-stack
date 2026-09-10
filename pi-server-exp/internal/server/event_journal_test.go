package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEventJournalRestoresAndCompacts(t *testing.T) {
	dataDir := t.TempDir()
	journal, restored, lastID, err := openEventJournal(dataDir, "session/one", 0)
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

	reopened, restored, lastID, err := openEventJournal(dataDir, "session/one", 0)
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

func TestEventJournalBatchesSyncWhenConfigured(t *testing.T) {
	journal, _, _, err := openEventJournal(t.TempDir(), "batched", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.close()
	if err := journal.append(EventRecord{ID: 1, Timestamp: time.Now().UTC(), Event: RPCEvent{"type": "one"}}); err != nil {
		t.Fatal(err)
	}
	// Appends are queued to the writer goroutine; wait for the first sync.
	deadline := time.Now().Add(2 * time.Second)
	for journal.lastSync.IsZero() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	journal.mu.Lock()
	firstSync := journal.lastSync
	journal.mu.Unlock()
	if firstSync.IsZero() {
		t.Fatal("first append did not sync")
	}
	if err := journal.append(EventRecord{ID: 2, Timestamp: time.Now().UTC(), Event: RPCEvent{"type": "two"}}); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		journal.mu.Lock()
		changed := !journal.lastSync.Equal(firstSync)
		journal.mu.Unlock()
		if changed {
			t.Fatal("second append unexpectedly forced another sync")
		}
		// Give the writer a moment to process the second append; a forced
		// sync would flip lastSync within that window.
		time.Sleep(20 * time.Millisecond)
	}
}

// Regression: appends queued while an asynchronous compaction is rewriting
// the file must land in the new journal in their original order. The single
// writer goroutine processes ops in FIFO order, so the rewrite swaps the file
// before those appends are written.
func TestEventJournalAsyncCompactKeepsSubsequentAppends(t *testing.T) {
	dataDir := t.TempDir()
	journal, _, _, err := openEventJournal(dataDir, "async-compact", 0)
	if err != nil {
		t.Fatal(err)
	}
	appendEvent := func(id uint64, kind string) {
		t.Helper()
		if err := journal.append(EventRecord{ID: id, Timestamp: time.Now().UTC(), Event: RPCEvent{"type": kind}}); err != nil {
			t.Fatal(err)
		}
	}
	for id := uint64(1); id <= 5; id++ {
		appendEvent(id, "before")
	}
	// Fire-and-forget rewrite that drops the first two records, then keep
	// appending while the rewrite is in flight.
	snapshot := []EventRecord{
		{ID: 3, Timestamp: time.Now().UTC(), Event: RPCEvent{"type": "before"}},
		{ID: 4, Timestamp: time.Now().UTC(), Event: RPCEvent{"type": "before"}},
		{ID: 5, Timestamp: time.Now().UTC(), Event: RPCEvent{"type": "before"}},
	}
	journal.requestCompact(snapshot)
	for id := uint64(6); id <= 10; id++ {
		appendEvent(id, "after")
	}
	if err := journal.close(); err != nil {
		t.Fatal(err)
	}

	reopened, restored, lastID, err := openEventJournal(dataDir, "async-compact", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.close()
	if lastID != 10 {
		t.Fatalf("expected last id 10, got %d", lastID)
	}
	if len(restored) != 8 {
		t.Fatalf("expected 8 records (3 retained + 5 appended during rewrite), got %d", len(restored))
	}
	for i, record := range restored {
		want := uint64(i + 3)
		if record.ID != want {
			t.Fatalf("record %d: expected id %d, got %d", i, want, record.ID)
		}
	}
}
