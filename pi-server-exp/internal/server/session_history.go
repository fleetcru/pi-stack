package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

const defaultHistoryPageSize = 75
const maxHistoryPageSize = 150
const maxCachedHistoryBytes = 8 << 20

type historyCacheEntry struct {
	messages []any
	expires  time.Time
}

// sessionMessages pages Pi's get_messages response from newest to oldest.
// Pi itself returns the complete transcript, so trimming here prevents clients
// from retaining and rendering an unbounded historical JSONL conversation.
func (s *Server) sessionMessages(w http.ResponseWriter, r *http.Request, p *PiProcess) {
	limit := positiveQueryInt(r, "limit", defaultHistoryPageSize, maxHistoryPageSize)
	offset := positiveQueryInt(r, "offset", 0, int(^uint(0)>>1))
	resp := RPCEvent{"command": "get_messages"}
	var messages []any
	s.historyMu.Lock()
	cached, ok := s.historyCache[p.id]
	if ok && time.Now().Before(cached.expires) {
		messages = cached.messages
	}
	s.historyMu.Unlock()
	if messages == nil {
		ctx, cancel := requestContext(r.Context(), s.cfg.RequestTimeout)
		defer cancel()
		var err error
		resp, err = p.Request(ctx, RPCCommand{"type": "get_messages"})
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		data, _ := resp["data"].(map[string]any)
		messages, _ = data["messages"].([]any)
		// Cache only modest histories: caching giant transcripts merely moves the
		// browser memory problem into the daemon.
		if encoded, err := json.Marshal(messages); err == nil && len(encoded) <= maxCachedHistoryBytes {
			s.historyMu.Lock()
			s.historyCache[p.id] = historyCacheEntry{messages: messages, expires: time.Now().Add(20 * time.Second)}
			s.historyMu.Unlock()
		}
	}
	data, _ := resp["data"].(map[string]any)
	total := len(messages)
	end := total - offset
	if end < 0 {
		end = 0
	}
	start := end - limit
	if start < 0 {
		start = 0
	}
	page := messages[start:end]
	if data == nil {
		data = map[string]any{}
		resp["data"] = data
	}
	data["messages"] = page
	data["history"] = map[string]any{
		"total": total, "offset": offset, "limit": limit,
		"hasOlder": start > 0, "nextOffset": offset + len(page),
	}
	writeJSON(w, http.StatusOK, resp)
}

func positiveQueryInt(r *http.Request, key string, fallback, maximum int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil || value < 0 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

// invalidateHistoryCache removes the cached history for a session, forcing
// the next request to fetch fresh data from Pi. Called when a message_end
// event is dispatched to ensure REST history requests reflect new messages.
func (s *Server) invalidateHistoryCache(sessionID string) {
	s.historyMu.Lock()
	delete(s.historyCache, sessionID)
	s.historyMu.Unlock()
}
