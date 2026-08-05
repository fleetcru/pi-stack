package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"
)

func TestPiProcessRequestWaiterLifecycle(t *testing.T) {
	tests := []struct {
		name        string
		respond     bool
		wantErr     error
		wantSuccess bool
	}{
		{
			name:        "correlates response with request",
			respond:     true,
			wantSuccess: true,
		},
		{
			name:    "removes waiter after context cancellation",
			wantErr: context.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, reader := runningTestProcess(t)
			defer reader.Close()

			ctx := context.Background()
			var cancel context.CancelFunc = func() {}
			if tt.wantErr != nil {
				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
			}
			defer cancel()

			result := make(chan struct {
				event RPCEvent
				err   error
			}, 1)
			go func() {
				event, err := p.Request(ctx, RPCCommand{"type": "get_state"})
				result <- struct {
					event RPCEvent
					err   error
				}{event, err}
			}()

			command := readRPCCommand(t, reader)
			id, _ := command["id"].(string)
			if id == "" {
				t.Fatalf("request did not include an ID: %#v", command)
			}
			if tt.respond {
				p.dispatch(RPCEvent{"type": "response", "id": id, "success": true, "data": map[string]any{"ok": true}})
			}

			got := <-result
			if !errors.Is(got.err, tt.wantErr) {
				t.Fatalf("Request error = %v, want %v", got.err, tt.wantErr)
			}
			if tt.wantSuccess {
				if success, _ := got.event["success"].(bool); !success {
					t.Fatalf("response = %#v, want successful response", got.event)
				}
			}

			p.mu.RLock()
			waiterCount := len(p.waiters)
			p.mu.RUnlock()
			if waiterCount != 0 {
				t.Fatalf("waiters = %d, want 0", waiterCount)
			}

			// A late response after cancellation must be harmless.
			if !tt.respond {
				p.dispatch(RPCEvent{"type": "response", "id": id, "success": true})
			}
		})
	}
}

func runningTestProcess(t *testing.T) (*PiProcess, io.ReadCloser) {
	t.Helper()
	cfg := Config{EventHistoryMax: 20, EventHistoryBytes: 1024}
	p := NewPiProcess(SessionSpec{ID: "test-session", CWD: t.TempDir()}, cfg, testLogger())
	reader, writer := io.Pipe()
	p.mu.Lock()
	p.running = true
	p.stdin = writer
	p.done = make(chan struct{})
	p.mu.Unlock()
	return p, reader
}

func readRPCCommand(t *testing.T, reader io.Reader) RPCCommand {
	t.Helper()
	line, err := bufio.NewReader(reader).ReadBytes('\n')
	if err != nil {
		t.Fatalf("read RPC command: %v", err)
	}
	var command RPCCommand
	if err := json.Unmarshal(line, &command); err != nil {
		t.Fatalf("decode RPC command: %v", err)
	}
	return command
}
