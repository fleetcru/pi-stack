package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// relaySessionMessages serves the persisted Pi JSONL history for a bridged TUI
// session. It reads only Pi's default user session store, never arbitrary CWD
// files supplied by the extension.
func (s *Server) relaySessionMessages(w http.ResponseWriter, r *http.Request, external ExternalSession) {
	if !isDefaultPiSessionFile(external.SessionPath) {
		writeJSON(w, http.StatusOK, map[string]any{"command": "get_messages", "success": true, "data": map[string]any{"messages": []any{}, "history": map[string]any{"total": 0, "hasOlder": false}}})
		return
	}
	limit := positiveQueryInt(r, "limit", defaultHistoryPageSize, maxHistoryPageSize)
	offset := positiveQueryInt(r, "offset", 0, int(^uint(0)>>1))
	messages, total, err := readRelayMessagesPage(external.SessionPath, offset, limit)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	end := total - offset
	if end < 0 {
		end = 0
	}
	start := end - len(messages)
	if start < 0 {
		start = 0
	}
	writeJSON(w, http.StatusOK, map[string]any{"command": "get_messages", "success": true, "data": map[string]any{
		"messages": messages[start:end],
		"history":  map[string]any{"total": total, "offset": offset, "limit": limit, "hasOlder": start > 0, "nextOffset": offset + (end - start)},
	}})
}

func isDefaultPiSessionFile(path string) bool {
	if path == "" {
		return false
	}
	root, err := defaultMachineSessionRoot()
	if err != nil {
		return false
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return false
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && rel != "" && rel[:1] != "."
}

func relayNumber(value any) float64 {
	switch value := value.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		n, _ := value.Float64()
		return n
	default:
		return 0
	}
}

func relaySessionStats(external ExternalSession) map[string]any {
	usage := external.LastUsage
	tokens := relayNumber(usage["totalTokens"])
	contextWindow := relayNumber(external.Model["contextWindow"])
	percent := 0.0
	if contextWindow > 0 {
		percent = tokens / contextWindow * 100
	}
	return map[string]any{"totalMessages": external.MessageCount, "cost": external.TotalCost, "contextUsage": map[string]any{"tokens": tokens, "contextWindow": contextWindow, "percent": percent}, "approximate": true}
}

// readRelayMessages remains useful to tests and callers that explicitly need
// the full history. HTTP requests use the bounded page reader below.
func readRelayMessages(path string) ([]any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// Guard against multi-GB session files: check size before reading.
	if info, err := file.Stat(); err == nil && info.Size() > 32<<20 {
		return nil, fmt.Errorf("session file too large: %d bytes", info.Size())
	}
	reader := bufio.NewReaderSize(file, 64*1024)
	messages := make([]any, 0)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if message, ok := relayHistoryMessage(line); ok {
				messages = append(messages, message)
			}
		}
		if err != nil {
			break
		}
	}
	return messages, nil
}

// readRelayMessagesPage retains only the requested page plus its offset while
// streaming JSONL. This avoids allocating every historical message just to
// serve the most recent page.
func readRelayMessagesPage(path string, offset, limit int) ([]any, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	if info, err := file.Stat(); err == nil && info.Size() > 32<<20 {
		return nil, 0, fmt.Errorf("session file too large: %d bytes", info.Size())
	}
	keep := offset + limit
	if keep <= 0 {
		return []any{}, 0, nil
	}
	ring := make([]any, keep)
	head, count, total := 0, 0, 0
	reader := bufio.NewReaderSize(file, 64*1024)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			if message, ok := relayHistoryMessage(line); ok {
				ring[(head+count)%keep] = message
				if count < keep {
					count++
				} else {
					head = (head + 1) % keep
				}
				total++
			}
		}
		if readErr != nil {
			break
		}
	}
	available := count - offset
	if available < 0 {
		available = 0
	}
	page := make([]any, available)
	for i := range page {
		page[i] = ring[(head+i)%keep]
	}
	return page, total, nil
}

func relayHistoryMessage(line []byte) (any, bool) {
	var entry struct {
		Type    string          `json:"type"`
		Message json.RawMessage `json:"message"`
	}
	if json.Unmarshal(line, &entry) != nil || len(entry.Message) == 0 {
		return nil, false
	}
	var message any
	if json.Unmarshal(entry.Message, &message) != nil {
		return nil, false
	}
	if entry.Type == "message" {
		return message, true
	}
	if entry.Type == "tool_use" || entry.Type == "tool_result" {
		if object, ok := message.(map[string]any); ok {
			object["_historyType"] = entry.Type
			return object, true
		}
	}
	return nil, false
}
