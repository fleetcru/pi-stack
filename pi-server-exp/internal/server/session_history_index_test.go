package server

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryInvalidationIsScopedToSession(t *testing.T) {
	s := &Server{
		historyCache:      map[string]historyCacheEntry{"one": {}, "two": {}},
		historyIndexes:    map[string]historyIndex{"one.jsonl": {}, "two.jsonl": {}},
		historyIndexPaths: map[string]string{"one": "one.jsonl", "two": "two.jsonl"},
	}
	s.invalidateHistoryCache("one")
	if _, ok := s.historyIndexes["one.jsonl"]; ok {
		t.Fatal("stale session index retained")
	}
	if _, ok := s.historyIndexes["two.jsonl"]; !ok {
		t.Fatal("unrelated session index evicted")
	}
}

func TestIndexedHistoryReadsNewestPageAtMessageBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	content := "{\"type\":\"session\",\"id\":\"s\"}\n" +
		"{\"type\":\"message\",\"message\":{\"role\":\"user\",\"content\":\"one\"}}\n" +
		"{\"type\":\"model_change\",\"modelId\":\"m\"}\n" +
		"{\"type\":\"message\",\"message\":{\"role\":\"assistant\",\"content\":\"two\"}}\n" +
		"{\"type\":\"message\",\"message\":{\"role\":\"toolResult\",\"content\":[]}}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{logger: slog.Default(), historyIndexes: map[string]historyIndex{}}
	page, total, err := s.readIndexedHistoryPage(path, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(page) != 2 {
		t.Fatalf("total=%d page=%d", total, len(page))
	}
	first := page[0].(map[string]any)
	if first["role"] != "assistant" {
		t.Fatalf("unexpected first page item: %#v", first)
	}
	older, _, err := s.readIndexedHistoryPage(path, 2, 2)
	if err != nil || len(older) != 1 {
		t.Fatalf("older=%#v err=%v", older, err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(content, `"type":"message"`, `"type":"session"`, 1)
	if len(changed) != len(content) {
		t.Fatal("identity test must preserve size")
	}
	if err := os.WriteFile(path, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	// Simulate a daemon restart so validation loads the persisted identity rather
	// than trusting the already-open process's metadata cache.
	s.historyIndexes = map[string]historyIndex{}
	_, total, err = s.readIndexedHistoryPage(path, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("same-metadata rewrite reused stale index: total=%d", total)
	}
}
