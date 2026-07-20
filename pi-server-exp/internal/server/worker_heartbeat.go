package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

func (s *Server) startWorkerHeartbeats() {
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				workers := s.workers.List()
				var wg sync.WaitGroup
				sem := make(chan struct{}, 5) // max 5 concurrent health checks
				for _, worker := range workers {
					if worker.ID == "local" {
						continue
					}
					wg.Add(1)
					sem <- struct{}{}
					go func(w Worker) {
						defer wg.Done()
						defer func() { <-sem }()
						s.checkWorker(w)
					}(worker)
				}
				wg.Wait()
			case <-s.stopHeartbeat:
				return
			}
		}
	}()
}
func (s *Server) checkWorker(worker Worker) {
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(worker.URL, "/")+"/healthz", nil)
	if err != nil {
		s.workers.Heartbeat(worker.ID, false)
		return
	}
	if string(worker.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+string(worker.Token))
	}
	resp, err := s.httpClient.Do(req)
	if err != nil || resp == nil {
		s.workers.Heartbeat(worker.ID, false)
		return
	}
	defer resp.Body.Close()
	healthy := resp.StatusCode/100 == 2
	s.workers.Heartbeat(worker.ID, healthy)
	if !healthy {
		return
	}
	var body struct {
		Capacity struct {
			ActiveSessions int `json:"activeSessions"`
			MaxSessions    int `json:"maxSessions"`
		} `json:"capacity"`
	}
	if json.NewDecoder(resp.Body).Decode(&body) == nil {
		s.workers.UpdateCapacity(worker.ID, body.Capacity.ActiveSessions, body.Capacity.MaxSessions)
	}
}
