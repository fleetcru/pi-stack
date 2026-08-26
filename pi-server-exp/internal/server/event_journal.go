package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type eventJournal struct {
	file    *os.File
	path    string
	records int
	bytes   int64
}

type persistedEventRecord struct {
	ID        uint64          `json:"id"`
	Timestamp time.Time       `json:"timestamp"`
	Event     RPCEvent        `json:"event"`
}

func openEventJournal(dataDir, sessionID string) (*eventJournal, []EventRecord, uint64, error) {
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
	return &eventJournal{file: file, path: path, records: len(records), bytes: info.Size()}, records, lastID, nil
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
		return err
	}
	// Journal writes are explicitly durable before the event is published to
	// subscribers. This makes a reconnect after a daemon crash deterministic.
	if err := j.file.Sync(); err != nil {
		return err
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
	return j.file.Close()
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
	if info, statErr := file.Stat(); statErr == nil {
		j.bytes = info.Size()
	}
	return nil
}

var errEventJournalUnavailable = errors.New("event journal unavailable")
