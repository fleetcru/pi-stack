package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerAssetName(t *testing.T) {
	tests := []struct {
		goos, goarch string
		want         string
		wantErr      bool
	}{
		{goos: "windows", goarch: "amd64", want: "pi-server-windows-amd64.exe"},
		{goos: "linux", goarch: "amd64", want: "pi-server-linux-amd64"},
		{goos: "darwin", goarch: "amd64", wantErr: true},
		{goos: "windows", goarch: "arm64", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.goos+"-"+tt.goarch, func(t *testing.T) {
			got, err := serverAssetName(tt.goos, tt.goarch)
			if (err != nil) != tt.wantErr {
				t.Fatalf("serverAssetName() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("serverAssetName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnsureLatestDownloadsAndCachesRelease(t *testing.T) {
	const binary = "server-binary"
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(binary)))
	downloads := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/fleetcru/pi-stack/releases":
			fmt.Fprintf(w, `[{"tag_name":"tray-v9.0.0","assets":[]},{"tag_name":"server-v1.2.3","assets":[{"name":"pi-server-windows-amd64.exe","browser_download_url":%q,"size":%d},{"name":"SHA256SUMS","browser_download_url":%q,"size":80}]}]`,
				server.URL+"/download/server.exe", len(binary), server.URL+"/download/SHA256SUMS")
		case "/download/SHA256SUMS":
			fmt.Fprintf(w, "%s  pi-server-windows-amd64.exe\n", checksum)
		case "/download/server.exe":
			downloads++
			_, _ = w.Write([]byte(binary))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	d := &serverDownloader{
		client:  server.Client(),
		apiBase: server.URL,
		repo:    defaultReleaseRepo,
		goos:    "windows",
		goarch:  "amd64",
	}
	destination := filepath.Join(t.TempDir(), "pi-server.exe")
	for i := 0; i < 2; i++ {
		tag, err := d.ensureLatest(context.Background(), destination)
		if err != nil {
			t.Fatalf("ensureLatest() error = %v", err)
		}
		if tag != "server-v1.2.3" {
			t.Fatalf("ensureLatest() tag = %q", tag)
		}
	}
	if downloads != 1 {
		t.Fatalf("asset downloads = %d, want 1", downloads)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != binary {
		t.Fatalf("downloaded content = %q", got)
	}
	version, err := os.ReadFile(destination + ".version")
	if err != nil {
		t.Fatal(err)
	}
	if string(version) != "server-v1.2.3\n" {
		t.Fatalf("cached version = %q", version)
	}
}

func TestExpectedChecksumRejectsMissingEntry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("abc  another-file\n"))
	}))
	defer server.Close()
	d := &serverDownloader{client: server.Client()}
	asset := releaseAsset{Name: "SHA256SUMS", BrowserDownloadURL: server.URL}
	if _, err := d.expectedChecksum(context.Background(), asset, "pi-server-linux-amd64"); err == nil {
		t.Fatal("expectedChecksum accepted a missing entry")
	}
}

func TestDownloadAssetRejectsChecksumMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("server-binary"))
	}))
	defer server.Close()
	d := &serverDownloader{client: server.Client()}
	asset := releaseAsset{Name: "pi-server-linux-amd64", BrowserDownloadURL: server.URL, Size: int64(len("server-binary"))}
	destination := filepath.Join(t.TempDir(), "pi-server")
	if err := d.downloadAsset(context.Background(), asset, destination, strings.Repeat("0", 64)); err == nil {
		t.Fatal("downloadAsset accepted a checksum mismatch")
	}
	if fileExists(destination) {
		t.Fatal("checksum mismatch installed the binary")
	}
}

func TestEnsureLatestRejectsMissingPlatformAsset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"server-v1.2.3","assets":[]}]`))
	}))
	defer server.Close()
	d := &serverDownloader{
		client: server.Client(), apiBase: server.URL, repo: defaultReleaseRepo,
		goos: "linux", goarch: "amd64",
	}
	if _, err := d.ensureLatest(context.Background(), filepath.Join(t.TempDir(), "pi-server")); err == nil {
		t.Fatal("ensureLatest() accepted a release without the platform asset")
	}
}
