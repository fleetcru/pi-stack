//go:build !windows

package server

import (
	"os"
	"syscall"
)

// pidAlive reports whether a process with the given PID is currently running.
// Used to detect stale history lock files left behind by a crashed server:
// a lock whose owner PID is dead can be safely reclaimed.
func pidAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 performs a liveness check without delivering a signal.
	return proc.Signal(syscall.Signal(0)) == nil
}
