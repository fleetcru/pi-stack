package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	defaultReleaseRepo = "fleetcru/pi-stack"
	maxServerSize      = 100 << 20 // 100 MiB
)

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type githubRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type serverDownloader struct {
	client  *http.Client
	apiBase string
	repo    string
	goos    string
	goarch  string
}

func newServerDownloader(repo string) *serverDownloader {
	return &serverDownloader{
		client:  &http.Client{Timeout: 2 * time.Minute},
		apiBase: "https://api.github.com",
		repo:    repo,
		goos:    runtime.GOOS,
		goarch:  runtime.GOARCH,
	}
}

func managedServerPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	name := "pi-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(home, ".pi", "server", "bin", name), nil
}

func cachedServerVersion(destination string) (string, error) {
	version, err := os.ReadFile(destination + ".version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(version)), nil
}

func (d *serverDownloader) ensureLatest(ctx context.Context, destination string) (string, error) {
	release, err := d.latestRelease(ctx)
	if err != nil {
		return "", err
	}
	assetName, err := serverAssetName(d.goos, d.goarch)
	if err != nil {
		return "", err
	}
	asset, err := findAsset(release.Assets, assetName)
	if err != nil {
		return "", fmt.Errorf("release %s: %w", release.TagName, err)
	}
	versionPath := destination + ".version"
	if fileExists(destination) {
		if version, err := cachedServerVersion(destination); err == nil && version == release.TagName {
			return release.TagName, nil
		}
	}
	if err := d.downloadAsset(ctx, asset, destination); err != nil {
		return "", err
	}
	if err := atomicWriteFile(versionPath, []byte(release.TagName+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("save server version: %w", err)
	}
	return release.TagName, nil
}

func (d *serverDownloader) latestRelease(ctx context.Context) (githubRelease, error) {
	url := strings.TrimRight(d.apiBase, "/") + "/repos/" + d.repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "pi-server-tray")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := d.client.Do(req)
	if err != nil {
		return githubRelease{}, fmt.Errorf("check latest server release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return githubRelease{}, fmt.Errorf("check latest server release: GitHub returned %s", resp.Status)
	}
	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&release); err != nil {
		return githubRelease{}, fmt.Errorf("decode latest server release: %w", err)
	}
	if release.TagName == "" {
		return githubRelease{}, errors.New("latest server release has no tag")
	}
	return release, nil
}

func (d *serverDownloader) downloadAsset(ctx context.Context, asset releaseAsset, destination string) error {
	if asset.BrowserDownloadURL == "" {
		return errors.New("release asset has no download URL")
	}
	if asset.Size > maxServerSize {
		return fmt.Errorf("release asset is too large: %d bytes", asset.Size)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "pi-server-tray")
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", asset.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: server returned %s", asset.Name, resp.Status)
	}
	if resp.ContentLength > maxServerSize {
		return fmt.Errorf("download %s: response is too large", asset.Name)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create server directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".pi-server-download-*")
	if err != nil {
		return fmt.Errorf("create temporary download: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	written, copyErr := io.Copy(tmp, io.LimitReader(resp.Body, maxServerSize+1))
	closeErr := tmp.Close()
	if copyErr != nil {
		return fmt.Errorf("download %s: %w", asset.Name, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("save %s: %w", asset.Name, closeErr)
	}
	if written > maxServerSize {
		return fmt.Errorf("download %s: response exceeded %d bytes", asset.Name, maxServerSize)
	}
	if asset.Size > 0 && written != asset.Size {
		return fmt.Errorf("download %s: received %d bytes, expected %d", asset.Name, written, asset.Size)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("make server executable: %w", err)
	}
	if err := replaceFile(tmpPath, destination); err != nil {
		return fmt.Errorf("install downloaded server: %w", err)
	}
	return nil
}

func serverAssetName(goos, goarch string) (string, error) {
	if goarch != "amd64" {
		return "", fmt.Errorf("server releases do not support %s/%s", goos, goarch)
	}
	switch goos {
	case "windows":
		return "pi-server-windows-amd64.exe", nil
	case "linux":
		return "pi-server-linux-amd64", nil
	default:
		return "", fmt.Errorf("server releases do not support %s/%s", goos, goarch)
	}
}

func findAsset(assets []releaseAsset, name string) (releaseAsset, error) {
	for _, asset := range assets {
		if asset.Name == name {
			return asset, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("asset %q was not found", name)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return err
	}
	return replaceFile(tmpPath, path)
}
