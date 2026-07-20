package server

import (
	"os"
	"path/filepath"
	"strings"
)

func splitSessionPath(path string) (id, rest string) {
	tail := strings.TrimPrefix(path, "/v1/sessions/")
	parts := strings.SplitN(tail, "/", 2)
	id = parts[0]
	if len(parts) == 2 {
		rest = parts[1]
	}
	return id, rest
}

func validSessionID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func normalizeCWD(requested, fallback string) (string, error) {
	cwd := requested
	if cwd == "" {
		cwd = fallback
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", os.ErrInvalid
	}
	return abs, nil
}
