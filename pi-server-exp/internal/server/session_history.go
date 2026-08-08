package server

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
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

type historyIndex struct {
	Size     int64   `json:"size"`
	ModTime  int64   `json:"modTime"`
	Identity string  `json:"identity"`
	Offsets  []int64 `json:"offsets"`
}

const persistedHistoryIndexThreshold = 1 << 20

var errHistoryChanged = errors.New("history changed while indexing")

// sessionMessages pages Pi's get_messages response from newest to oldest.
// Pi itself returns the complete transcript, so trimming here prevents clients
// from retaining and rendering an unbounded historical JSONL conversation.
func (s *Server) sessionMessages(w http.ResponseWriter, r *http.Request, p *PiProcess) {
	limit := positiveQueryInt(r, "limit", defaultHistoryPageSize, maxHistoryPageSize)
	offset := positiveQueryInt(r, "offset", 0, int(^uint(0)>>1))
	resp := RPCEvent{"command": "get_messages"}
	if p.spec.SessionPath != "" {
		if messages, total, err := s.readIndexedHistoryPage(p.spec.SessionPath, limit, offset); err == nil {
			s.historyMu.Lock()
			s.historyIndexPaths[p.id] = p.spec.SessionPath
			s.historyMu.Unlock()
			writeJSON(w, http.StatusOK, RPCEvent{"command": "get_messages", "data": map[string]any{
				"messages": messages,
				"history":  map[string]any{"total": total, "offset": offset, "limit": limit, "hasOlder": offset+len(messages) < total, "nextOffset": offset + len(messages)},
			}})
			return
		}
	}
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

func (s *Server) readIndexedHistoryPage(path string, limit, newestOffset int) ([]any, int, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		messages, total, err := s.readIndexedHistoryPageOnce(path, limit, newestOffset)
		if err == nil {
			return messages, total, nil
		}
		lastErr = err
	}
	return nil, 0, lastErr
}

func (s *Server) readIndexedHistoryPageOnce(path string, limit, newestOffset int) ([]any, int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, err
	}
	index, err := s.historyIndexFor(path, info)
	if err != nil {
		return nil, 0, err
	}
	total := len(index.Offsets)
	end := total - newestOffset
	if end < 0 {
		end = 0
	}
	start := end - limit
	if start < 0 {
		start = 0
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	messages := make([]any, 0, end-start)
	for i := start; i < end; i++ {
		if _, err := file.Seek(index.Offsets[i], io.SeekStart); err != nil {
			return nil, 0, err
		}
		line, err := bufio.NewReader(file).ReadBytes('\n')
		if err != nil && err != io.EOF {
			return nil, 0, err
		}
		var entry struct {
			Message any `json:"message"`
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, 0, err
		}
		messages = append(messages, entry.Message)
	}
	finalInfo, err := os.Stat(path)
	if err != nil {
		return nil, 0, err
	}
	if finalInfo.Size() != index.Size || finalInfo.ModTime().UnixNano() != index.ModTime {
		return nil, 0, errHistoryChanged
	}
	return messages, total, nil
}

func (s *Server) historyIndexFor(path string, info os.FileInfo) (historyIndex, error) {
	s.historyMu.Lock()
	cached, ok := s.historyIndexes[path]
	s.historyMu.Unlock()
	// Within one daemon lifetime, filesystem metadata keeps repeated pages fast.
	// Persisted sidecars receive full-content validation below.
	if ok && cached.Size == info.Size() && cached.ModTime == info.ModTime().UnixNano() {
		return cached, nil
	}
	identity, err := historyFileIdentity(path)
	if err != nil {
		return historyIndex{}, err
	}
	var index historyIndex
	if data, err := os.ReadFile(path + ".piidx"); err == nil && json.Unmarshal(data, &index) == nil && index.Size == info.Size() && index.ModTime == info.ModTime().UnixNano() && index.Identity == identity {
		s.historyMu.Lock()
		s.historyIndexes[path] = index
		s.historyMu.Unlock()
		return index, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return historyIndex{}, err
	}
	defer file.Close()
	reader := bufio.NewReaderSize(file, 64<<10)
	var position int64
	for {
		lineStart := position
		prefix := make([]byte, 0, 4<<10)
		for {
			fragment, readErr := reader.ReadSlice('\n')
			position += int64(len(fragment))
			if len(prefix) < cap(prefix) {
				remaining := cap(prefix) - len(prefix)
				if remaining > len(fragment) {
					remaining = len(fragment)
				}
				prefix = append(prefix, fragment[:remaining]...)
			}
			if readErr == bufio.ErrBufferFull {
				continue
			}
			if readErr != nil && readErr != io.EOF {
				return historyIndex{}, readErr
			}
			if historyRecordIsMessage(prefix) {
				index.Offsets = append(index.Offsets, lineStart)
			}
			if readErr == io.EOF {
				position = info.Size()
				break
			}
			break
		}
		if position >= info.Size() {
			break
		}
	}
	finalInfo, err := os.Stat(path)
	if err != nil {
		return historyIndex{}, err
	}
	if finalInfo.Size() != info.Size() || finalInfo.ModTime().UnixNano() != info.ModTime().UnixNano() {
		return historyIndex{}, errHistoryChanged
	}
	index.Size, index.ModTime, index.Identity = info.Size(), info.ModTime().UnixNano(), identity
	s.historyMu.Lock()
	s.historyIndexes[path] = index
	s.historyMu.Unlock()
	if info.Size() >= persistedHistoryIndexThreshold {
		if err := writeJSONAtomic(path+".piidx", index); err != nil {
			s.logger.Debug("history index sidecar unavailable; using memory index", "path", path, "error", err)
		}
	}
	return index, nil
}

func historyFileIdentity(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func historyRecordIsMessage(prefix []byte) bool {
	var header struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(bytes.TrimSpace(prefix), &header) == nil {
		return header.Type == "message"
	}
	// Very large records are intentionally not accumulated. Pi writes type near
	// the start of each JSONL object, so compact the bounded prefix for detection.
	compact := bytes.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, prefix)
	return bytes.Contains(compact, []byte(`"type":"message"`))
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
	// File metadata validation makes stale indexes harmless; evict only this
	// session so activity elsewhere does not force unrelated index rebuilds.
	if path := s.historyIndexPaths[sessionID]; path != "" {
		delete(s.historyIndexes, path)
		delete(s.historyIndexPaths, sessionID)
	}
	s.historyMu.Unlock()
}

const idempotencyTTL = 60 * time.Second

// checkIdempotency returns true if this key was already seen within the TTL
// window. If not, it records the key and returns false.
func (s *Server) checkIdempotency(key string) bool {
	s.idempotencyMu.Lock()
	defer s.idempotencyMu.Unlock()
	if s.idempotency == nil {
		s.idempotency = make(map[string]time.Time)
	}
	now := time.Now()
	// Evict expired entries opportunistically.
	for k, exp := range s.idempotency {
		if now.After(exp) {
			delete(s.idempotency, k)
		}
	}
	if exp, ok := s.idempotency[key]; ok && now.Before(exp) {
		return true
	}
	s.idempotency[key] = now.Add(idempotencyTTL)
	return false
}
