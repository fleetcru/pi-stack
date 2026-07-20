package server

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
)

type DirectoryInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// listDirectories exposes only roots permitted by PI_SERVER_ALLOWED_ROOTS. Without
// that setting, the daemon default cwd is the single browseable root.
func (s *Server) listDirectories(w http.ResponseWriter, r *http.Request) {
	requested := r.URL.Query().Get("path")
	if requested == "" {
		roots := s.directoryRoots()
		writeJSON(w, http.StatusOK, map[string]any{"roots": roots, "directories": roots})
		return
	}
	cwd, err := s.allowedCWD(requested)
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	entries, err := os.ReadDir(cwd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	dirs := make([]DirectoryInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, DirectoryInfo{Name: entry.Name(), Path: filepath.Join(cwd, entry.Name())})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	parent := filepath.Dir(cwd)
	if _, err := s.allowedCWD(parent); err != nil {
		parent = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": cwd, "parent": parent, "directories": dirs})
}

func (s *Server) directoryRoots() []DirectoryInfo {
	roots := s.cfg.AllowedRoots
	if len(roots) == 0 {
		roots = []string{s.cfg.CWD}
	}
	out := make([]DirectoryInfo, 0, len(roots))
	for _, root := range roots {
		if abs, err := filepath.Abs(root); err == nil {
			out = append(out, DirectoryInfo{Name: filepath.Base(abs), Path: abs})
		}
	}
	return out
}
