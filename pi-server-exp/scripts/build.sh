#!/usr/bin/env bash
set -euo pipefail

# Always build from the pi-server repository root, even when invoked from a
# different folder after the repository has been moved.
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

mkdir -p dist
GOOS=linux GOARCH=amd64 go build -o dist/pi-server-linux-amd64 ./cmd/pi-server
GOOS=windows GOARCH=amd64 go build -o dist/pi-server-windows-amd64.exe ./cmd/pi-server
GOOS=darwin GOARCH=arm64 go build -o dist/pi-server-darwin-arm64 ./cmd/pi-server
