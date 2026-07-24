package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func gitTestRepo(t *testing.T) string {
	t.Helper()
	cwd := t.TempDir()
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = cwd
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(cwd, "README.md"), []byte("initial\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = cwd
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v %s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = cwd
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v %s", err, out)
	}
	return cwd
}

func TestGitStatusStructured(t *testing.T) {
	cwd := gitTestRepo(t)
	if err := os.WriteFile(filepath.Join(cwd, "changed.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	s := New(Config{RequestTimeout: time.Second, DataDir: t.TempDir(), AllowedRoots: []string{cwd}}, testLogger())
	if _, err := s.sessions.RegisterSpec(SessionSpec{ID: "test", CWD: cwd, Transport: "rpc"}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/test/git/status?format=json", nil)
	rec := httptest.NewRecorder()
	s.gitHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status code: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Status GitStatus `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status.Branch != "main" || len(body.Status.Untracked) != 1 || body.Status.Untracked[0] != "changed.txt" {
		t.Fatalf("unexpected status: %+v", body.Status)
	}
}

func TestGitWorktreeLifecycle(t *testing.T) {
	cwd := gitTestRepo(t)
	s := New(Config{RequestTimeout: time.Second, DataDir: t.TempDir(), AllowedRoots: []string{cwd}}, testLogger())
	if _, err := s.sessions.RegisterSpec(SessionSpec{ID: "test", CWD: cwd, Transport: "rpc"}); err != nil {
		t.Fatal(err)
	}
	worktreePath := filepath.Join(cwd, ".pi-worktrees", "feature")
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		t.Fatal(err)
	}
	body := `{"path":".pi-worktrees/feature","branch":"agent/feature","startPoint":"main"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/test/git/worktrees", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.gitHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create code: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(worktreePath); err != nil {
		t.Fatalf("worktree was not created: %v", err)
	}
	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/sessions/test/git/worktrees", strings.NewReader(`{"path":".pi-worktrees/feature"}`))
	deleteRec := httptest.NewRecorder()
	s.gitHandler(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete code: %d %s", deleteRec.Code, deleteRec.Body.String())
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists: %v", err)
	}
}

func TestGitCommitAndMerge(t *testing.T) {
	cwd := gitTestRepo(t)
	s := New(Config{RequestTimeout: time.Second, DataDir: t.TempDir(), AllowedRoots: []string{cwd}}, testLogger())
	if _, err := s.sessions.RegisterSpec(SessionSpec{ID: "test", CWD: cwd, Transport: "rpc"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "feature.txt"), []byte("feature\n"), 0644); err != nil {
		t.Fatal(err)
	}
	commitReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/test/git/commit", strings.NewReader(`{"message":"add feature","stageAll":true}`))
	commitRec := httptest.NewRecorder()
	s.gitHandler(commitRec, commitReq)
	if commitRec.Code != http.StatusOK {
		t.Fatalf("commit code: %d %s", commitRec.Code, commitRec.Body.String())
	}
	for _, args := range [][]string{{"checkout", "-b", "feature"}, {"checkout", "main"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = cwd
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	mergeReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/test/git/merge", strings.NewReader(`{"branch":"feature"}`))
	mergeRec := httptest.NewRecorder()
	s.gitHandler(mergeRec, mergeReq)
	if mergeRec.Code != http.StatusOK {
		t.Fatalf("merge code: %d %s", mergeRec.Code, mergeRec.Body.String())
	}
}

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
