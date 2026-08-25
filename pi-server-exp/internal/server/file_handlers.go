package server

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

const (
	defaultFileTreeLimit = 300
	maxFileTreeLimit     = 2000
)

type FileInfo struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	cwd, err := s.allowedCWD(r.URL.Query().Get("cwd"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	entries, err := os.ReadDir(cwd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	files := []FileInfo{}
	for _, e := range entries {
		info, _ := e.Info()
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		files = append(files, FileInfo{Name: e.Name(), Path: filepath.Join(cwd, e.Name()), IsDir: e.IsDir(), Size: size})
	}
	writeJSON(w, http.StatusOK, map[string]any{"cwd": cwd, "files": files})
}

func (s *Server) fileTree(w http.ResponseWriter, r *http.Request) {
	cwd, err := s.allowedCWD(r.URL.Query().Get("cwd"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	limit := defaultFileTreeLimit
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, parseErr := strconv.Atoi(rawLimit)
		if parseErr != nil || parsed < 1 || parsed > maxFileTreeLimit {
			writeErrorText(w, http.StatusBadRequest, "limit must be an integer between 1 and 2000")
			return
		}
		limit = parsed
	}
	files := []FileInfo{}
	stop := errors.New("file limit reached")
	err = filepath.WalkDir(cwd, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if len(files) >= limit {
			return stop
		}
		if path == cwd {
			return nil
		}
		info, _ := d.Info()
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		files = append(files, FileInfo{Name: d.Name(), Path: path, IsDir: d.IsDir(), Size: size})
		return nil
	})
	if err != nil && !errors.Is(err, stop) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cwd": cwd, "limit": limit, "files": files})
}
