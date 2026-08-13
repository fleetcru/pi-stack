package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type GitFileChange struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

type GitStatus struct {
	Branch       string          `json:"branch"`
	Ahead        int             `json:"ahead"`
	Behind       int             `json:"behind"`
	Staged       []string        `json:"staged"`
	Modified     []string        `json:"modified"`
	Untracked    []string        `json:"untracked"`
	Conflicts    []string        `json:"conflicts"`
	Changes      []GitFileChange `json:"changes"`
	HasUpstream  bool            `json:"hasUpstream"`
	HasRemote    bool            `json:"hasRemote"`
	IsDefault    bool            `json:"isDefault"`
	IsWorktree   bool            `json:"isWorktree"`
	WorktreePath string          `json:"worktreePath,omitempty"`
	DefaultBranch string         `json:"defaultBranch,omitempty"`
	RemoteURL    string          `json:"remoteUrl,omitempty"`
	GitHubRepo   string          `json:"githubRepo,omitempty"`
}

type GitBranch struct {
	Name      string `json:"name"`
	Current   bool   `json:"current"`
	Remote    string `json:"remote,omitempty"`
	IsDefault bool   `json:"isDefault"`
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
	// SetUpstream pushes with -u so a freshly created (upstream-less) branch
	// records its tracking remote in one step.
	SetUpstream bool `json:"setUpstream"`
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
			if status.WorktreePath == "" {
				status.WorktreePath = s.currentWorktreePath(r.Context(), spec.CWD, status.Branch)
			}
			writeJSON(w, http.StatusOK, map[string]any{"cwd": spec.CWD, "status": status})
			return
		}
		s.writeGitText(w, r, spec.CWD, "status", "--short", "--branch")
	case "diff":
		s.writeGitText(w, r, spec.CWD, "diff", "--no-ext-diff")
	case "file-diff":
		s.gitFileDiff(w, r, spec.CWD)
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

func (s *Server) gitFileDiff(w http.ResponseWriter, r *http.Request, cwd string) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" || filepath.IsAbs(path) || path == "." || strings.HasPrefix(path, "../") || strings.Contains(path, "\\..\\") {
		writeErrorText(w, http.StatusBadRequest, "path must be a relative file path")
		return
	}
	candidate := filepath.Join(cwd, filepath.FromSlash(path))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		resolved = candidate
	}
	rootOutput, err := s.runGit(r.Context(), cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		writeGitError(w, err)
		return
	}
	root, err := filepath.Abs(strings.TrimSpace(rootOutput))
	if err != nil {
		writeGitError(w, err)
		return
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		writeErrorText(w, http.StatusBadRequest, "path must be inside the session repository")
		return
	}
	// Git commands run with cwd as their working directory, so the path
	// argument must be relative to cwd rather than the repository root.
	gitPath, err := filepath.Rel(cwd, resolved)
	if err != nil || gitPath == ".." || strings.HasPrefix(gitPath, ".."+string(filepath.Separator)) || filepath.IsAbs(gitPath) {
		writeErrorText(w, http.StatusBadRequest, "path must be inside the session directory")
		return
	}
	gitPath = filepath.ToSlash(gitPath)
	diff, diffErr := s.runGit(r.Context(), cwd, "diff", "HEAD", "--no-ext-diff", "--", gitPath)
	isUntracked := false
	if diffErr == nil && diff == "" {
		statusOutput, statusErr := s.runGit(r.Context(), cwd, "status", "--porcelain=v1", "--", gitPath)
		isUntracked = statusErr == nil && strings.HasPrefix(strings.TrimSpace(statusOutput), "??")
	}
	if diffErr != nil || isUntracked {
		// Untracked files are not included in git diff. Render them as an
		// addition-only patch so the client can use one diff renderer.
		data, readErr := os.ReadFile(candidate)
		if readErr != nil {
			writeGitError(w, diffErr)
			return
		}
		var b strings.Builder
		fmt.Fprintf(&b, "diff --git a/%s b/%s\nnew file\n--- /dev/null\n+++ b/%s\n", filepath.ToSlash(rel), filepath.ToSlash(rel), filepath.ToSlash(rel))
		for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
			b.WriteByte('+')
			b.WriteString(line)
			b.WriteByte('\n')
		}
		diff = b.String()
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": filepath.ToSlash(rel), "diff": diff})
}

// resolveDefaultBranch returns the repository's primary branch name (origin/HEAD
// if available, otherwise `main`, then `master`). Empty string when the repo has
// no recognizable default.
func (s *Server) resolveDefaultBranch(ctx context.Context, cwd string) string {
	if out, err := s.runGit(ctx, cwd, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		name := strings.TrimSpace(out)
		if strings.HasPrefix(name, "origin/") {
			return strings.TrimPrefix(name, "origin/")
		}
		return name
	}
	for _, candidate := range []string{"main", "master"} {
		if _, err := s.runGit(ctx, cwd, "rev-parse", "--verify", "--quiet", "refs/heads/"+candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// normalizeGitRemoteURL returns a stable comparison key for a git remote URL,
// stripping protocol/scheme and trailing .git. Ported from T3 Code (MIT).
func normalizeGitRemoteURL(value string) string {
	normalized := strings.TrimSpace(value)
	normalized = strings.TrimRight(normalized, "/")
	normalized = strings.TrimSuffix(normalized, ".git")
	normalized = strings.ToLower(normalized)

	re := regexp.MustCompile(`^(?:ssh|https?|git)://`)
	if re.MatchString(normalized) {
		if u, err := url.Parse(normalized); err == nil {
			var repoPath []string
			for _, seg := range strings.Split(u.Path, "/") {
				if seg != "" {
					repoPath = append(repoPath, seg)
				}
			}
			joined := strings.Join(repoPath, "/")
			if u.Hostname() != "" && strings.Contains(joined, "/") {
				return u.Hostname() + "/" + joined
			}
		}
		return normalized
	}

	scp := regexp.MustCompile(`^git@([^:/\s]+)[:/]([^/\s]+(?:/[^/\s]+)+)$`)
	if m := scp.FindStringSubmatch(normalized); m != nil {
		return m[1] + "/" + m[2]
	}
	return normalized
}

// parseGitHubRepositoryNameWithOwner returns a best-effort "owner/repo"
// identifier from common GitHub remote URL shapes, or empty string.
// Ported from T3 Code (MIT).
func parseGitHubRepositoryNameWithOwner(remoteURL string) string {
	trimmed := strings.TrimSpace(remoteURL)
	if trimmed == "" {
		return ""
	}
	re := regexp.MustCompile(`(?i)^(?:git@github\.com:|ssh://git@github\.com/|https://github\.com/|git://github\.com/)([^/\s]+/[^/\s]+?)(?:\.git)?/?$`)
	m := re.FindStringSubmatch(trimmed)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func (s *Server) gitStatus(ctx context.Context, cwd string) (GitStatus, error) {
	output, err := s.runGit(ctx, cwd, "status", "--porcelain=v1", "--branch", "--ahead-behind")
	if err != nil {
		return GitStatus{}, err
	}
	status := GitStatus{Staged: []string{}, Modified: []string{}, Untracked: []string{}, Conflicts: []string{}, Changes: []GitFileChange{}}
	status.DefaultBranch = s.resolveDefaultBranch(ctx, cwd)
	status.IsWorktree = isGitWorktree(ctx, s, cwd)
	// hasRemote: session repo has at least one remote configured.
	if out, err := s.runGit(ctx, cwd, "remote"); err == nil {
		status.HasRemote = len(strings.TrimSpace(out)) > 0
	}
	// Resolve the origin remote URL for client-side "open PR/compare" links.
	if out, err := s.runGit(ctx, cwd, "remote", "get-url", "origin"); err == nil {
		status.RemoteURL = strings.TrimSpace(out)
		status.GitHubRepo = parseGitHubRepositoryNameWithOwner(status.RemoteURL)
	}
	changeIndex := make(map[string]int)
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
			// Upstream present means the branch has a configured tracking remote.
			status.HasUpstream = strings.Contains(line, "...")
			status.IsDefault = status.DefaultBranch != "" && branch == status.DefaultBranch
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
			changeIndex[path] = len(status.Changes)
			change := GitFileChange{Path: path, Status: "A"}
			if data, readErr := os.ReadFile(filepath.Join(cwd, filepath.FromSlash(path))); readErr == nil && len(data) > 0 {
				text := string(data)
				change.Additions = strings.Count(text, "\n")
				if !strings.HasSuffix(text, "\n") {
					change.Additions++
				}
			}
			status.Changes = append(status.Changes, change)
			continue
		}
		if x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D') {
			status.Conflicts = append(status.Conflicts, path)
		}
		changeIndex[path] = len(status.Changes)
		changeStatus := string(y)
		if changeStatus == " " {
			changeStatus = string(x)
		}
		if changeStatus == "" || changeStatus == " " {
			changeStatus = "M"
		}
		status.Changes = append(status.Changes, GitFileChange{Path: path, Status: changeStatus})
		if x != ' ' {
			status.Staged = append(status.Staged, path)
		}
		if y != ' ' {
			status.Modified = append(status.Modified, path)
		}
	}
	for _, args := range [][]string{{"diff", "--numstat"}, {"diff", "--cached", "--numstat"}} {
		numstat, err := s.runGit(ctx, cwd, args...)
		if err != nil {
			return GitStatus{}, err
		}
		for _, line := range strings.Split(numstat, "\n") {
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) != 3 {
				continue
			}
			path := parts[2]
			idx, ok := changeIndex[path]
			if !ok {
				continue
			}
			if parts[0] != "-" {
				additions, _ := strconv.Atoi(parts[0])
				status.Changes[idx].Additions += additions
			}
			if parts[1] != "-" {
				deletions, _ := strconv.Atoi(parts[1])
				status.Changes[idx].Deletions += deletions
			}
		}
	}
	return status, nil
}

func (s *Server) gitBranches(ctx context.Context, cwd string) ([]GitBranch, error) {
	// Use %09 (not \t) as the field separator: git's --format treats %09 as an
	// unambiguous tab and passes a literal backslash-t or raw tab through in a
	// way that shifts the %(HEAD) padding when a branch is not current.
	output, err := s.runGit(ctx, cwd, "for-each-ref", "--format=%(HEAD)%09%(refname:short)%09%(upstream:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	branches := make([]GitBranch, 0)
	defaultBranch := s.resolveDefaultBranch(ctx, cwd)
	// Split on newlines WITHOUT trimming leading whitespace: %(HEAD) pads non-current
	// branches with a leading SPACE, and any whole-line TrimSpace would strip that
	// space + following tab, silently losing that branch's name. Only trim the
	// trailing newline/CR (Windows) and trailing empty fields.
	for _, rawLine := range strings.Split(output, "\n") {
		rawLine = strings.TrimSuffix(rawLine, "\r")
		if strings.TrimSpace(rawLine) == "" {
			continue
		}
		parts := strings.SplitN(rawLine, "\t", 3)
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			continue
		}
		branch := GitBranch{Name: parts[1], Current: strings.HasPrefix(parts[0], "*")}
		if len(parts) >= 3 && parts[2] != "" {
			branch.Remote = parts[2]
		}
		branch.IsDefault = defaultBranch != "" && parts[1] == defaultBranch
		branches = append(branches, branch)
	}
	// Sort current branch first, then default, then alphabetically.
	sort.SliceStable(branches, func(i, j int) bool {
		bi, bj := branches[i], branches[j]
		if bi.Current != bj.Current {
			return bi.Current
		}
		if bi.IsDefault != bj.IsDefault {
			return bi.IsDefault
		}
		return bi.Name < bj.Name
	})
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

// isGitWorktree reports whether cwd is (or is inside) a linked git worktree,
// as opposed to the repository's primary working tree.
func isGitWorktree(ctx context.Context, s *Server, cwd string) bool {
	out, err := s.runGit(ctx, cwd, "worktree", "list", "--porcelain")
	if err != nil {
		return false
	}
	primary := ""
	ext := map[string]bool{}
	var current string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			current = strings.TrimPrefix(line, "worktree ")
			if primary == "" {
				primary = current
			}
		case current != "" && strings.HasPrefix(line, "branch refs/heads/"):
			ext[current] = true
		}
	}
	if primary == "" {
		return false
	}
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return false
	}
	if absCwd == primary || strings.HasPrefix(absCwd, primary+string(filepath.Separator)) {
		return false
	}
	return true
}

// currentWorktreePath returns the linked worktree path whose branch matches the
// session's checked-out branch, or "" when cwd is the primary working tree.
func (s *Server) currentWorktreePath(ctx context.Context, cwd, branch string) string {
	if branch == "" {
		return ""
	}
	worktrees, err := s.gitWorktrees(ctx, cwd)
	if err != nil {
		return ""
	}
	for _, item := range worktrees {
		if item.Branch == branch {
			return item.Path
		}
	}
	return ""
}

// sanitizeBranchFragment converts an arbitrary string into a valid, lowercase,
// slash-propagating git refName fragment (max 64 chars). Falls back to "update".
func sanitizeBranchFragment(raw string) string {
	normalized := strings.TrimSpace(raw)
	normalized = strings.ToLower(normalized)
	replacer := strings.NewReplacer(`'`, "", `"`, "", "`", "")
	normalized = replacer.Replace(normalized)
	normalized = strings.TrimLeft(normalized, "./\t-_")
	normalized = strings.TrimRight(normalized, " .\t-_")

	re := regexp.MustCompile(`[^a-z0-9/_-]+`)
	fragment := re.ReplaceAllString(normalized, "-")
	fragment = regexp.MustCompile(`/+/`).ReplaceAllString(fragment, "/")
	fragment = regexp.MustCompile(`-+`).ReplaceAllString(fragment, "-")
	fragment = strings.TrimLeft(fragment, "./_-")
	fragment = strings.TrimRight(fragment, "./_- ")
	if len(fragment) > 64 {
		fragment = fragment[:64]
	}
	fragment = strings.TrimRight(fragment, "./_- ")
	if fragment == "" {
		return "update"
	}
	return fragment
}

// sanitizeFeatureBranchName converts an arbitrary string into a feature-or-
// namespaced git refName, mirroring T3 Code's contract:
//   - a title that already carries a `feature/…` prefix is preserved verbatim
//     (no double `feature/feature/…`);
//   - any other slash-namespace is preserved inside the feature prefix, e.g.
//     `docs/readme` → `feature/docs/readme`;
//   - a bare label becomes `feature/<label>`.
func sanitizeFeatureBranchName(raw string) string {
	sanitized := sanitizeBranchFragment(raw)
	if strings.Contains(sanitized, "/") {
		if strings.HasPrefix(sanitized, "feature/") {
			return sanitized
		}
		return "feature/" + sanitized
	}
	return "feature/" + sanitized
}

// resolveAutoFeatureBranchName returns a unique feature/<sanitized> name that
// does not collide with any existing branch, appending a numeric suffix as
// needed. Mirrors T3 Code's auto-branch naming to keep agent branches warm.
func (s *Server) resolveAutoFeatureBranchName(ctx context.Context, cwd, preferred string) (string, error) {
	base := sanitizeFeatureBranchName(preferred)
	if base == "feature/update" {
		base = "feature/agent"
	}
	branches, err := s.gitBranches(ctx, cwd)
	if err != nil {
		return "", err
	}
	existing := make(map[string]bool, len(branches))
	for _, b := range branches {
		existing[b.Name] = true
	}
	candidate := base
	if !existing[candidate] {
		return candidate, nil
	}
	for suffix := 2; suffix < 1000; suffix++ {
		candidate = fmt.Sprintf("%s-%d", base, suffix)
		if !existing[candidate] {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("unable to allocate a unique feature branch")
}

// createAutoWorktree creates a linked worktree under <repoRoot>/.pi-worktrees/
// on a freshly created feature branch (based on the repo default branch). It
// returns the created path and branch name. The caller is responsible for
// recording these on the SessionSpec.
func (s *Server) createAutoWorktree(ctx context.Context, cwd, title string) (string, string, error) {
	rootOutput, err := s.runGit(ctx, cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", err
	}
	repoRoot, err := filepath.Abs(strings.TrimSpace(rootOutput))
	if err != nil {
		return "", "", err
	}
	defaultBranch := s.resolveDefaultBranch(ctx, cwd)
	if defaultBranch == "" {
		return "", "", fmt.Errorf("cannot create worktree: no default branch (main/master) found")
	}
	branch, err := s.resolveAutoFeatureBranchName(ctx, cwd, title)
	if err != nil {
		return "", "", err
	}
	dirName := sanitizeBranchFragment(title)
	if dirName == "update" {
		dirName = "agent"
	}
	path := filepath.Join(repoRoot, ".pi-worktrees", dirName)
	// De-conflict the directory name if it already exists.
	for suffix := 2; ; suffix++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		path = filepath.Join(repoRoot, ".pi-worktrees", fmt.Sprintf("%s-%d", dirName, suffix))
		if suffix > 1000 {
			return "", "", fmt.Errorf("unable to allocate a unique worktree directory")
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", "", err
	}
	if _, err := s.runGit(ctx, cwd, "worktree", "add", "-b", branch, path, defaultBranch); err != nil {
		return "", "", err
	}
	// Ensure the parent repo ignores the linked worktrees directory so it
	// doesn't show up as untracked. Best-effort: never block creation on it.
	s.ensureWorktreesGitignored(repoRoot)
	return path, branch, nil
}

// ensureWorktreesGitignored makes sure <repoRoot>/.gitignore ignores the
// .pi-worktrees/ directory created by createAutoWorktree, in any repo the user
// runs a session in (not just ones that already ignore it). Best-effort:
// errors are swallowed because they must never block worktree creation.
func (s *Server) ensureWorktreesGitignored(repoRoot string) {
	const entry = ".pi-worktrees/"
	gitignore := filepath.Join(repoRoot, ".gitignore")
	existing, rerr := os.ReadFile(gitignore)
	if rerr == nil && strings.Contains(string(existing), entry) {
		return
	}
	content := string(existing)
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "# pi-server auto-created per-session worktrees\n" + entry + "\n"
	// Atomic-ish write via temp file + rename so a crash can't truncate it.
	tmp := gitignore + ".pi-worktree.tmp"
	if werr := os.WriteFile(tmp, []byte(content), 0644); werr != nil {
		return
	}
	if rerr := os.Rename(tmp, gitignore); rerr != nil {
		_ = os.Remove(tmp)
	}
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
			args = []string{"push"}
			if req.SetUpstream {
				args = append(args, "-u")
			}
			args = append(args, remote, branch)
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
	for _, root := range s.resolvedRoots {
		rel, err := filepath.Rel(root, absPath)
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
	applyProcessAttrs(cmd)
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
