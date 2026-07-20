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
	if len(s.cfg.AllowedRoots) == 0 {
		return cwd, nil
	}
	for _, root := range s.cfg.AllowedRoots {
		abs, err := filepath.Abs(strings.TrimSpace(root))
		if err != nil {
			continue
		}
		// Resolve symlinks on the root too
		abs, err = filepath.EvalSymlinks(abs)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(abs, cwd)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return cwd, nil
		}
	}
	return "", fmt.Errorf("cwd is outside PI_SERVER_ALLOWED_ROOTS")
}
