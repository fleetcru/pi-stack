package server

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRestartRecoveryPreservesEventCursorAndCommandReceipt(t *testing.T) {
	dataDir := t.TempDir()
	journal, _, _, err := openEventJournal(dataDir, "restart-session")
	if err != nil {
		t.Fatal(err)
	}
	record := EventRecord{ID: 41, Timestamp: time.Now().UTC(), Event: RPCEvent{"type": "agent_end"}}
	if err := journal.append(record); err != nil {
		t.Fatal(err)
	}
	if err := journal.close(); err != nil {
		t.Fatal(err)
	}

	receipts := newCommandReceiptStore(filepath.Join(dataDir, "command-receipts.json"))
	receipts.put("restart-session:key", "command-41", time.Minute)

	reopened, restored, lastID, err := openEventJournal(dataDir, "restart-session")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.close()
	if lastID != 41 || len(restored) != 1 || restored[0].ID != 41 {
		t.Fatalf("event cursor was not restored: last=%d records=%+v", lastID, restored)
	}
	reloadedReceipts := newCommandReceiptStore(filepath.Join(dataDir, "command-receipts.json"))
	receipt, ok := reloadedReceipts.get("restart-session:key")
	if !ok || receipt.CommandID != "command-41" {
		t.Fatalf("command receipt was not restored: %+v, ok=%v", receipt, ok)
	}
}
