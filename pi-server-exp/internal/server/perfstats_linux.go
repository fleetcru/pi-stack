//go:build linux

package server

import (
	"os"
	"strconv"
	"strings"
)

// readProcessStats reads cumulative CPU time (utime+stime, seconds) and
// resident set size (bytes) from /proc. ok is false when the process has
// exited or the fields cannot be parsed.
func readProcessStats(pid int) (cpuSeconds float64, rssBytes int64, identity uint64, ok bool) {
	if pid <= 0 {
		return 0, 0, 0, false
	}
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, 0, 0, false
	}
	// The comm field may contain spaces and parentheses; fields resume after
	// the final closing parenthesis.
	index := strings.LastIndexByte(string(b), ')')
	if index < 0 || index+2 > len(b) {
		return 0, 0, 0, false
	}
	fields := strings.Fields(string(b)[index+2:])
	// After comm, field 1 is state; utime is field 12 and stime field 13 in
	// this slice (overall fields 14 and 15).
	if len(fields) < 22 {
		return 0, 0, 0, false
	}
	utime, err1 := strconv.ParseFloat(fields[11], 64)
	stime, err2 := strconv.ParseFloat(fields[12], 64)
	startTicks, err3 := strconv.ParseUint(fields[19], 10, 64)
	rssPages, err4 := strconv.ParseInt(fields[21], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return 0, 0, 0, false
	}
	ticks := 100.0 // USER_HZ on Linux
	cpuSeconds = (utime + stime) / ticks
	rssBytes = rssPages * int64(os.Getpagesize())
	return cpuSeconds, rssBytes, startTicks, true
}
