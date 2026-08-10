//go:build windows

package main

import _ "embed"

// Windows requires a real multi-resolution .ico resource for notification-area icons.
//
//go:embed assets/icon.ico
var trayIcon []byte
