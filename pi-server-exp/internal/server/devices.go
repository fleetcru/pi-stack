package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"net/http"
	"strings"
	"sync"
	"time"
)

type deviceRecord struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	TokenHash string    `json:"tokenHash"`
	CreatedAt time.Time `json:"createdAt"`
	LastSeen  time.Time `json:"lastSeen,omitempty"`
	RevokedAt time.Time `json:"revokedAt,omitempty"`
}

type deviceRegistry struct {
	mu      sync.Mutex
	path    string
	devices map[string]deviceRecord
}

func newDeviceRegistry(path string) *deviceRegistry {
	return &deviceRegistry{path: path, devices: map[string]deviceRecord{}}
}

func (r *deviceRegistry) load() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var records []deviceRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return err
	}
	for _, record := range records {
		if record.ID != "" && record.TokenHash != "" {
			r.devices[record.ID] = record
		}
	}
	return nil
}

func (r *deviceRegistry) saveLocked() error {
	records := make([]deviceRecord, 0, len(r.devices))
	for _, record := range r.devices {
		records = append(records, record)
	}
	return writeJSONAtomic(r.path, records)
}

func (r *deviceRegistry) create(name string) (deviceRecord, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "trusted device"
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return deviceRecord{}, "", err
	}
	token := "pi_dev_" + hex.EncodeToString(raw[:])
	sum := sha256.Sum256([]byte(token))
	record := deviceRecord{
		ID: newRequestID(), Name: name, TokenHash: hex.EncodeToString(sum[:]), CreatedAt: time.Now().UTC(),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.devices[record.ID] = record
	if err := r.saveLocked(); err != nil {
		delete(r.devices, record.ID)
		return deviceRecord{}, "", err
	}
	return record, token, nil
}

func (r *deviceRegistry) authenticate(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	sum := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(sum[:])
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, record := range r.devices {
		if record.TokenHash == hash && record.RevokedAt.IsZero() {
			record.LastSeen = time.Now().UTC()
			r.devices[id] = record
			_ = r.saveLocked()
			return id, true
		}
	}
	return "", false
}

func (r *deviceRegistry) list() []deviceRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]deviceRecord, 0, len(r.devices))
	for _, record := range r.devices {
		record.TokenHash = ""
		out = append(out, record)
	}
	return out
}

func (r *deviceRegistry) revoke(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.devices[id]
	if !ok || !record.RevokedAt.IsZero() {
		return false
	}
	record.RevokedAt = time.Now().UTC()
	r.devices[id] = record
	return r.saveLocked() == nil
}

func publicDevice(record deviceRecord) map[string]any {
	return map[string]any{
		"id": record.ID, "name": record.Name, "createdAt": record.CreatedAt,
		"lastSeen": record.LastSeen, "revokedAt": record.RevokedAt,
	}
}

func (s *Server) requireBootstrap(w http.ResponseWriter, r *http.Request) bool {
	if s.cfg.AuthToken == "" || r.Header.Get("Authorization") != "Bearer "+s.cfg.AuthToken {
		writeErrorCode(w, r, http.StatusForbidden, CodeForbidden, "bootstrap authorization required")
		return false
	}
	return true
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	if !s.requireBootstrap(w, r) {
		return
	}
	records := s.devices.list()
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, publicDevice(record))
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": out})
}

func (s *Server) createDevice(w http.ResponseWriter, r *http.Request) {
	if !s.requireBootstrap(w, r) {
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeErrorCode(w, r, http.StatusBadRequest, CodeBadRequest, "invalid request body")
		return
	}
	record, token, err := s.devices.create(input.Name)
	if err != nil {
		writeErrorCode(w, r, http.StatusInternalServerError, CodeInternal, "failed to create device")
		return
	}
	response := publicDevice(record)
	response["token"] = token
	writeJSON(w, http.StatusCreated, response)
}

func (s *Server) revokeDevice(w http.ResponseWriter, r *http.Request) {
	if !s.requireBootstrap(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/devices/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	if !s.devices.revoke(id) {
		writeErrorCode(w, r, http.StatusNotFound, CodeNotFound, "device not found or already revoked")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revoked": id})
}
