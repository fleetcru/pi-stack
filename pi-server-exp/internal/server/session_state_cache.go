package server

import (
	"context"
	"time"
)

type cachedSessionState struct {
	data    map[string]any
	expires time.Time
}

// cachedPiState avoids a get_state RPC per session on every inventory poll.
// State is reconciled frequently enough for UI controls while Pi streaming
// events remain the low-latency source for chat/activity rendering.
func (s *Server) cachedPiState(ctx context.Context, id string, process *PiProcess) (map[string]any, error) {
	now := time.Now()
	s.stateCacheMu.Lock()
	cached, ok := s.stateCache[id]
	s.stateCacheMu.Unlock()
	if ok && now.Before(cached.expires) {
		return cached.data, nil
	}
	response, err := process.Request(ctx, RPCCommand{"type": "get_state"})
	if err != nil {
		return nil, err
	}
	data, _ := response["data"].(map[string]any)
	if data == nil {
		data = map[string]any{}
	}
	s.stateCacheMu.Lock()
	s.stateCache[id] = cachedSessionState{data: data, expires: now.Add(2 * time.Second)}
	// Evict expired entries to prevent unbounded growth. Runs under the
	// write lock but only on the (infrequent) cache-miss path.
	if len(s.stateCache) > 16 {
		for key, entry := range s.stateCache {
			if now.After(entry.expires) && key != id {
				delete(s.stateCache, key)
			}
		}
	}
	s.stateCacheMu.Unlock()
	return data, nil
}
