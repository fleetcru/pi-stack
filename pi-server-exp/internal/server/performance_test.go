package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"
)

func newPerfTestServer(t *testing.T) *Server {
	t.Helper()
	dataDir, cwd := t.TempDir(), t.TempDir()
	t.Setenv("PI_SERVER_DATA_DIR", dataDir)
	t.Setenv("PI_SERVER_CWD", cwd)
	t.Setenv("PI_SERVER_AUTH_TOKEN", "performance-test-token")
	cfg := ConfigFromEnv()
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { close(s.stopHeartbeat) })
	return s
}

func TestPerformanceFirstSampleHasNullCPU(t *testing.T) {
	collector := newPerformanceCollector(time.Second, 10, nil, nil, nil)
	collector.sample(time.Now())
	snapshot := collector.snapshot()
	if len(snapshot.Server.Samples) != 1 {
		t.Fatalf("expected 1 server sample, got %d", len(snapshot.Server.Samples))
	}
	if snapshot.Server.Samples[0].CPUPercent != nil {
		t.Fatalf("first sample cpuPercent should be null, got %v", *snapshot.Server.Samples[0].CPUPercent)
	}
	if snapshot.Server.Current == nil {
		t.Fatal("current should reference the latest sample")
	}
	if snapshot.SampleIntervalSeconds <= 0 || snapshot.HistoryLimit <= 0 {
		t.Fatalf("unexpected snapshot config: %+v", snapshot)
	}
	if snapshot.Sessions == nil || snapshot.Requests == nil {
		t.Fatalf("sessions and requests must be arrays, not null")
	}
}

func TestPerformanceSecondSampleComputesCPU(t *testing.T) {
	_, rss, identity, ok := readProcessStats(os.Getpid())
	if !ok {
		t.Skip("process stats unavailable on this platform")
	}
	if rss <= 0 || identity == 0 {
		t.Fatalf("process stats missing RSS or identity: rss=%d identity=%d", rss, identity)
	}
	c := newPerformanceCollector(time.Second, 10, nil, nil, nil)
	c.sample(time.Now())
	time.Sleep(30 * time.Millisecond)
	c.sample(time.Now().Add(time.Second))
	snapshot := c.snapshot()
	if len(snapshot.Server.Samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(snapshot.Server.Samples))
	}
	second := snapshot.Server.Samples[1]
	if second.CPUPercent == nil {
		t.Fatalf("second sample should have a cpuPercent after a valid baseline")
	}
	if *second.CPUPercent < 0 || *second.CPUPercent > 100 {
		t.Fatalf("cpuPercent %v outside normalized 0..100 range", *second.CPUPercent)
	}
}

func TestPerformanceHistoryRingIsBounded(t *testing.T) {
	c := newPerformanceCollector(time.Second, 3, nil, nil, nil)
	now := time.Now()
	for i := 0; i < 7; i++ {
		c.sample(now.Add(time.Duration(i) * time.Second))
	}
	snapshot := c.snapshot()
	if len(snapshot.Server.Samples) != 3 {
		t.Fatalf("history limit not enforced: %d samples", len(snapshot.Server.Samples))
	}
	last := len(snapshot.Server.Samples) - 1
	if snapshot.Server.Samples[last].At != snapshot.Server.Current.At {
		t.Fatalf("current should reference the newest retained sample")
	}
}

func TestPerformanceCPUBaselineResetsOnProcessIdentityChange(t *testing.T) {
	process := &PiProcess{
		id:      "s1",
		cmd:     &exec.Cmd{Process: &os.Process{Pid: 4242}},
		running: true,
	}
	registry := NewSessionRegistry("", 0)
	registry.sessions["s1"] = process
	registry.specs["s1"] = SessionSpec{ID: "s1", Title: "Named session"}
	collector := newPerformanceCollector(time.Second, 10, registry, nil, nil)

	identity := uint64(100)
	cpu := 10.0
	collector.readStats = func(pid int) (float64, int64, uint64, bool) {
		if pid == 4242 {
			return cpu, 4096, identity, true
		}
		return 1, 8192, 1, true
	}

	now := time.Now()
	collector.sample(now)
	cpu = 11
	collector.sample(now.Add(time.Second))
	snapshot := collector.snapshot()
	if snapshot.Sessions[0].Current.CPUPercent == nil {
		t.Fatal("same process identity should produce a CPU delta")
	}
	if snapshot.Sessions[0].Name != "Named session" {
		t.Fatalf("session name = %q", snapshot.Sessions[0].Name)
	}

	identity = 200
	cpu = 100
	collector.sample(now.Add(2 * time.Second))
	snapshot = collector.snapshot()
	if snapshot.Sessions[0].Current.CPUPercent != nil {
		t.Fatalf("new process identity must reset CPU baseline, got %v", *snapshot.Sessions[0].Current.CPUPercent)
	}
}

func TestPerformancePrunesDeletedSessions(t *testing.T) {
	collector := newPerformanceCollector(time.Second, 10, nil, nil, nil)
	collector.mu.Lock()
	collector.sessionHistories["deleted"] = &sessionPerfHistory{name: "deleted"}
	collector.mu.Unlock()
	collector.sample(time.Now())
	collector.mu.Lock()
	_, exists := collector.sessionHistories["deleted"]
	collector.mu.Unlock()
	if exists {
		t.Fatal("history for deleted session should be pruned")
	}
}

func TestPerformanceRequestRates(t *testing.T) {
	s := newPerfTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	s.httpSrv.Handler.ServeHTTP(httptest.NewRecorder(), req)

	now := time.Now()
	s.perf.sample(now)
	s.httpSrv.Handler.ServeHTTP(httptest.NewRecorder(), req)
	s.httpSrv.Handler.ServeHTTP(httptest.NewRecorder(), req)
	// Unauthenticated requests are rejected with 401 by the middleware chain;
	// all three count as errors via metricResponseWriter.
	s.perf.sample(now.Add(2 * time.Second))
	snapshot := s.perf.snapshot()
	latest := snapshot.Server.Samples[len(snapshot.Server.Samples)-1]
	if latest.RequestRate == nil || *latest.RequestRate != 1.0 {
		t.Fatalf("requestRate = %v, want 1.0", latest.RequestRate)
	}
	if latest.ErrorRate == nil || *latest.ErrorRate != 1.0 {
		t.Fatalf("errorRate = %v, want 1.0", latest.ErrorRate)
	}
	if len(snapshot.Requests) != 1 || snapshot.Requests[0].Count != 3 {
		t.Fatalf("requests = %+v", snapshot.Requests)
	}
	if snapshot.Requests[0].Errors != 3 {
		t.Fatalf("errors = %d, want 3", snapshot.Requests[0].Errors)
	}
	if snapshot.Requests[0].Route != "GET /healthz" {
		t.Fatalf("route label = %q, want GET /healthz", snapshot.Requests[0].Route)
	}
	if snapshot.Requests[0].AverageMs < 0 || snapshot.Requests[0].MaxMs < 0 {
		t.Fatalf("latency fields must be numeric ms, got %+v", snapshot.Requests[0])
	}
}

func TestPerformanceEndpointJSON(t *testing.T) {
	s := newPerfTestServer(t)
	rec := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/performance", nil)
	request.Header.Set("Authorization", "Bearer performance-test-token")
	s.httpSrv.Handler.ServeHTTP(rec, request)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var snapshot performanceSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
	if snapshot.Server.Kind != "server" || snapshot.Server.ID != "server" {
		t.Fatalf("server series = %+v", snapshot.Server)
	}
	if snapshot.Sessions == nil || snapshot.Requests == nil {
		t.Fatalf("sessions/requests must serialize as arrays")
	}
}

func TestPerformanceRequestCounterResetSkipsRates(t *testing.T) {
	metrics := newRequestMetrics()
	metrics.routes["GET /healthz"] = requestMetric{Count: 1, ErrorCount: 1, Total: time.Millisecond}
	collector := newPerformanceCollector(time.Second, 10, nil, nil, metrics)
	collector.haveLastReq = true
	collector.lastReq = requestMetric{Count: 10, ErrorCount: 5, Total: time.Second}
	collector.sample(time.Now())
	current := collector.snapshot().Server.Current
	if current == nil {
		t.Fatal("current sample is nil")
	}
	if current.RequestRate != nil || current.ErrorRate != nil || current.AverageLatencyMs != nil {
		t.Fatalf("rates should be omitted after counter reset: %+v", current)
	}
}

func TestPerformanceCollectorClampsUnsafeRetention(t *testing.T) {
	collector := newPerformanceCollector(time.Millisecond, maximumPerformanceHistoryMax+1, nil, nil, nil)
	if collector.interval != minimumPerformanceInterval {
		t.Fatalf("interval = %v, want %v", collector.interval, minimumPerformanceInterval)
	}
	if collector.historyMax != maximumPerformanceHistoryMax {
		t.Fatalf("historyMax = %d, want %d", collector.historyMax, maximumPerformanceHistoryMax)
	}
}

func TestPerformanceConfigFromEnv(t *testing.T) {
	t.Setenv("PI_SERVER_PERFORMANCE_INTERVAL", "5s")
	t.Setenv("PI_SERVER_PERFORMANCE_HISTORY_MAX", "12")
	cfg := ConfigFromEnv()
	if cfg.PerformanceInterval != 5*time.Second {
		t.Fatalf("PerformanceInterval = %v", cfg.PerformanceInterval)
	}
	if cfg.PerformanceHistoryMax != 12 {
		t.Fatalf("PerformanceHistoryMax = %d", cfg.PerformanceHistoryMax)
	}
}

func TestPerformanceOpenAPIDocument(t *testing.T) {
	s := newPerfTestServer(t)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	var spec struct {
		Paths      map[string]map[string]any `json:"paths"`
		Components struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := spec.Paths["/v1/performance"]; !ok {
		t.Fatalf("/v1/performance missing from OpenAPI paths")
	}
	for _, schema := range []string{"PerformanceSample", "PerformanceSeries", "RequestPerformance", "PerformanceSnapshot"} {
		if _, ok := spec.Components.Schemas[schema]; !ok {
			t.Fatalf("OpenAPI schema %s missing", schema)
		}
	}
}
