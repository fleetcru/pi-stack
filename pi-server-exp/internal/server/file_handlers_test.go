package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileTreeHonorsBoundedLimit(t *testing.T) {
	cwd := t.TempDir()
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(filepath.Join(cwd, fmt.Sprintf("file-%d.txt", i)), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s := New(Config{RequestTimeout: time.Second, DataDir: t.TempDir(), AllowedRoots: []string{cwd}}, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/v1/files/tree?cwd="+url.QueryEscape(cwd)+"&limit=2", nil)
	rec := httptest.NewRecorder()
	s.fileTree(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"limit":2`) {
		t.Fatalf("response does not report requested limit: %s", rec.Body.String())
	}
	if strings.Count(rec.Body.String(), `"name":`) != 2 {
		t.Fatalf("expected 2 files: %s", rec.Body.String())
	}
}

func TestFileTreeRejectsInvalidLimit(t *testing.T) {
	cwd := t.TempDir()
	s := New(Config{RequestTimeout: time.Second, DataDir: t.TempDir(), AllowedRoots: []string{cwd}}, testLogger())

	for _, limit := range []string{"0", "2001", "invalid"} {
		t.Run(limit, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/files/tree?cwd="+url.QueryEscape(cwd)+"&limit="+limit, nil)
			rec := httptest.NewRecorder()
			s.fileTree(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}
