package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
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

	messages, total, err := readRelayMessagesPage(path, 0, 40)
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
