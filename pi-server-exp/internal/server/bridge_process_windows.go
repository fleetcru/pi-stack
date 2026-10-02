//go:build windows

package server

import (
	"strconv"
	"time"

	"golang.org/x/sys/windows"
)

// bridgeProcessStart returns a kernel creation identity, not a client-supplied PID timestamp.
func bridgeProcessStart(pid int) (string, time.Time, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", time.Time{}, err
	}
	defer windows.CloseHandle(handle)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
		return "", time.Time{}, err
	}
	ns := created.Nanoseconds()
	return strconv.FormatInt(ns, 10), time.Unix(0, ns), nil
}

func bridgeProcessExited(pid int, identity string) bool {
	if pid <= 0 || identity == "" {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// A nonexistent PID is evidence of exit. Access denied is not.
		return err == windows.ERROR_INVALID_PARAMETER
	}
	defer windows.CloseHandle(handle)
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(handle, &created, &exited, &kernel, &user) != nil {
		return false
	}
	if strconv.FormatInt(created.Nanoseconds(), 10) != identity {
		return true
	}
	var exitCode uint32
	return windows.GetExitCodeProcess(handle, &exitCode) == nil && exitCode != 259
}
