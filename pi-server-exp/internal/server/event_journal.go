package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type eventJournal struct {
	file         *os.File
	path         string
	records      int
	bytes        int64
	syncInterval time.Duration
	lastSync     time.Time
}

type persistedEventRecord struct {
	ID        uint64    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Event     RPCEvent  `json:"event"`
}

// syncInterval controls fsync batching. Zero preserves strict per-event durability.
// The optional form preserves compatibility with existing callers and tests.
func openEventJournal(dataDir, sessionID string, syncIntervals ...time.Duration) (*eventJournal, []EventRecord, uint64, error) {
	var syncInterval time.Duration
	if len(syncIntervals) > 0 {
		syncInterval = syncIntervals[0]
	}
	if syncInterval < 0 {
		syncInterval = 0
	}
	if dataDir == "" {
		return nil, nil, 0, nil
	}
	dir := filepath.Join(dataDir, "events")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, nil, 0, err
	}
	path := filepath.Join(dir, safeEventJournalName(sessionID)+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, nil, 0, err
	}
	records, lastID, err := readEventJournal(path)
	if err != nil {
		_ = file.Close()
		return nil, nil, 0, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, 0, err
	}
	return &eventJournal{file: file, path: path, records: len(records), bytes: info.Size(), syncInterval: syncInterval}, records, lastID, nil
}

func safeEventJournalName(sessionID string) string {
	name := strings.TrimSpace(sessionID)
	if name == "" {
		return "session"
	}
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func readEventJournal(path string) ([]EventRecord, uint64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	var records []EventRecord
	var lastID uint64
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxRPCJSONLRecordBytes)
	for scanner.Scan() {
		var persisted persistedEventRecord
		if err := json.Unmarshal(scanner.Bytes(), &persisted); err != nil {
			// A torn final write should not make the whole session unusable.
			if scanner.Err() == nil {
				continue
			}
			return nil, 0, err
		}
		if persisted.ID == 0 {
			continue
		}
		if persisted.ID > lastID {
			lastID = persisted.ID
		}
		encoded, err := json.Marshal(persisted.Event)
		if err != nil {
			continue
		}
		records = append(records, EventRecord{
			ID: persisted.ID, Timestamp: persisted.Timestamp,
			Event: persisted.Event, size: len(encoded),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}
	return records, lastID, nil
}

func (j *eventJournal) append(record EventRecord) error {
	if j == nil || j.file == nil {
		return nil
	}
	data, err := json.Marshal(persistedEventRecord{
		ID: record.ID, Timestamp: record.Timestamp, Event: record.Event,
	})
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := j.file.Write(data); err != nil {
		if errors.Is(err, os.ErrClosed) {
			j.file = nil
			return nil
		}
		return err
	}
	// Strict mode fsyncs every event. A positive interval safely batches fsyncs
	// while preserving ordered append; close and compaction always force a sync.
	if j.syncInterval <= 0 || j.lastSync.IsZero() || time.Since(j.lastSync) >= j.syncInterval {
		if err := j.file.Sync(); err != nil {
			return err
		}
		j.lastSync = time.Now()
	}
	j.records++
	j.bytes += int64(len(data))
	return nil
}

func (j *eventJournal) shouldCompact(maxRecords, maxBytes int) bool {
	if j == nil {
		return false
	}
	return (maxRecords > 0 && j.records > maxRecords*2) || (maxBytes > 0 && j.bytes > int64(maxBytes)*2)
}

func (j *eventJournal) close() error {
	if j == nil || j.file == nil {
		return nil
	}
	if err := j.file.Sync(); err != nil {
		return err
	}
	err := j.file.Close()
	j.file = nil
	return err
}

func (j *eventJournal) compact(records []EventRecord) error {
	if j == nil {
		return nil
	}
	temp, err := os.CreateTemp(filepath.Dir(j.path), ".events-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempName)
	}
	for _, record := range records {
		data, err := json.Marshal(persistedEventRecord{
			ID: record.ID, Timestamp: record.Timestamp, Event: record.Event,
		})
		if err != nil {
			cleanup()
			return err
		}
		if _, err := temp.Write(append(data, '\n')); err != nil {
			cleanup()
			return err
		}
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	if err := j.file.Close(); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	if err := os.Rename(tempName, j.path); err != nil {
		return fmt.Errorf("replace event journal: %w", err)
	}
	file, err := os.OpenFile(j.path, os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	j.file = file
	j.records = len(records)
	j.bytes = 0
	j.lastSync = time.Now()
	if info, statErr := file.Stat(); statErr == nil {
		j.bytes = info.Size()
	}
	return nil
}

var errEventJournalUnavailable = errors.New("event journal unavailable")

const (
	defaultEventJournalMaxTotalBytes = int64(512 << 20)
	defaultEventJournalMaxAge        = 30 * 24 * time.Hour
)

func cleanupEventJournals(dataDir string, activeSessionIDs map[string]struct{}) error {
	dir := filepath.Join(dataDir, "events")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	type journalFile struct {
		path    string
		size    int64
		modTime time.Time
	}
	now := time.Now()
	files := make([]journalFile, 0, len(entries))
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
		if _, active := activeSessionIDs[sessionID]; !active && now.Sub(info.ModTime()) > defaultEventJournalMaxAge {
			_ = os.Remove(path)
			continue
		}
		files = append(files, journalFile{path: path, size: info.Size(), modTime: info.ModTime()})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modTime.Before(files[j].modTime) })
	for _, file := range files {
		if total <= defaultEventJournalMaxTotalBytes {
			break
		}
		if _, active := activeSessionIDs[strings.TrimSuffix(filepath.Base(file.path), ".jsonl")]; active {
			continue
		}
		if err := os.Remove(file.path); err == nil {
			total -= file.size
		}
	}
	return nil
}

func removeEventJournal(dataDir, sessionID string) error {
	path := filepath.Join(dataDir, "events", safeEventJournalName(sessionID)+".jsonl")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
