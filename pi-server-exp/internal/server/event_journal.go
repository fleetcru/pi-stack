package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type eventJournal struct {
	mu           sync.Mutex
	cond         *sync.Cond
	file         *os.File
	path         string
	records      int
	bytes        int64
	syncInterval time.Duration
	lastSync     time.Time
	logger       *slog.Logger
	// ops is the pending write queue drained by a single writer goroutine.
	// Appends and compactions are serialized through it so the dispatch path
	// never blocks on disk I/O, and no two goroutines touch the file.
	ops     []journalOp
	writer  bool
	stopped bool
	wg      sync.WaitGroup
}

type journalOp struct {
	record  *EventRecord
	compact []EventRecord
	done    chan error
}

type persistedEventRecord struct {
	ID        uint64    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Event     RPCEvent  `json:"event"`
}

// openEventJournal opens (or creates) the append-only journal for a session.
// syncInterval controls fsync batching. Zero preserves strict per-event
// durability. The optional logger receives asynchronous write failures.
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
	j := &eventJournal{file: file, path: path, records: len(records), bytes: info.Size(), syncInterval: syncInterval}
	j.cond = sync.NewCond(&j.mu)
	return j, records, lastID, nil
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

// append queues a record for the writer goroutine and returns immediately.
// Disk failures are reported through the journal logger, not the caller.
func (j *eventJournal) append(record EventRecord) error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	if j.stopped {
		j.mu.Unlock()
		return nil
	}
	j.enqueueLocked(journalOp{record: &record})
	j.mu.Unlock()
	return nil
}

// requestCompact schedules a journal rewrite without waiting. The dispatch
// path uses this so a multi-second compaction never stalls event processing.
func (j *eventJournal) requestCompact(records []EventRecord) {
	if j == nil {
		return
	}
	j.mu.Lock()
	if j.stopped {
		j.mu.Unlock()
		return
	}
	j.enqueueLocked(journalOp{compact: records})
	j.mu.Unlock()
}

// compact schedules a rewrite and waits for it. Startup and tests use this.
func (j *eventJournal) compact(records []EventRecord) error {
	if j == nil {
		return nil
	}
	done := make(chan error, 1)
	j.mu.Lock()
	if j.stopped {
		j.mu.Unlock()
		return nil
	}
	j.enqueueLocked(journalOp{compact: records, done: done})
	j.mu.Unlock()
	return <-done
}

// enqueueLocked starts the writer on first use and wakes it. Caller holds mu.
func (j *eventJournal) enqueueLocked(op journalOp) {
	j.ops = append(j.ops, op)
	if !j.writer {
		j.writer = true
		j.wg.Add(1)
		go j.writerLoop()
	}
	j.cond.Signal()
}

// writerLoop is the only goroutine that touches the journal file.
func (j *eventJournal) writerLoop() {
	defer j.wg.Done()
	for {
		j.mu.Lock()
		for len(j.ops) == 0 && !j.stopped {
			j.cond.Wait()
		}
		if len(j.ops) == 0 && j.stopped {
			file := j.file
			j.file = nil
			j.writer = false
			j.mu.Unlock()
			if file != nil {
				_ = file.Sync()
				_ = file.Close()
			}
			return
		}
		op := j.ops[0]
		j.ops = j.ops[1:]
		file := j.file
		j.mu.Unlock()

		var err error
		switch {
		case op.record != nil:
			err = j.writeRecord(file, *op.record)
		case op.compact != nil:
			err = j.rewrite(file, op.compact)
		}
		if op.done != nil {
			op.done <- err
			close(op.done)
		} else if err != nil && j.logger != nil {
			j.logger.Warn("event journal write failed", "path", j.path, "error", err)
		}
	}
}

// writeRecord marshals and appends one record. Runs on the writer goroutine.
func (j *eventJournal) writeRecord(file *os.File, record EventRecord) error {
	if file == nil {
		return nil
	}
	data, err := json.Marshal(persistedEventRecord{
		ID: record.ID, Timestamp: record.Timestamp, Event: record.Event,
	})
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := file.Write(data); err != nil {
		if errors.Is(err, os.ErrClosed) {
			j.mu.Lock()
			j.file = nil
			j.mu.Unlock()
			return nil
		}
		return err
	}
	j.mu.Lock()
	syncInterval, lastSync := j.syncInterval, j.lastSync
	j.records++
	j.bytes += int64(len(data))
	j.mu.Unlock()
	// Strict mode fsyncs every event. A positive interval safely batches fsyncs
	// while preserving ordered append; close and compaction always force a sync.
	if syncInterval <= 0 || lastSync.IsZero() || time.Since(lastSync) >= syncInterval {
		if err := file.Sync(); err != nil {
			return err
		}
		j.mu.Lock()
		j.lastSync = time.Now()
		j.mu.Unlock()
	}
	return nil
}

func (j *eventJournal) shouldCompact(maxRecords, maxBytes int) bool {
	if j == nil {
		return false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return (maxRecords > 0 && j.records > maxRecords*2) || (maxBytes > 0 && j.bytes > int64(maxBytes)*2)
}

// close stops the writer after draining every queued operation.
func (j *eventJournal) close() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	j.stopped = true
	j.cond.Broadcast()
	started := j.writer
	j.mu.Unlock()
	if started {
		j.wg.Wait()
		return nil
	}
	// No writer was ever started (no appends); close directly.
	j.mu.Lock()
	file := j.file
	j.file = nil
	j.mu.Unlock()
	if file == nil {
		return nil
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return file.Close()
}

// rewrite atomically replaces the journal with the given records. Runs on the
// writer goroutine, so appends queued after the snapshot land in the new file
// in their original order once the swap completes.
func (j *eventJournal) rewrite(file *os.File, records []EventRecord) error {
	if file == nil {
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
	if err := file.Close(); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	if err := os.Rename(tempName, j.path); err != nil {
		return fmt.Errorf("replace event journal: %w", err)
	}
	newFile, err := os.OpenFile(j.path, os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		// The journal file is gone; disable further writes but keep running.
		j.mu.Lock()
		j.file = nil
		j.mu.Unlock()
		return err
	}
	j.mu.Lock()
	j.file = newFile
	j.records = len(records)
	j.bytes = 0
	j.lastSync = time.Now()
	j.mu.Unlock()
	if info, statErr := newFile.Stat(); statErr == nil {
		j.mu.Lock()
		j.bytes = info.Size()
		j.mu.Unlock()
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
