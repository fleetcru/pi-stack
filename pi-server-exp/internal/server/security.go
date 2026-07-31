package server

import (
	"fmt"
	"path/filepath"
	"strings"
)

func hostAllowed(host string, allowed []string) bool {
	for _, value := range allowed {
		if strings.EqualFold(strings.TrimSpace(value), host) {
			return true
		}
	}
	return false
}

// resolveAllowedRoots pre-resolves symlinks on all allowed roots at startup
// so allowedCWD() doesn't need to call EvalSymlinks on every request.
func resolveAllowedRoots(roots []string) []string {
	resolved := make([]string, 0, len(roots))
	for _, root := range roots {
		abs, err := filepath.Abs(strings.TrimSpace(root))
		if err != nil {
			resolved = append(resolved, root)
			continue
		}
		if resolved_path, err := filepath.EvalSymlinks(abs); err == nil {
			resolved = append(resolved, resolved_path)
		} else {
			resolved = append(resolved, abs)
		}
	}
	return resolved
}

func (s *Server) allowedCWD(requested string) (string, error) {
	cwd, err := normalizeCWD(requested, s.cfg.CWD)
	if err != nil {
		return "", err
	}
	// Resolve symlinks on the requested path
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", fmt.Errorf("cannot resolve path: %w", err)
	}
	if len(s.resolvedRoots) == 0 {
		return cwd, nil
	}
	for _, abs := range s.resolvedRoots {
		rel, err := filepath.Rel(abs, cwd)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return cwd, nil
		}
	}
	return "", fmt.Errorf("cwd is outside PI_SERVER_ALLOWED_ROOTS")
}
