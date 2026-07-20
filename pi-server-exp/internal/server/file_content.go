package server

import (
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxFileContentBytes = 1 << 20 // 1 MiB

var imageMIME = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
	".svg":  "image/svg+xml",
}

func (s *Server) fileContent(w http.ResponseWriter, r *http.Request) {
	s.writeFileContent(w, r, r.URL.Query().Get("path"))
}

func (s *Server) sessionFileContent(w http.ResponseWriter, r *http.Request, sessionID string) {
	// Session-scoped path still enforces allowed roots. Remote sessions are proxied.
	if s.proxyRemoteSession(w, r, sessionID, "files/content") {
		return
	}
	if _, ok := s.sessions.GetSpec(sessionID); !ok {
		writeErrorCode(w, r, http.StatusNotFound, CodeSessionNotFound, "session not found")
		return
	}
	s.writeFileContent(w, r, r.URL.Query().Get("path"))
}

func (s *Server) writeFileContent(w http.ResponseWriter, r *http.Request, requested string) {
	w.Header().Set("Cache-Control", "no-store")
	if requested == "" {
		writeErrorCode(w, r, http.StatusBadRequest, CodeBadRequest, "path required")
		return
	}
	abs, err := filepath.Abs(requested)
	if err != nil {
		writeErrorCode(w, r, http.StatusBadRequest, CodeBadRequest, "invalid path")
		return
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// If the file does not exist, EvalSymlinks fails; still reject outside roots via parent checks.
		if !os.IsNotExist(err) {
			writeErrorCode(w, r, http.StatusBadRequest, CodeBadRequest, "cannot resolve path")
			return
		}
		// Validate parent is allowed, then report not found without leaking.
		parent := filepath.Dir(abs)
		if _, err := s.allowedCWD(parent); err != nil {
			writeErrorCode(w, r, http.StatusForbidden, CodePathNotAllowed, "path is outside allowed roots")
			return
		}
		writeErrorCode(w, r, http.StatusNotFound, CodeNotFound, "file not found")
		return
	}
	// Enforce allowed roots against resolved path using the same relative-check as CWD,
	// but for files: allowed if the file lives under an allowed root directory.
	if err := s.allowedFilePath(resolved); err != nil {
		writeErrorCode(w, r, http.StatusForbidden, CodePathNotAllowed, "path is outside allowed roots")
		return
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		writeErrorCode(w, r, http.StatusNotFound, CodeNotFound, "file not found")
		return
	}
	if info.IsDir() {
		writeErrorCode(w, r, http.StatusBadRequest, CodeNotAFile, "path is a directory")
		return
	}
	if !info.Mode().IsRegular() {
		writeErrorCode(w, r, http.StatusBadRequest, CodeNotAFile, "path is not a regular file")
		return
	}
	f, err := os.Open(resolved)
	if err != nil {
		writeErrorCode(w, r, http.StatusBadRequest, CodeBadRequest, "cannot open file")
		return
	}
	defer f.Close()

	limit := maxFileContentBytes
	buf := make([]byte, limit+1)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		writeErrorCode(w, r, http.StatusInternalServerError, CodeInternal, "failed to read file")
		return
	}
	truncated := n > limit
	if truncated {
		n = limit
	}
	data := buf[:n]
	size := info.Size()

	if isBinarySample(data) {
		ext := strings.ToLower(filepath.Ext(resolved))
		if mime, ok := imageMIME[ext]; ok && size <= maxFileContentBytes && !truncated {
			writeJSON(w, http.StatusOK, map[string]any{
				"path":      resolved,
				"mimeType":  mime,
				"encoding":  "base64",
				"binary":    true,
				"truncated": false,
				"content":   base64.StdEncoding.EncodeToString(data),
				"size":      size,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"path":      resolved,
			"mimeType":  "application/octet-stream",
			"binary":    true,
			"encoding":  "",
			"truncated": truncated,
			"size":      size,
		})
		return
	}

	if !utf8.Valid(data) {
		writeJSON(w, http.StatusOK, map[string]any{
			"path":      resolved,
			"mimeType":  "application/octet-stream",
			"binary":    true,
			"encoding":  "",
			"truncated": truncated,
			"size":      size,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"path":      resolved,
		"mimeType":  "text/plain; charset=utf-8",
		"encoding":  "utf-8",
		"binary":    false,
		"truncated": truncated,
		"content":   string(data),
		"size":      size,
	})
}

func (s *Server) allowedFilePath(path string) error {
	// Reuse allowedCWD by validating the containing directory exists and is allowed.
	// For a file, check parent directory against roots, and that the file path is under a root.
	if len(s.cfg.AllowedRoots) == 0 {
		return nil
	}
	for _, root := range s.cfg.AllowedRoots {
		abs, err := filepath.Abs(strings.TrimSpace(root))
		if err != nil {
			continue
		}
		abs, err = filepath.EvalSymlinks(abs)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(abs, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
	}
	return os.ErrPermission
}

func isBinarySample(data []byte) bool {
	// NUL byte in sample indicates binary.
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}
