//go:build !windows

package main

import _ "embed"

// macOS and Linux accept PNG tray icon data.
//
//go:embed assets/icon.png
var trayIcon []byte
