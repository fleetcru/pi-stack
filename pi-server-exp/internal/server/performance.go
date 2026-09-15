package server

import (
	"net/http"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"
)

const (
	defaultPerformanceInterval   = 15 * time.Second
	defaultPerformanceHistoryMax = 240
	minimumPerformanceInterval   = time.Second
	maximumPerformanceHistoryMax = 5760
)

// performanceSample is one observation of a monitored process. cpuPercent is
// normalized across logical CPUs to a 0..100 range and is null for the first
// sample of a process (no prior baseline) and whenever a new process identity
// is observed for the same series (PID reuse reset).
type performanceSample struct {
	At               string   `json:"at"`
	CPUPercent       *float64 `json:"cpuPercent"`
	RSSBytes         *int64   `json:"rssBytes"`
	HeapAllocBytes   *uint64  `json:"heapAllocBytes,omitempty"`
	Goroutines       *int     `json:"goroutines,omitempty"`
	RequestRate      *float64 `json:"requestRate,omitempty"`
	ErrorRate        *float64 `json:"errorRate,omitempty"`
	AverageLatencyMs *float64 `json:"averageLatencyMs,omitempty"`
	ActiveSessions   *int     `json:"activeSessions,omitempty"`
	ActiveRuns       *int     `json:"activeRuns,omitempty"`
	QueuedRuns       *int     `json:"queuedRuns,omitempty"`
}

type performanceSeries struct {
	ID      string              `json:"id"`
	Name    string              `json:"name"`
	Kind    string              `json:"kind"`
	PID     int                 `json:"pid"`
	Running bool                `json:"running"`
	Current *performanceSample  `json:"current"`
	Samples []performanceSample `json:"samples"`
}

type requestPerformance struct {
	Route     string  `json:"route"`
	Count     uint64  `json:"count"`
	Errors    uint64  `json:"errors"`
	AverageMs float64 `json:"averageMs"`
	MaxMs     float64 `json:"maxMs"`
}

type performanceSnapshot struct {
	SampleIntervalSeconds float64              `json:"sampleIntervalSeconds"`
	HistoryLimit          int                  `json:"historyLimit"`
	Server                performanceSeries    `json:"server"`
	Sessions              []performanceSeries  `json:"sessions"`
	Requests              []requestPerformance `json:"requests"`
}

// sessionPerfHistory retains per-session samples plus the CPU baseline used to
// normalize utilization. identity is the OS process creation identity, so a
// restart or PID reuse resets the baseline instead of producing a bogus spike.
type sessionPerfHistory struct {
	name     string
	pid      int
	identity uint64
	samples  []performanceSample
	lastCPU  float64
	haveCPU  bool
}

type processStatsReader func(pid int) (cpuSeconds float64, rssBytes int64, identity uint64, ok bool)

type performanceCollector struct {
	sampleMu   sync.Mutex
	mu         sync.Mutex
	interval   time.Duration
	historyMax int

	serverSamples  []performanceSample
	serverIdentity uint64
	lastServerCPU  float64
	haveServerCPU  bool

	lastReq      requestMetric
	haveLastReq  bool
	lastSampleAt time.Time

	sessionHistories map[string]*sessionPerfHistory

	sessions  *SessionRegistry
	admission *TaskAdmission
	metrics   *requestMetrics
	readStats processStatsReader
}

func newPerformanceCollector(interval time.Duration, historyMax int, sessions *SessionRegistry, admission *TaskAdmission, metrics *requestMetrics) *performanceCollector {
	if interval <= 0 {
		interval = defaultPerformanceInterval
	} else if interval < minimumPerformanceInterval {
		interval = minimumPerformanceInterval
	}
	if historyMax <= 0 {
		historyMax = defaultPerformanceHistoryMax
	} else if historyMax > maximumPerformanceHistoryMax {
		historyMax = maximumPerformanceHistoryMax
	}
	return &performanceCollector{
		interval:         interval,
		historyMax:       historyMax,
		sessionHistories: map[string]*sessionPerfHistory{},
		sessions:         sessions,
		admission:        admission,
		metrics:          metrics,
		readStats:        readProcessStats,
	}
}

// start launches the background sampler. It selects on stop, matching the
// worker heartbeat goroutine lifecycle, so Shutdown's existing
// close(s.stopHeartbeat) stops it.
func (c *performanceCollector) start(stop <-chan struct{}) {
	c.sample(time.Now())
	go func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.sample(time.Now())
			case <-stop:
				return
			}
		}
	}()
}

// sample records one round of observations. Registry lock is never held while
// PiProcess methods are called: session processes are copied under the
// registry read lock first, respecting SessionRegistry.mu -> PiProcess.mu.
func (c *performanceCollector) sample(now time.Time) {
	c.sampleMu.Lock()
	defer c.sampleMu.Unlock()

	at := now.UTC().Format(time.RFC3339Nano)
	elapsed := c.interval
	if !c.lastSampleAt.IsZero() {
		elapsed = now.Sub(c.lastSampleAt)
		if elapsed <= 0 {
			elapsed = c.interval
		}
	}
	c.lastSampleAt = now

	// Snapshot processes under the registry read lock, release, then touch
	// each PiProcess individually.
	type target struct {
		id      string
		name    string
		process *PiProcess
	}
	var targets []target
	var knownIDs map[string]bool
	if c.sessions != nil {
		c.sessions.mu.RLock()
		for id, p := range c.sessions.sessions {
			name := id
			if spec, exists := c.sessions.specs[id]; exists && spec.Title != "" {
				name = spec.Title
			}
			targets = append(targets, target{id: id, name: name, process: p})
			if knownIDs == nil {
				knownIDs = map[string]bool{}
			}
			knownIDs[id] = true
		}
		c.sessions.mu.RUnlock()
	}

	serverPID := os.Getpid()
	cpuSeconds, rss, identity, ok := c.readStats(serverPID)

	sample := performanceSample{At: at}
	if ok {
		rssBytes := rss
		sample.RSSBytes = &rssBytes
		if c.serverIdentity != identity {
			c.haveServerCPU = false
			c.serverIdentity = identity
		}
		if pct, valid := cpuPercentDelta(c.lastServerCPU, c.haveServerCPU, cpuSeconds, elapsed); valid {
			value := pct
			sample.CPUPercent = &value
		}
		c.lastServerCPU = cpuSeconds
		c.haveServerCPU = true
	} else {
		c.haveServerCPU = false
	}

	heap := runtime.MemStats{}
	runtime.ReadMemStats(&heap)
	heapBytes := heap.HeapAlloc
	sample.HeapAllocBytes = &heapBytes
	goroutines := runtime.NumGoroutine()
	sample.Goroutines = &goroutines

	if c.admission != nil {
		activeRuns := c.admission.Active()
		queuedRuns := c.admission.Queued()
		sample.ActiveRuns = &activeRuns
		sample.QueuedRuns = &queuedRuns
	}
	if c.sessions != nil {
		activeSessions := c.sessions.ActiveCount()
		sample.ActiveSessions = &activeSessions
	}

	if c.metrics != nil {
		counters := c.metrics.counters()
		count, errors, total := sumRequestMetrics(counters)
		if c.haveLastReq && count >= c.lastReq.Count && errors >= c.lastReq.ErrorCount && total >= c.lastReq.Total {
			dCount := float64(count - c.lastReq.Count)
			dErrors := float64(errors - c.lastReq.ErrorCount)
			dTotal := total - c.lastReq.Total
			requestRate := dCount / elapsed.Seconds()
			errorRate := dErrors / elapsed.Seconds()
			averageLatencyMs := 0.0
			if dCount > 0 {
				averageLatencyMs = (dTotal.Seconds() * 1000) / dCount
			}
			sample.RequestRate = &requestRate
			sample.ErrorRate = &errorRate
			sample.AverageLatencyMs = &averageLatencyMs
		}
		c.lastReq = requestMetric{Count: count, ErrorCount: errors, Total: total}
		c.haveLastReq = true
	}

	c.mu.Lock()
	c.serverSamples = appendRing(c.serverSamples, sample, c.historyMax)

	// Per-session samples and pruning of deleted sessions.
	for _, t := range targets {
		pid, running := t.process.ProcessTarget()
		history, exists := c.sessionHistories[t.id]
		if !exists {
			history = &sessionPerfHistory{}
			c.sessionHistories[t.id] = history
		}
		history.name = t.name
		history.pid = pid
		sample := performanceSample{At: at}
		if running && pid > 0 {
			if cpu, rss, identity, ok := c.readStats(pid); ok {
				if history.identity != identity {
					history.haveCPU = false
					history.identity = identity
				}
				rssBytes := rss
				sample.RSSBytes = &rssBytes
				if pct, valid := cpuPercentDelta(history.lastCPU, history.haveCPU, cpu, elapsed); valid {
					value := pct
					sample.CPUPercent = &value
				}
				history.lastCPU = cpu
				history.haveCPU = true
			} else {
				history.haveCPU = false
			}
		} else {
			// Exited or unknown process: reset the CPU baseline so a future
			// PID (possibly reused) starts a fresh measurement window.
			history.haveCPU = false
			history.identity = 0
		}
		history.samples = appendRing(history.samples, sample, c.historyMax)
	}
	for id := range c.sessionHistories {
		if !knownIDs[id] {
			delete(c.sessionHistories, id)
		}
	}
	c.mu.Unlock()
}

func cpuPercentDelta(lastCPU float64, haveCPU bool, cpuSeconds float64, elapsed time.Duration) (float64, bool) {
	if !haveCPU || elapsed <= 0 || cpuSeconds < lastCPU {
		return 0, false
	}
	percent := (cpuSeconds - lastCPU) / elapsed.Seconds() * 100 / float64(runtime.NumCPU())
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return percent, true
}

func appendRing(samples []performanceSample, sample performanceSample, limit int) []performanceSample {
	samples = append(samples, sample)
	if len(samples) > limit {
		samples = samples[len(samples)-limit:]
	}
	return samples
}

func sumRequestMetrics(routes map[string]requestMetric) (count uint64, errors uint64, total time.Duration) {
	for _, value := range routes {
		count += value.Count
		errors += value.ErrorCount
		total += value.Total
	}
	return count, errors, total
}

func (c *performanceCollector) snapshot() performanceSnapshot {
	intervalSeconds := c.interval.Seconds()
	snapshot := performanceSnapshot{
		SampleIntervalSeconds: intervalSeconds,
		HistoryLimit:          c.historyMax,
		Sessions:              []performanceSeries{},
		Requests:              []requestPerformance{},
	}
	if c.metrics != nil {
		snapshot.Requests = c.metrics.performance()
	}

	// Collect process state before taking the history lock. This keeps registry
	// and PiProcess locks independent from the collector lock.
	type sessionTarget struct {
		name    string
		pid     int
		running bool
	}
	targets := map[string]sessionTarget{}
	if c.sessions != nil {
		type registeredTarget struct {
			id      string
			name    string
			process *PiProcess
		}
		registered := []registeredTarget{}
		c.sessions.mu.RLock()
		for id, process := range c.sessions.sessions {
			name := id
			if spec, exists := c.sessions.specs[id]; exists && spec.Title != "" {
				name = spec.Title
			}
			registered = append(registered, registeredTarget{id: id, name: name, process: process})
		}
		c.sessions.mu.RUnlock()
		for _, target := range registered {
			pid, running := target.process.ProcessTarget()
			targets[target.id] = sessionTarget{name: target.name, pid: pid, running: running}
		}
	}

	c.mu.Lock()
	server := performanceSeries{ID: "server", Name: "pi-server", Kind: "server", PID: os.Getpid(), Running: true, Samples: []performanceSample{}}
	if len(c.serverSamples) > 0 {
		server.Samples = append(server.Samples, c.serverSamples...)
		current := server.Samples[len(server.Samples)-1]
		server.Current = &current
	}
	snapshot.Server = server
	for id, history := range c.sessionHistories {
		target := targets[id]
		name := history.name
		if target.name != "" {
			name = target.name
		}
		series := performanceSeries{
			ID:      id,
			Name:    name,
			Kind:    "session",
			PID:     target.pid,
			Running: target.running,
			Samples: []performanceSample{},
		}
		if len(history.samples) > 0 {
			series.Samples = append(series.Samples, history.samples...)
			current := series.Samples[len(series.Samples)-1]
			series.Current = &current
		}
		snapshot.Sessions = append(snapshot.Sessions, series)
	}
	c.mu.Unlock()

	sort.Slice(snapshot.Sessions, func(i, j int) bool {
		return snapshot.Sessions[i].ID < snapshot.Sessions[j].ID
	})
	return snapshot
}

func (s *Server) performance(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.perf.snapshot())
}
