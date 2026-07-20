package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

// SessionSummary is the unified inventory shape for local and remote sessions.
type SessionSummary struct {
	ID              string         `json:"id"`
	WorkerID        string         `json:"workerId"`
	WorkerSessionID string         `json:"workerSessionId,omitempty"`
	CWD             string         `json:"cwd,omitempty"`
	Status          string         `json:"status,omitempty"`
	Managed         bool           `json:"managed"`
	Transport       string         `json:"transport,omitempty"`
	Project         string         `json:"project,omitempty"`
	Title           string         `json:"title,omitempty"`
	TaskType        string         `json:"taskType,omitempty"`
	Labels          []string       `json:"labels,omitempty"`
	CreatedAt       time.Time      `json:"createdAt,omitempty"`
	UpdatedAt       time.Time      `json:"updatedAt,omitempty"`
	State           map[string]any `json:"state,omitempty"`
}

type partialFailure struct {
	WorkerID  string `json:"workerId,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	Error     string `json:"error"`
	Code      string `json:"code,omitempty"`
}

func localSummaryFromSpec(spec SessionSpec) SessionSummary {
	return SessionSummary{
		ID:        spec.ID,
		WorkerID:  "local",
		CWD:       spec.CWD,
		Status:    spec.Status,
		Managed:   spec.Managed,
		Transport: spec.Transport,
		Project:   spec.Project,
		Title:     spec.Title,
		TaskType:  spec.TaskType,
		Labels:    append([]string(nil), spec.Labels...),
		CreatedAt: spec.CreatedAt,
		UpdatedAt: spec.UpdatedAt,
	}
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "local"
	}
	includeState := r.URL.Query().Get("include") == "state"

	if scope == "local" {
		specs := s.sessions.ListSpecs()
		// Preserve today's list semantics: raw local specs.
		writeJSON(w, http.StatusOK, map[string]any{"sessions": specs})
		return
	}

	if scope != "all" {
		writeErrorCode(w, r, http.StatusBadRequest, CodeBadRequest, "scope must be local or all")
		return
	}

	summaries := make([]SessionSummary, 0)
	failures := make([]partialFailure, 0)

	// Collect specs and identify running local sessions for parallel state fetch.
	specs := s.sessions.ListSpecs()
	type stateResult struct {
		id    string
		data  map[string]any
		runtime map[string]any
		running bool
		status  string
	}
	runningSpecs := make([]SessionSpec, 0)
	for _, spec := range specs {
		if includeState && spec.Transport != "relay" {
			if p, ok := s.sessions.Get(spec.ID); ok {
				if st := p.Status(); st["running"] == true {
					runningSpecs = append(runningSpecs, spec)
				}
			}
		}
	}
	// Fetch state for all running sessions in parallel.
	stateResults := make(map[string]stateResult)
	if len(runningSpecs) > 0 {
		var wg sync.WaitGroup
		ch := make(chan stateResult, len(runningSpecs))
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		for _, spec := range runningSpecs {
			wg.Add(1)
			go func(spec SessionSpec) {
				defer wg.Done()
				p, _ := s.sessions.Get(spec.ID)
				ps := p.Status()
				res := stateResult{id: spec.ID, runtime: map[string]any{}, running: false}
				if rt, ok := ps["runtimeStatus"].(map[string]any); ok {
					res.runtime["runtimeStatus"] = rt
					if rs, _ := rt["state"].(string); rs != "" {
						res.status = rs
					}
				}
				if st, ok := ps["running"].(bool); ok {
					res.running = st
				}
				if status, ok := ps["status"].(string); ok && res.status == "" {
					res.status = status
				}
				data, err := s.cachedPiState(ctx, spec.ID, p)
				if err == nil && data != nil {
					res.data = data
				}
				ch <- res
			}(spec)
		}
		wg.Wait()
		close(ch)
		for res := range ch {
			stateResults[res.id] = res
		}
	}
	// Build summaries, merging parallel state results.
	for _, spec := range specs {
		sum := localSummaryFromSpec(spec)
		if includeState {
			if spec.Transport == "relay" {
				snap := s.external.stateSnapshot(spec.ID)
				if snap == nil {
					// Relay spec from a previous run with no live bridge — skip it.
					continue
				}
				if status, _ := snap["status"].(string); status != "" {
					sum.Status = status
				}
				sum.State = snap
			} else if res, ok := stateResults[spec.ID]; ok {
				sum.State = res.runtime
				sum.State["running"] = res.running
				if res.status != "" {
					sum.Status = res.status
				}
				if res.data != nil {
					if model, ok := res.data["model"]; ok {
						sum.State["model"] = model
					}
					if thinking, ok := res.data["thinkingLevel"]; ok {
						sum.State["thinkingLevel"] = thinking
					}
					if streaming, ok := res.data["isStreaming"]; ok {
						sum.State["isStreaming"] = streaming
					}
					if title, _ := res.data["sessionName"].(string); title != "" && title != spec.Title {
						sum.Title = title
						// Debounce: only fire one update goroutine per session at a time.
						s.pendingTitleMu.Lock()
						if !s.pendingTitle[spec.ID] {
							s.pendingTitle[spec.ID] = true
							s.pendingTitleMu.Unlock()
							go func(id, title string) {
								defer func() {
									s.pendingTitleMu.Lock()
									delete(s.pendingTitle, id)
									s.pendingTitleMu.Unlock()
								}()
								_, _ = s.sessions.UpdateMetadata(id, SessionMetadataUpdate{Title: &title})
							}(spec.ID, title)
						} else {
							s.pendingTitleMu.Unlock()
						}
					}
				}
			} else {
				sum.State = map[string]any{"running": false}
			}
		}
		summaries = append(summaries, sum)
	}

	// Remote mappings are grouped by worker so a worker inventory is fetched
	// once per request instead of once per mapped session.
	remotes := s.remoteSessions.List()
	type remoteResult struct {
		summary SessionSummary
		fail    *partialFailure
	}
	results := make([]remoteResult, len(remotes))
	byWorker := make(map[string][]int)
	for i, rec := range remotes {
		byWorker[rec.WorkerID] = append(byWorker[rec.WorkerID], i)
	}
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	for workerID, indexes := range byWorker {
		worker, ok := s.workers.Get(workerID)
		if !ok {
			for _, i := range indexes {
				rec := remotes[i]
				results[i] = remoteResult{summary: remoteSummaryStub(rec), fail: &partialFailure{WorkerID: rec.WorkerID, SessionID: rec.ID, Error: "mapped worker no longer exists", Code: CodeWorkerNotFound}}
			}
			continue
		}
		wg.Add(1)
		go func(worker Worker, indexes []int) {
			defer wg.Done()
			details, err := s.fetchRemoteWorkerSessionSummaries(ctx, worker)
			for _, i := range indexes {
				rec := remotes[i]
				sum := remoteSummaryStub(rec)
				if err != nil {
					results[i] = remoteResult{summary: sum, fail: &partialFailure{WorkerID: rec.WorkerID, SessionID: rec.ID, Error: "failed to load remote session summary", Code: CodeBadGateway}}
					continue
				}
				if detail, ok := details[rec.WorkerSessionID]; ok {
					sum = mergeRemoteSummary(sum, detail, includeState)
				} else {
					sum.Status = "missing"
				}
				results[i] = remoteResult{summary: sum}
			}
		}(worker, indexes)
	}
	wg.Wait()
	for _, res := range results {
		summaries = append(summaries, res.summary)
		if res.fail != nil {
			failures = append(failures, *res.fail)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sessions":        summaries,
		"partialFailures": failures,
	})
}

func remoteSummaryStub(rec RemoteSession) SessionSummary {
	return SessionSummary{ID: rec.ID, WorkerID: rec.WorkerID, WorkerSessionID: rec.WorkerSessionID, CreatedAt: rec.CreatedAt, Status: "mapped"}
}

func mergeRemoteSummary(sum, detail SessionSummary, includeState bool) SessionSummary {
	if detail.CWD != "" {
		sum.CWD = detail.CWD
	}
	if detail.Status != "" {
		sum.Status = detail.Status
	}
	if detail.Project != "" {
		sum.Project = detail.Project
	}
	if detail.Title != "" {
		sum.Title = detail.Title
	}
	if detail.TaskType != "" {
		sum.TaskType = detail.TaskType
	}
	if len(detail.Labels) > 0 {
		sum.Labels = detail.Labels
	}
	if !detail.UpdatedAt.IsZero() {
		sum.UpdatedAt = detail.UpdatedAt
	}
	if includeState && detail.State != nil {
		sum.State = detail.State
	}
	return sum
}

func (s *Server) fetchRemoteWorkerSessionSummaries(ctx context.Context, worker Worker) (map[string]SessionSummary, error) {
	// Workers expose GET /v1/sessions as local specs. Fetch it once and let all
	// mappings for this worker share the result.
	base := stringsTrimRightSlash(worker.URL) + "/v1/sessions"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base, nil)
	if err != nil {
		return nil, err
	}
	if string(worker.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+string(worker.Token))
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errStatus(resp.StatusCode)
	}
	var body struct {
		Sessions []SessionSpec `json:"sessions"`
	}
	// Cap response body to prevent OOM from a compromised or malicious worker.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return nil, err
	}
	out := make(map[string]SessionSummary, len(body.Sessions))
	for _, spec := range body.Sessions {
		out[spec.ID] = localSummaryFromSpec(spec)
	}
	return out, nil
}

func (s *Server) fetchRemoteSessionSummary(ctx context.Context, worker Worker, workerSessionID string) (SessionSummary, error) {
	summaries, err := s.fetchRemoteWorkerSessionSummaries(ctx, worker)
	if err != nil {
		return SessionSummary{}, err
	}
	if summary, ok := summaries[workerSessionID]; ok {
		return summary, nil
	}
	return SessionSummary{ID: workerSessionID, Status: "missing"}, nil
}

func (s *Server) sessionSummary(w http.ResponseWriter, r *http.Request, id string) {
	if spec, ok := s.sessions.GetSpec(id); ok {
		// Relay specs with no live bridge are stale — treat as not found.
		if spec.Transport == "relay" && s.external.stateSnapshot(id) == nil {
			writeErrorCode(w, r, http.StatusNotFound, CodeSessionNotFound, "session not found")
			return
		}
		writeJSON(w, http.StatusOK, localSummaryFromSpec(spec))
		return
	}
	if rec, ok := s.remoteSessions.Get(id); ok {
		sum := SessionSummary{
			ID:              rec.ID,
			WorkerID:        rec.WorkerID,
			WorkerSessionID: rec.WorkerSessionID,
			CreatedAt:       rec.CreatedAt,
			Status:          "mapped",
		}
		if worker, ok := s.workers.Get(rec.WorkerID); ok {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			if detail, err := s.fetchRemoteSessionSummary(ctx, worker, rec.WorkerSessionID); err == nil {
				if detail.CWD != "" {
					sum.CWD = detail.CWD
				}
				if detail.Status != "" {
					sum.Status = detail.Status
				}
				sum.Project = detail.Project
				sum.Title = detail.Title
				sum.TaskType = detail.TaskType
				sum.Labels = detail.Labels
				sum.UpdatedAt = detail.UpdatedAt
			}
		}
		writeJSON(w, http.StatusOK, sum)
		return
	}
	writeErrorCode(w, r, http.StatusNotFound, CodeSessionNotFound, "session not found")
}

type statusError int

func (e statusError) Error() string { return http.StatusText(int(e)) }
func errStatus(code int) error      { return statusError(code) }

func stringsTrimRightSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
