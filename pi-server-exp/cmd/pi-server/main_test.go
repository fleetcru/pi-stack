package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	clog "github.com/charmbracelet/log"
)

func TestParseLogFormat(t *testing.T) {
	tests := []struct {
		input string
		want  clog.Formatter
		err   bool
	}{
		{"text", clog.TextFormatter, false},
		{"Text", clog.TextFormatter, false},
		{"json", clog.JSONFormatter, false},
		{"JSON", clog.JSONFormatter, false},
		{"logfmt", clog.LogfmtFormatter, false},
		{"Logfmt", clog.LogfmtFormatter, false},
		{"", clog.TextFormatter, false}, // default
		{"xml", 0, true},
		{"csv", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseLogFormat(tc.input)
			if (err != nil) != tc.err {
				t.Fatalf("parseLogFormat(%q) error = %v, wantErr %v", tc.input, err, tc.err)
			}
			if got != tc.want {
				t.Fatalf("parseLogFormat(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  clog.Level
		err   bool
	}{
		{"debug", clog.DebugLevel, false},
		{"Debug", clog.DebugLevel, false},
		{"info", clog.InfoLevel, false},
		{"Info", clog.InfoLevel, false},
		{"", clog.InfoLevel, false}, // default
		{"warn", clog.WarnLevel, false},
		{"warning", clog.WarnLevel, false},
		{"error", clog.ErrorLevel, false},
		{"Error", clog.ErrorLevel, false},
		{"trace", 0, true},
		{"fatal", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseLogLevel(tc.input)
			if (err != nil) != tc.err {
				t.Fatalf("parseLogLevel(%q) error = %v, wantErr %v", tc.input, err, tc.err)
			}
			if got != tc.want {
				t.Fatalf("parseLogLevel(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestFilteredArgs(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "no bg flag",
			input: []string{"--addr", ":3141", "--log-file", "out.log"},
			want:  []string{"--addr", ":3141", "--log-file", "out.log"},
		},
		{
			name:  "bg flag alone",
			input: []string{"--bg", "--addr", ":3141"},
			want:  []string{"--addr", ":3141"},
		},
		{
			name:  "bg flag at end",
			input: []string{"--addr", ":3141", "--bg"},
			want:  []string{"--addr", ":3141"},
		},
		{
			name:  "only bg flag",
			input: []string{"--bg"},
			want:  []string{},
		},
		{
			name:  "empty args",
			input: []string{},
			want:  []string{},
		},
		{
			name:  "bg=value form",
			input: []string{"--bg=true", "--addr", ":3141"},
			want:  []string{"--addr", ":3141"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filteredArgs(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("filteredArgs(%v) = %v (len %d), want %v (len %d)", tc.input, got, len(got), tc.want, len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("filteredArgs(%v)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestNewLoggerTextFormat(t *testing.T) {
	var buf bytes.Buffer
	l := newLogger(&buf, clog.TextFormatter, clog.InfoLevel, false)
	l.Info("hello", "key", "value")
	out := buf.String()
	if !strings.Contains(out, "hello") {
		t.Fatalf("expected 'hello' in output, got %q", out)
	}
	if !strings.Contains(out, "key=value") {
		t.Fatalf("expected 'key=value' in output, got %q", out)
	}
}

func TestNewLoggerJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	l := newLogger(&buf, clog.JSONFormatter, clog.InfoLevel, false)
	l.Info("structured", "count", 42)
	out := buf.String()
	if !strings.Contains(out, `"msg"`) {
		t.Fatalf("expected msg key in JSON output, got %q", out)
	}
	if !strings.Contains(out, `"count"`) {
		t.Fatalf("expected count key in JSON output, got %q", out)
	}
}

func TestNewLoggerLogfmtFormat(t *testing.T) {
	var buf bytes.Buffer
	l := newLogger(&buf, clog.LogfmtFormatter, clog.DebugLevel, false)
	l.Debug("debugging", "x", 1)
	out := buf.String()
	if !strings.Contains(out, "debugging") {
		t.Fatalf("expected 'debugging' in output, got %q", out)
	}
}

func TestNewLoggerLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := newLogger(&buf, clog.TextFormatter, clog.WarnLevel, false)
	l.Debug("should not appear")
	l.Info("should not appear")
	l.Warn("should appear")
	l.Error("should appear")
	out := buf.String()
	if strings.Contains(out, "should not appear") {
		t.Fatalf("debug/info messages should be filtered, got %q", out)
	}
	if !strings.Contains(out, "should appear") {
		t.Fatalf("warn/error messages should pass, got %q", out)
	}
}

func TestNewLoggerTTYFalseDisablesColor(t *testing.T) {
	var buf bytes.Buffer
	l := newLogger(&buf, clog.TextFormatter, clog.InfoLevel, false)
	// When tty is false, output should have no ANSI escape codes.
	l.Info("plain")
	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("expected no ANSI escapes when tty=false, got %q", out)
	}
	// Verify output is non-empty and useful.
	if len(out) == 0 {
		t.Fatal("expected non-empty output")
	}
}

func TestLoopbackAddr(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:3141", true},
		{"localhost:3141", true},
		{"[::1]:3141", true},
		{"0.0.0.0:3141", false},
		{"192.168.1.1:3141", false},
		{"10.0.0.1:3141", false},
		{"", false},
		{":3141", false},
	}
	for _, tc := range tests {
		t.Run(tc.addr, func(t *testing.T) {
			got := loopbackAddr(tc.addr)
			if got != tc.want {
				t.Fatalf("loopbackAddr(%q) = %v, want %v", tc.addr, got, tc.want)
			}
		})
	}
}

func TestStderrLogPath(t *testing.T) {
	path := stderrLogPath()
	if !strings.HasSuffix(path, "stderr.log") {
		t.Fatalf("expected path ending with stderr.log, got %q", path)
	}
	// Should not be empty.
	if path == "" {
		t.Fatal("expected non-empty stderr log path")
	}
}

func TestParseLogFormatJSONOutput(t *testing.T) {
	// Verify that JSON formatter actually produces JSON-like output.
	var buf bytes.Buffer
	l := newLogger(&buf, clog.JSONFormatter, clog.InfoLevel, false)
	l.Info("test-message", "foo", "bar")
	out := buf.String()
	if !strings.Contains(out, "{") || !strings.Contains(out, "}") {
		t.Fatalf("expected JSON output with braces, got %q", out)
	}
	// Confirm the message text is present.
	if !strings.Contains(out, "test-message") {
		t.Fatalf("expected 'test-message' in JSON output, got %q", out)
	}
}

func TestNewLoggerTimestampPresent(t *testing.T) {
	var buf bytes.Buffer
	l := newLogger(&buf, clog.TextFormatter, clog.InfoLevel, false)
	l.Info("with-time")
	out := buf.String()
	// The charmbracelet text format includes a time prefix like "01/02/2006 15:04:05"
	// We just check the line contains a date-like pattern (slashes).
	if !strings.Contains(out, "/") {
		t.Fatalf("expected timestamp in output, got %q", out)
	}
}

// TestParseLogFormatRoundtrip validates all three formatters produce
// non-empty, distinct output for the same input.
func TestFormatterDistinction(t *testing.T) {
	formats := []struct {
		name string
		f    clog.Formatter
	}{
		{"text", clog.TextFormatter},
		{"json", clog.JSONFormatter},
		{"logfmt", clog.LogfmtFormatter},
	}
	var outputs []string
	for _, fmt := range formats {
		var buf bytes.Buffer
		l := newLogger(&buf, fmt.f, clog.InfoLevel, false)
		l.Info("roundtrip", "x", 1)
		outputs = append(outputs, buf.String())
	}
	// All three should produce different output.
	for i := 0; i < len(outputs); i++ {
		for j := i + 1; j < len(outputs); j++ {
			if outputs[i] == outputs[j] {
				t.Fatalf("formats %d and %d produced identical output: %q", i, j, outputs[i])
			}
		}
	}
}

func TestNewLoggerWriteCloser(t *testing.T) {
	// Verify that a Logger backed by a WriteCloser works correctly
	// (simulates --log-file behavior).
	pr, pw := io.Pipe()
	go func() {
		l := newLogger(pw, clog.TextFormatter, clog.InfoLevel, false)
		l.Info("pipe-test")
		pw.Close()
	}()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, pr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "pipe-test") {
		t.Fatalf("expected 'pipe-test' in piped output, got %q", buf.String())
	}
}
