package server

import (
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRelayMessagesPageReturnsBoundedNewestPage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for i := 0; i < 100; i++ {
		if err := encoder.Encode(map[string]any{"type": "message", "message": map[string]any{"role": "user", "content": i}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	server := &Server{historyIndexes: map[string]historyIndex{}, logger: slog.Default()}
	messages, total, err := server.readIndexedRelayMessagesPage(path, 0, 40)
	if err != nil {
		t.Fatal(err)
	}
	if total != 100 || len(messages) != 40 {
		t.Fatalf("total=%d page=%d", total, len(messages))
	}
	first := messages[0].(map[string]any)
	if first["content"] != float64(60) {
		t.Fatalf("unexpected first message: %#v", first)
	}

	response := relayMessagesPageResponse(messages, total, 0, 40)
	recorder := httptest.NewRecorder()
	writeJSON(recorder, 200, response)
	if recorder.Code != 200 {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	data := response["data"].(map[string]any)
	if len(data["messages"].([]any)) != 40 {
		t.Fatalf("response page was re-sliced: %#v", data["messages"])
	}
}

func TestIndexedRelayHistoryIncludesToolRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	content := "{\"type\":\"message\",\"message\":{\"role\":\"user\",\"content\":\"one\"}}\n" +
		"{\"type\":\"tool_use\",\"message\":{\"id\":\"call-1\",\"name\":\"bash\"}}\n" +
		"{\"type\":\"tool_result\",\"message\":{\"toolCallId\":\"call-1\",\"content\":\"done\"}}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil { t.Fatal(err) }
	server := &Server{historyIndexes: map[string]historyIndex{}, logger: slog.Default()}
	messages, total, err := server.readIndexedRelayMessagesPage(path, 0, 10)
	if err != nil { t.Fatal(err) }
	if total != 3 || len(messages) != 3 { t.Fatalf("total=%d messages=%#v", total, messages) }
	if historyType, _ := messages[1].(map[string]any)["_historyType"].(string); historyType != "tool_use" {
		t.Fatalf("tool history type not preserved: %#v", messages[1])
	}
}

func TestIndexedRelayHistoryHandlesLongRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	entry, err := json.Marshal(map[string]any{"type": "message", "message": map[string]any{"role": "user", "content": strings.Repeat("x", 8<<10)}})
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(path, append(entry, '\n'), 0o600); err != nil { t.Fatal(err) }
	server := &Server{historyIndexes: map[string]historyIndex{}, logger: slog.Default()}
	messages, total, err := server.readIndexedRelayMessagesPage(path, 0, 10)
	if err != nil { t.Fatal(err) }
	if total != 1 || len(messages) != 1 { t.Fatalf("total=%d messages=%d", total, len(messages)) }
}
