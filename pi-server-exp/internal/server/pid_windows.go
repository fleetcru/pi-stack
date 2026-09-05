//go:build windows

package server

import (
	"golang.org/x/sys/windows"
)

// pidAlive reports whether a process with the given PID is currently running.
// Used to detect stale history lock files left behind by a crashed server:
// a lock whose owner PID is dead can be safely reclaimed.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// Query-limited access is enough to detect liveness and works across
	// privilege boundaries, unlike full access which can fail with
	// ERROR_ACCESS_DENIED for elevated processes.
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// The process may exist but be inaccessible; ERROR_ACCESS_DENIED
		// still proves it is alive.
		if err == windows.ERROR_ACCESS_DENIED {
			return true
		}
		return false
	}
	defer windows.CloseHandle(handle)
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	// STILL_ACTIVE (259) means the process has not exited yet.
	return code == 259
}
