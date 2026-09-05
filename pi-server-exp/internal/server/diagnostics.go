package server

import (
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request) {
	var journalBytes int64
	var journalFiles int
	var retainedEvents int
	var retainedEventBytes int
	var droppedEvents uint64
	for _, id := range s.sessions.List() {
		if process, ok := s.sessions.Get(id); ok {
			retained, bytes, dropped := process.EventMetrics()
			retainedEvents += retained
			retainedEventBytes += bytes
			droppedEvents += dropped
		}
	}
	entries, err := os.ReadDir(filepath.Join(s.cfg.DataDir, "events"))
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
				continue
			}
			if info, statErr := entry.Info(); statErr == nil {
				journalFiles++
				journalBytes += info.Size()
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"apiVersion": s.apiVersion(),
		"startedAt":   s.startedAt,
		"uptime":      time.Since(s.startedAt).String(),
		"dataDir":    s.cfg.DataDir,
		"sessions": map[string]any{
			"count":  len(s.sessions.List()),
			"active": s.sessions.ActiveCount(),
		},
		"workers": map[string]any{
			"count":  len(s.workers.List()),
			"healthy": countHealthyWorkers(s.workers.List()),
		},
		"devices": map[string]any{
			"count": len(s.devices.list()),
		},
		"eventJournal": map[string]any{
			"files": journalFiles,
			"bytes": journalBytes,
		},
		"eventReplay": map[string]any{
			"retained":      retainedEvents,
			"retainedBytes": retainedEventBytes,
			"dropped":       droppedEvents,
		},
	})
}

func (s *Server) apiVersion() string {
	return APIVersion
}

func countHealthyWorkers(workers []Worker) int {
	count := 0
	for _, worker := range workers {
		if worker.Status == "healthy" || worker.ID == "local" {
			count++
		}
	}
	return count
}
