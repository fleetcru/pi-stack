package server

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestRunGitStatus(t *testing.T) {
	cwd := t.TempDir()
	for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = cwd
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if err := os.WriteFile(cwd+"/a.txt", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	s := New(Config{RequestTimeout: time.Second, DataDir: t.TempDir()}, testLogger())
	out, err := s.runGit(context.Background(), cwd, "status", "--short")
	if err != nil || out == "" {
		t.Fatalf("status: %v %q", err, out)
	}
}
