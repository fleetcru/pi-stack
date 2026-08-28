package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type commandReceipt struct {
	Key       string    `json:"key"`
	CommandID string    `json:"commandId"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type commandReceiptStore struct {
	mu       sync.Mutex
	path     string
	receipts map[string]commandReceipt
	loaded   bool
}

func newCommandReceiptStore(path string) *commandReceiptStore {
	return &commandReceiptStore{path: path, receipts: map[string]commandReceipt{}}
}

func (s *commandReceiptStore) loadLocked() {
	if s.loaded {
		return
	}
	s.loaded = true
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var records []commandReceipt
	if json.Unmarshal(data, &records) != nil {
		return
	}
	now := time.Now()
	for _, record := range records {
		if record.Key != "" && record.CommandID != "" && record.ExpiresAt.After(now) {
			s.receipts[record.Key] = record
		}
	}
}

func (s *commandReceiptStore) saveLocked() {
	records := make([]commandReceipt, 0, len(s.receipts))
	for _, record := range s.receipts {
		records = append(records, record)
	}
	_ = writeJSONAtomic(s.path, records)
}

func (s *commandReceiptStore) get(key string) (commandReceipt, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadLocked()
	record, ok := s.receipts[key]
	if !ok || !record.ExpiresAt.After(time.Now()) {
		if ok {
			delete(s.receipts, key)
			s.saveLocked()
		}
		return commandReceipt{}, false
	}
	return record, true
}

func (s *commandReceiptStore) put(key, commandID string, ttl time.Duration) {
	if key == "" || commandID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadLocked()
	s.receipts[key] = commandReceipt{Key: key, CommandID: commandID, ExpiresAt: time.Now().Add(ttl)}
	s.saveLocked()
}

func (s *commandReceiptStore) pathForTests() string {
	return filepath.Clean(s.path)
}
