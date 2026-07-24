package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type GitStatus struct {
	Branch    string   `json:"branch"`
	Ahead     int      `json:"ahead"`
	Behind    int      `json:"behind"`
	Staged    []string `json:"staged"`
	Modified  []string `json:"modified"`
	Untracked []string `json:"untracked"`
	Conflicts []string `json:"conflicts"`
}

type GitBranch struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
	Remote  string `json:"remote,omitempty"`
}

type GitWorktree struct {
	Path     string `json:"path"`
	Head     string `json:"head"`
	Branch   string `json:"branch,omitempty"`
	Detached bool   `json:"detached"`
}

type gitWorktreeRequest struct {
	Path           string `json:"path"`
	Branch         string `json:"branch"`
	StartPoint     string `json:"startPoint"`
	ExistingBranch bool   `json:"existingBranch"`
}

type gitCommitRequest struct {
	Message  string `json:"message"`
	StageAll bool   `json:"stageAll"`
}

type gitMergeRequest struct {
	Branch string `json:"branch"`
}

type gitRemoteRequest struct {
	Remote string `json:"remote"`
	Branch string `json:"branch"`
}

func (s *Server) gitHandler(w http.ResponseWriter, r *http.Request) {
	id, action := splitSessionPath(r.URL.Path)
	if !strings.HasPrefix(action, "git/") {
		http.NotFound(w, r)
		return
	}
	if s.proxyRemoteSession(w, r, id, action) {
		return
	}
	spec, ok := s.sessions.GetSpec(id)
	if !ok {
		writeErrorText(w, http.StatusNotFound, "session not found")
		return
	}
	resource := strings.TrimPrefix(action, "git/")
	if r.Method != http.MethodGet && (resource == "worktrees" || resource == "commit" || resource == "merge" || resource == "merge-abort" || resource == "pull" || resource == "push") {
		s.gitWriteOperation(w, r, spec, id, resource)
		return
	}
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	switch resource {
	case "status":
		if r.URL.Query().Get("format") == "json" {
			status, err := s.gitStatus(r.Context(), spec.CWD)
			if err != nil {
				writeGitError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"cwd": spec.CWD, "status": status})
			return
		}
		s.writeGitText(w, r, spec.CWD, "status", "--short", "--branch")
	case "diff":
		s.writeGitText(w, r, spec.CWD, "diff", "--no-ext-diff")
	case "log":
		s.writeGitText(w, r, spec.CWD, "log", "--oneline", "-n", "20")
	case "head":
		s.writeGitText(w, r, spec.CWD, "log", "-1", "--format=%H%n%h%n%s%n%an%n%cI")
	case "branches":
		branches, err := s.gitBranches(r.Context(), spec.CWD)
		if err != nil {
			writeGitError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"cwd": spec.CWD, "branches": branches})
	case "worktrees":
		worktrees, err := s.gitWorktrees(r.Context(), spec.CWD)
		if err != nil {
			writeGitError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"cwd": spec.CWD, "worktrees": worktrees})
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) writeGitText(w http.ResponseWriter, r *http.Request, cwd, name string, args ...string) {
	output, err := s.runGit(r.Context(), cwd, append([]string{name}, args...)...)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cwd": cwd, "command": "git " + strings.Join(append([]string{name}, args...), " "), "output": output})
}

func writeGitError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errorsIsNotRepo(err) {
		status = http.StatusUnprocessableEntity
	}
	writeError(w, status, err)
}

func errorsIsNotRepo(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "not a git repository")
}

func (s *Server) gitStatus(ctx context.Context, cwd string) (GitStatus, error) {
	output, err := s.runGit(ctx, cwd, "status", "--porcelain=v1", "--branch", "--ahead-behind")
	if err != nil {
		return GitStatus{}, err
	}
	status := GitStatus{Staged: []string{}, Modified: []string{}, Untracked: []string{}, Conflicts: []string{}}
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			branch := strings.TrimPrefix(line, "## ")
			branch = strings.Split(branch, "...")[0]
			if i := strings.Index(branch, " "); i >= 0 {
				branch = branch[:i]
			}
			status.Branch = branch
			if i := strings.Index(line, "[ahead "); i >= 0 {
				fields := strings.Fields(line[i+7:])
				if len(fields) > 0 {
					status.Ahead, _ = strconv.Atoi(fields[0])
				}
			}
			if i := strings.Index(line, "behind "); i >= 0 {
				fields := strings.Fields(line[i+7:])
				if len(fields) > 0 {
					status.Behind, _ = strconv.Atoi(fields[0])
				}
			}
			continue
		}
		if len(line) < 3 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if arrow := strings.Index(path, " -> "); arrow >= 0 {
			path = path[arrow+4:]
		}
		x, y := line[0], line[1]
		if x == '?' && y == '?' {
			status.Untracked = append(status.Untracked, path)
			continue
		}
		if x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D') {
			status.Conflicts = append(status.Conflicts, path)
			continue
		}
		if x != ' ' {
			status.Staged = append(status.Staged, path)
		}
		if y != ' ' {
			status.Modified = append(status.Modified, path)
		}
	}
	return status, nil
}

func (s *Server) gitBranches(ctx context.Context, cwd string) ([]GitBranch, error) {
	output, err := s.runGit(ctx, cwd, "for-each-ref", "--format=%(HEAD)\t%(refname:short)\t%(upstream:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	branches := make([]GitBranch, 0)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		branch := GitBranch{Name: parts[1], Current: parts[0] == "*"}
		if len(parts) == 3 {
			branch.Remote = parts[2]
		}
		branches = append(branches, branch)
	}
	return branches, nil
}

func (s *Server) gitWorktrees(ctx context.Context, cwd string) ([]GitWorktree, error) {
	output, err := s.runGit(ctx, cwd, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	worktrees := make([]GitWorktree, 0)
	var current *GitWorktree
	flush := func() {
		if current != nil {
			worktrees = append(worktrees, *current)
			current = nil
		}
	}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			current = &GitWorktree{Path: strings.TrimPrefix(line, "worktree ")}
		case current != nil && strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimPrefix(line, "HEAD ")
		case current != nil && strings.HasPrefix(line, "branch "):
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		case current != nil && line == "detached":
			current.Detached = true
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return worktrees, nil
}

func (s *Server) gitWriteOperation(w http.ResponseWriter, r *http.Request, spec SessionSpec, sessionID, resource string) {
	if resource == "commit" {
		var req gitCommitRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil || strings.TrimSpace(req.Message) == "" {
			writeErrorText(w, http.StatusBadRequest, "commit message is required")
			return
		}
		if len(req.Message) > 500 {
			writeErrorText(w, http.StatusBadRequest, "commit message is too long")
			return
		}
		if req.StageAll {
			if _, err := s.runGit(r.Context(), spec.CWD, "add", "--all", "--"); err != nil {
				writeGitError(w, err)
				return
			}
		}
		output, err := s.runGit(r.Context(), spec.CWD, "commit", "-m", req.Message)
		if err != nil {
			writeGitError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessionId": sessionID, "output": output})
		return
	}
	if resource == "merge-abort" {
		output, err := s.runGit(r.Context(), spec.CWD, "merge", "--abort")
		if err != nil {
			writeGitError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessionId": sessionID, "output": output})
		return
	}
	if resource == "pull" || resource == "push" {
		var req gitRemoteRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		remote := strings.TrimSpace(req.Remote)
		if remote == "" {
			remote = "origin"
		}
		if _, err := s.runGit(r.Context(), spec.CWD, "remote", "get-url", remote); err != nil {
			writeErrorText(w, http.StatusBadRequest, "unknown Git remote")
			return
		}
		branch := strings.TrimSpace(req.Branch)
		if branch == "" {
			branchOutput, err := s.runGit(r.Context(), spec.CWD, "branch", "--show-current")
			if err != nil {
				writeGitError(w, err)
				return
			}
			branch = strings.TrimSpace(branchOutput)
		}
		if branch == "" {
			writeErrorText(w, http.StatusBadRequest, "branch is required for a detached HEAD")
			return
		}
		var args []string
		if resource == "pull" {
			args = []string{"pull", "--ff-only", remote, branch}
		} else {
			args = []string{"push", remote, branch}
		}
		output, err := s.runGit(r.Context(), spec.CWD, args...)
		if err != nil {
			if resource == "pull" {
				if status, statusErr := s.gitStatus(r.Context(), spec.CWD); statusErr == nil && len(status.Conflicts) > 0 {
					writeJSON(w, http.StatusConflict, map[string]any{"error": "pull resulted in conflicts", "conflicts": status.Conflicts, "output": output})
					return
				}
			}
			writeGitError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessionId": sessionID, "remote": remote, "branch": branch, "output": output})
		return
	}
	if resource == "merge" {
		var req gitMergeRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil || strings.TrimSpace(req.Branch) == "" {
			writeErrorText(w, http.StatusBadRequest, "branch is required")
			return
		}
		status, err := s.gitStatus(r.Context(), spec.CWD)
		if err != nil {
			writeGitError(w, err)
			return
		}
		if len(status.Staged)+len(status.Modified)+len(status.Untracked)+len(status.Conflicts) > 0 {
			writeErrorText(w, http.StatusConflict, "working tree must be clean before merging")
			return
		}
		if _, err := s.runGit(r.Context(), spec.CWD, "check-ref-format", "--branch", req.Branch); err != nil {
			writeErrorText(w, http.StatusBadRequest, "invalid branch name")
			return
		}
		output, err := s.runGit(r.Context(), spec.CWD, "merge", "--no-edit", "--", req.Branch)
		if err != nil {
			if status, statusErr := s.gitStatus(r.Context(), spec.CWD); statusErr == nil && len(status.Conflicts) > 0 {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "merge resulted in conflicts", "conflicts": status.Conflicts, "output": output})
				return
			}
			writeGitError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessionId": sessionID, "output": output})
		return
	}
	if resource != "worktrees" {
		http.NotFound(w, r)
		return
	}
	s.gitWorktreeMutation(w, r, spec, sessionID)
}

func (s *Server) gitWorktreeMutation(w http.ResponseWriter, r *http.Request, spec SessionSpec, sessionID string) {
	var req gitWorktreeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Path == "" {
		writeErrorText(w, http.StatusBadRequest, "path is required")
		return
	}
	path, err := s.validateWorktreePath(spec.CWD, req.Path, r.Method == http.MethodDelete)
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if r.Method == http.MethodPost {
		args := []string{"worktree", "add"}
		if req.Branch == "" {
			writeErrorText(w, http.StatusBadRequest, "branch is required")
			return
		}
		if req.ExistingBranch {
			args = append(args, path, req.Branch)
		} else {
			args = append(args, "-b", req.Branch, path)
			if req.StartPoint != "" {
				args = append(args, req.StartPoint)
			}
		}
		if _, err := s.runGit(r.Context(), spec.CWD, args...); err != nil {
			writeGitError(w, err)
			return
		}
	} else if r.Method == http.MethodDelete {
		known, listErr := s.gitWorktrees(r.Context(), spec.CWD)
		if listErr != nil {
			writeGitError(w, listErr)
			return
		}
		knownPath := false
		for _, item := range known {
			itemPath, _ := filepath.Abs(filepath.Clean(item.Path))
			if filepath.Clean(itemPath) == filepath.Clean(path) {
				knownPath = true
				break
			}
		}
		if !knownPath {
			writeErrorText(w, http.StatusNotFound, "worktree is not owned by this repository")
			return
		}
		if _, err := s.runGit(r.Context(), spec.CWD, "worktree", "remove", path); err != nil {
			writeGitError(w, err)
			return
		}
	} else {
		http.NotFound(w, r)
		return
	}
	worktrees, err := s.gitWorktrees(r.Context(), spec.CWD)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessionId": sessionID, "worktrees": worktrees})
}

func (s *Server) validateWorktreePath(cwd, requested string, deleting bool) (string, error) {
	path := requested
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	candidate := path
	if deleting {
		candidate, err = filepath.EvalSymlinks(path)
		if err != nil {
			return "", fmt.Errorf("cannot resolve worktree: %w", err)
		}
	} else {
		parent := filepath.Dir(path)
		if _, err := os.Stat(parent); err != nil {
			return "", fmt.Errorf("worktree parent is unavailable: %w", err)
		}
		parent, err = filepath.EvalSymlinks(parent)
		if err != nil {
			return "", err
		}
		candidate = filepath.Join(parent, filepath.Base(path))
	}
	if !s.pathWithinAllowedRoots(candidate) {
		return "", fmt.Errorf("worktree path is outside PI_SERVER_ALLOWED_ROOTS")
	}
	repoRoot, err := s.runGit(context.Background(), cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("session directory is not a Git repository: %w", err)
	}
	repoRoot, err = filepath.Abs(strings.TrimSpace(repoRoot))
	if err != nil {
		return "", err
	}
	checkPath := candidate
	if !deleting {
		checkPath = filepath.Dir(candidate)
	}
	rel, err := filepath.Rel(repoRoot, checkPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worktree path must be inside the session repository")
	}
	if deleting {
		if _, err := os.Stat(candidate); err != nil {
			return "", err
		}
	} else if _, err := os.Stat(candidate); err == nil {
		return "", fmt.Errorf("worktree path already exists")
	}
	return candidate, nil
}

func (s *Server) pathWithinAllowedRoots(path string) bool {
	if len(s.cfg.AllowedRoots) == 0 {
		return true
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, root := range s.cfg.AllowedRoots {
		absRoot, err := filepath.Abs(strings.TrimSpace(root))
		if err != nil {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(absRoot); err == nil {
			absRoot = resolved
		}
		rel, err := filepath.Rel(absRoot, absPath)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

func (s *Server) runGit(parent context.Context, cwd string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, s.cfg.RequestTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message != "" {
			return message, fmt.Errorf("git %s: %s", strings.Join(args, " "), message)
		}
		return string(out), err
	}
	return string(out), nil
}
