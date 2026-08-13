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
	if !body.Status.IsDefault {
		t.Fatalf("expected main to be flagged as default branch")
	}
	if body.Status.HasRemote {
		t.Fatalf("bare test repo should have no remote")
	}
}

func TestGitStatusWorktreeDetection(t *testing.T) {
	cwd := gitTestRepo(t)
	s := New(Config{RequestTimeout: time.Second, DataDir: t.TempDir(), AllowedRoots: []string{cwd}}, testLogger())
	if _, err := s.sessions.RegisterSpec(SessionSpec{ID: "test", CWD: cwd, Transport: "rpc"}); err != nil {
		t.Fatal(err)
	}
	worktreePath := filepath.Join(cwd, ".pi-worktrees", "feature")
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		t.Fatal(err)
	}
	body := `{"path":".pi-worktrees/feature","branch":"feature/test","startPoint":"main"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/test/git/worktrees", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.gitHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create worktree: %d %s", rec.Code, rec.Body.String())
	}
	// Register a session whose cwd is the worktree.
	if _, err := s.sessions.RegisterSpec(SessionSpec{ID: "wt", CWD: worktreePath, Transport: "rpc"}); err != nil {
		t.Fatal(err)
	}
	statusReq := httptest.NewRequest(http.MethodGet, "/v1/sessions/wt/git/status?format=json", nil)
	statusRec := httptest.NewRecorder()
	s.gitHandler(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("status: %d %s", statusRec.Code, statusRec.Body.String())
	}
	var out struct {
		Status GitStatus `json:"status"`
	}
	if err := json.Unmarshal(statusRec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Status.IsWorktree {
		t.Fatalf("expected IsWorktree=true for worktree session, got %+v", out.Status)
	}
	if out.Status.IsDefault {
		t.Fatalf("feature branch should not be default, got %+v", out.Status)
	}
}

func TestBuildSessionSpecAutoWorktree(t *testing.T) {
	cwd := gitTestRepo(t)
	s := New(Config{RequestTimeout: time.Second, DataDir: t.TempDir(), AllowedRoots: []string{cwd}}, testLogger())
	req := createSessionRequest{
		CWD:            cwd,
		Title:          "Fix login bug",
		CreateWorktree: &createWorktreeOptions{Enabled: true},
	}
	spec, err := s.buildSessionSpec(req)
	if err != nil {
		t.Fatalf("buildSessionSpec: %v", err)
	}
	if spec.WorktreePath == "" {
		t.Fatalf("expected auto worktree path to be set")
	}
	if _, err := os.Stat(spec.CWD); err != nil {
		t.Fatalf("worktree dir not created: %v", err)
	}
	if spec.Metadata["worktreeBranch"] == "" {
		t.Fatalf("expected worktreeBranch metadata")
	}
	branch := spec.Metadata["worktreeBranch"]
	if !strings.HasPrefix(branch, "feature/") {
		t.Fatalf("expected feature/ branch, got %q", branch)
	}
	// Deleting the spec should clean up the worktree.
	if err := s.removeOwnedWorktree(spec); err != nil {
		t.Fatalf("removeOwnedWorktree: %v", err)
	}
	if _, err := os.Stat(spec.CWD); !os.IsNotExist(err) {
		t.Fatalf("worktree dir still exists after cleanup")
	}
}

func TestAutoWorktreeBranchDeconfliction(t *testing.T) {
	cwd := gitTestRepo(t)
	s := New(Config{RequestTimeout: time.Second, DataDir: t.TempDir(), AllowedRoots: []string{cwd}}, testLogger())
	mk := func(title string) string {
		req := createSessionRequest{CWD: cwd, Title: title, CreateWorktree: &createWorktreeOptions{Enabled: true}}
		spec, err := s.buildSessionSpec(req)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		return spec.Metadata["worktreeBranch"]
	}
	first := mk("Refactor auth")
	second := mk("Refactor auth")
	if first == second {
		t.Fatalf("expected deconflicted branch names, got %q and %q", first, second)
	}
	if !strings.HasSuffix(second, "-2") && !strings.HasSuffix(second, "-3") {
		t.Fatalf("expected numeric suffix on second branch, got %q", second)
	}
}

func TestSanitizeFeatureBranchName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Refactor auth", "feature/refactor-auth"},
		{"feature/demo", "feature/demo"},        // no double feature/ prefix
		{"docs/readme", "feature/docs/readme"}, // preserved namespace
		{"!!!", "feature/update"},              // fallback label
		{"feature/Renamed", "feature/renamed"},
	}
	for _, c := range cases {
		if got := sanitizeFeatureBranchName(c.in); got != c.want {
			t.Errorf("sanitizeFeatureBranchName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseGitHubRepositoryNameWithOwner(t *testing.T) {
	cases := []struct {
		url, want string
	}{
		{"git@github.com:owner/repo.git", "owner/repo"},
		{"https://github.com/owner/repo", "owner/repo"},
		{"ssh://git@github.com/owner/repo.git", "owner/repo"},
		{"https://github.com/abc/def/?x=1", ""}, // not a direct repo URL
		{"", ""},
	}
	for _, c := range cases {
		if got := parseGitHubRepositoryNameWithOwner(c.url); got != c.want {
			t.Errorf("parseGitHubRepositoryNameWithOwner(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestNormalizeGitRemoteURL(t *testing.T) {
	if got := normalizeGitRemoteURL("https://github.com/owner/repo.git"); got != "github.com/owner/repo" {
		t.Errorf("normalize https = %q, want github.com/owner/repo", got)
	}
	if got := normalizeGitRemoteURL("git@github.com:owner/repo.git"); got != "github.com/owner/repo" {
		t.Errorf("normalize scp = %q, want github.com/owner/repo", got)
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

func TestEnsureWorktreesGitignored(t *testing.T) {
	s := &Server{}
	const entry = ".pi-worktrees/"

	t.Run("creates gitignore when missing", func(t *testing.T) {
		dir := t.TempDir()
		s.ensureWorktreesGitignored(dir)
		got, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if err != nil {
			t.Fatalf("expected .gitignore to be created: %v", err)
		}
		if !strings.Contains(string(got), entry) {
			t.Fatalf("gitignore missing entry: %q", string(got))
		}
	})

	t.Run("idempotent on repeated calls", func(t *testing.T) {
		dir := t.TempDir()
		s.ensureWorktreesGitignored(dir)
		s.ensureWorktreesGitignored(dir)
		got, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if n := strings.Count(string(got), entry); n != 1 {
			t.Fatalf("expected exactly 1 entry, got %d in: %q", n, string(got))
		}
	})

	t.Run("preserves existing content and appends", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n*.log"), 0644); err != nil {
		t.Fatal(err)
		}
		s.ensureWorktreesGitignored(dir)
		got, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
		c := string(got)
		if !strings.Contains(c, "node_modules/") || !strings.Contains(c, "*.log") {
			t.Fatalf("existing content lost: %q", c)
		}
		if !strings.Contains(c, entry) {
			t.Fatalf("entry not appended: %q", c)
		}
		if !strings.Contains(c, "node_modules/\n") {
			t.Fatalf("no newline preserved between entries: %q", c)
		}
	})

	t.Run("leaves already-ignored repo untouched", func(t *testing.T) {
		dir := t.TempDir()
		preexisting := "node_modules/\n.pi-worktrees/\n"
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(preexisting), 0644); err != nil {
			t.Fatal(err)
		}
		s.ensureWorktreesGitignored(dir)
		got, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if string(got) != preexisting {
			t.Fatalf("expected file unchanged, got: %q", string(got))
		}
	})
}

