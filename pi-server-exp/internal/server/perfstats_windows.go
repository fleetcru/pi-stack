//go:build windows

package server

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var k32GetProcessMemoryInfo = windows.NewLazySystemDLL("kernel32.dll").NewProc("K32GetProcessMemoryInfo")

// processMemoryCounters mirrors PROCESS_MEMORY_COUNTERS (Win32). Field layout
// matches the C struct on both amd64 and arm64: two DWORDs followed by
// SIZE_T-sized counters, with implicit padding after PageFaultCount.
type processMemoryCounters struct {
	CB                      uint32
	PageFaultCount          uint32
	PeakWorkingSetSize      uintptr
	WorkingSetSize          uintptr
	QuotaPeakPagedPoolUsage uintptr
	QuotaPagedPoolUsage     uintptr
	QuotaPeakNonPagedUsage  uintptr
	QuotaNonPagedPoolUsage  uintptr
	PagefileUsage           uintptr
	PeakPagefileUsage       uintptr
}

// readProcessStats returns cumulative CPU time (user+kernel, seconds) and
// resident set size in bytes for the given PID. ok is false when the process
// cannot be opened or queried; callers treat that as "unavailable".
func readProcessStats(pid int) (cpuSeconds float64, rssBytes int64, identity uint64, ok bool) {
	if pid <= 0 {
		return 0, 0, 0, false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return 0, 0, 0, false
	}
	defer windows.CloseHandle(handle)

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return 0, 0, 0, false
	}
	cpuSeconds = filetimeSeconds(kernel) + filetimeSeconds(user)
	identity = filetimeTicks(creation)

	var counters processMemoryCounters
	counters.CB = uint32(unsafe.Sizeof(counters))
	r1, _, _ := k32GetProcessMemoryInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&counters)), uintptr(counters.CB))
	if r1 == 0 {
		return 0, 0, 0, false
	}
	return cpuSeconds, int64(counters.WorkingSetSize), identity, true
}

func filetimeTicks(t windows.Filetime) uint64 {
	return uint64(t.HighDateTime)<<32 | uint64(t.LowDateTime)
}

func filetimeSeconds(t windows.Filetime) float64 {
	// FILETIME stores 100-nanosecond intervals since 1601.
	return float64(filetimeTicks(t)) * 1e-7
}
