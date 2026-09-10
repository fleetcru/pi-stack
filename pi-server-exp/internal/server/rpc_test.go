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
	cfg := Config{EventHistoryMax: 20, EventHistoryBytes: 1024, RequestTimeout: 5 * time.Second}
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

// Regression: activePromptID must be assigned under the same lock that sets
// turnActive. A very fast failed prompt response dispatched between the
// stdin write and a late assignment would otherwise match neither the waiter
// table nor activePromptID, leaving turnActive stuck forever.
func TestSendPromptRejectedImmediatelyReleasesTurn(t *testing.T) {
	for attempt := 0; attempt < 200; attempt++ {
		p, reader := runningTestProcess(t)

		// Reader goroutine: echo a failed response for every prompt command
		// as fast as possible, racing Send's post-unlock bookkeeping.
		readDone := make(chan struct{})
		go func() {
			defer close(readDone)
			defer reader.Close()
			for {
				command, err := readRPCCommandOrEOF(reader)
				if err != nil || command == nil {
					return
				}
				if id, ok := command["id"].(string); ok && command["type"] == "prompt" {
					p.dispatch(RPCEvent{"type": "response", "id": id, "success": false, "error": "rejected"})
				}
			}
		}()

		if err := p.Send(RPCCommand{"type": "prompt", "id": "fast-" + time.Now().Format("150405.000000000"), "message": "hi"}); err != nil {
			t.Fatalf("attempt %d: Send failed: %v", attempt, err)
		}
		// End the synthetic command stream after the one command. Without this,
		// the echo goroutine waits forever for a second line.
		p.mu.Lock()
		_ = p.stdin.Close()
		p.stdin = nil
		p.mu.Unlock()

		// Give a wrong late assignment no room: by the time Send returns, the
		// rejected response may already have been dispatched.
		deadline := time.Now().Add(2 * time.Second)
		for {
			p.mu.RLock()
			stuck := p.turnActive
			p.mu.RUnlock()
			if !stuck {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("attempt %d: turnActive stuck after rejected prompt", attempt)
			}
			time.Sleep(time.Millisecond)
		}
		<-readDone
	}
}

func readRPCCommandOrEOF(r io.Reader) (RPCCommand, error) {
	line, err := bufio.NewReader(r).ReadBytes('\n')
	if len(line) == 0 {
		return nil, err
	}
	var command RPCCommand
	if err := json.Unmarshal(line, &command); err != nil {
		return nil, err
	}
	return command, nil
}
