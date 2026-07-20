package server

import (
	"log/slog"
	"net/http/httptest"
	"os"
	"testing"
)

func TestDirectoryBrowserRespectsRoots(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(root+"/child", 0755); err != nil {
		t.Fatal(err)
	}
	s := New(Config{CWD: root, DataDir: t.TempDir(), AllowedRoots: []string{root}}, slog.Default())
	w := httptest.NewRecorder()
	s.listDirectories(w, httptest.NewRequest("GET", "/v1/directories", nil))
	if w.Code != 200 {
		t.Fatalf("roots status %d", w.Code)
	}
	w = httptest.NewRecorder()
	s.listDirectories(w, httptest.NewRequest("GET", "/v1/directories?path="+root, nil))
	if w.Code != 200 {
		t.Fatalf("browse status %d", w.Code)
	}
	w = httptest.NewRecorder()
	s.listDirectories(w, httptest.NewRequest("GET", "/v1/directories?path=/", nil))
	if w.Code != 403 {
		t.Fatalf("outside root status %d", w.Code)
	}
}
