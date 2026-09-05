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
	"path/filepath"
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
	if p.spec.ManagedSessionDir != "" {
		if messages, total, err := readManagedHistoryPage(p.spec.ManagedSessionDir, limit, offset); err == nil {
			writeJSON(w, http.StatusOK, RPCEvent{"command": "get_messages", "data": map[string]any{
				"messages": messages,
				"history": map[string]any{"total": total, "offset": offset, "limit": limit, "hasOlder": offset+len(messages) < total, "nextOffset": offset + len(messages)},
			}})
			return
		}
	}
	resp := RPCEvent{"command": "get_messages"}
	historyPath := p.spec.SessionPath
	if historyPath == "" && p.spec.ManagedSessionDir != "" {
		// Managed RPC sessions keep their JSONL transcript inside the managed
		// session directory; SessionPath is only populated for attached sessions.
		if entries, err := os.ReadDir(p.spec.ManagedSessionDir); err == nil {
			for i := len(entries) - 1; i >= 0; i-- {
				if !entries[i].IsDir() && filepath.Ext(entries[i].Name()) == ".jsonl" {
					historyPath = filepath.Join(p.spec.ManagedSessionDir, entries[i].Name())
					break
				}
			}
		}
	}
	if historyPath != "" {
		if messages, total, err := s.readIndexedHistoryPage(historyPath, limit, offset); err == nil {
			s.historyMu.Lock()
			s.historyIndexPaths[p.id] = historyPath
			s.historyMu.Unlock()
			writeJSON(w, http.StatusOK, RPCEvent{"command": "get_messages", "data": map[string]any{
				"messages": messages,
				"history":  map[string]any{"total": total, "offset": offset, "limit": limit, "hasOlder": offset+len(messages) < total, "nextOffset": offset + len(messages)},
			}})
			return
		}
	}
	var messages []any
	status := p.Status()
	runtimeStatus, _ := status["runtimeStatus"].(map[string]any)
	runtimeState, _ := runtimeStatus["state"].(string)
	// During an active turn, the cached transcript may predate the latest
	// message deltas. Gap recovery must read Pi's current history instead of
	// re-serving that stale snapshot.
	useCache := runtimeState == "" || runtimeState == "idle"
	if useCache {
		s.historyMu.Lock()
		cached, ok := s.historyCache[p.id]
		if ok && time.Now().Before(cached.expires) {
			messages = cached.messages
		}
		s.historyMu.Unlock()
	}
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
		if useCache {
			if encoded, err := json.Marshal(messages); err == nil && len(encoded) <= maxCachedHistoryBytes {
				s.historyMu.Lock()
				s.historyCache[p.id] = historyCacheEntry{messages: messages, expires: time.Now().Add(20 * time.Second)}
				s.historyMu.Unlock()
			}
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

// readManagedHistoryPage merges all JSONL transcripts because Pi creates a
// new transcript file when a managed process restarts.
func readManagedHistoryPage(dir string, limit, offset int) ([]any, int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil { return nil, 0, err }
	var messages []any
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" { continue }
		file, openErr := os.Open(filepath.Join(dir, entry.Name()))
		if openErr != nil { return nil, 0, openErr }
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64<<10), 8<<20)
		for scanner.Scan() {
			var record struct { Type string `json:"type"`; Message any `json:"message"` }
			if json.Unmarshal(scanner.Bytes(), &record) == nil && record.Type == "message" && record.Message != nil { messages = append(messages, record.Message) }
		}
		scanErr := scanner.Err()
		_ = file.Close()
		if scanErr != nil { return nil, 0, scanErr }
	}
	total := len(messages)
	end := total - offset
	if end < 0 { end = 0 }
	start := end - limit
	if start < 0 { start = 0 }
	return messages[start:end], total, nil
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

func (s *Server) loadIdempotencyLocked() {
	if s.idempotency != nil {
		return
	}
	s.idempotency = make(map[string]time.Time)
	data, err := os.ReadFile(filepath.Join(s.cfg.DataDir, "idempotency.json"))
	if err != nil {
		return
	}
	var persisted map[string]time.Time
	if json.Unmarshal(data, &persisted) == nil {
		now := time.Now()
		for key, expires := range persisted {
			if expires.After(now) {
				s.idempotency[key] = expires
			}
		}
	}
}

func (s *Server) persistIdempotencyLocked() {
	if s.idempotency == nil {
		return
	}
	if err := writeJSONAtomic(filepath.Join(s.cfg.DataDir, "idempotency.json"), s.idempotency); err != nil {
		s.logger.Warn("failed to persist idempotency keys", "error", err)
	}
}

// idempotencySeen reports whether a key was recorded within the TTL window.
func (s *Server) idempotencySeen(key string) bool {
	s.idempotencyMu.Lock()
	defer s.idempotencyMu.Unlock()
	s.loadIdempotencyLocked()
	now := time.Now()
	changed := false
	for k, exp := range s.idempotency {
		if now.After(exp) {
			delete(s.idempotency, k)
			changed = true
		}
	}
	if changed {
		s.persistIdempotencyLocked()
	}
	exp, ok := s.idempotency[key]
	return ok && now.Before(exp)
}

func (s *Server) recordIdempotency(key string) {
	s.idempotencyMu.Lock()
	defer s.idempotencyMu.Unlock()
	s.loadIdempotencyLocked()
	s.idempotency[key] = time.Now().Add(idempotencyTTL)
	s.persistIdempotencyLocked()
}

// checkIdempotency atomically checks and records a key. It is used by local
// command endpoints so a retry from another paired device cannot submit the
// same command twice after a daemon restart.
func (s *Server) checkIdempotency(key string) bool {
	s.idempotencyMu.Lock()
	defer s.idempotencyMu.Unlock()
	s.loadIdempotencyLocked()
	now := time.Now()
	changed := false
	for k, exp := range s.idempotency {
		if now.After(exp) {
			delete(s.idempotency, k)
			changed = true
		}
	}
	if exp, ok := s.idempotency[key]; ok && now.Before(exp) {
		if changed {
			s.persistIdempotencyLocked()
		}
		return true
	}
	s.idempotency[key] = now.Add(idempotencyTTL)
	s.persistIdempotencyLocked()
	return false
}
