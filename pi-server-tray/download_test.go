package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	downloads := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/fleetcru/pi-stack/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"server-v1.2.3","assets":[{"name":"pi-server-windows-amd64.exe","browser_download_url":%q,"size":%d}]}`,
				server.URL+"/download/server.exe", len(binary))
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

func TestEnsureLatestRejectsMissingPlatformAsset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"server-v1.2.3","assets":[]}`))
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
