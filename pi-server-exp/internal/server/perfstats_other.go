//go:build !windows && !linux

package server

// readProcessStats is unavailable on platforms without a supported
// /proc-style interface. Callers surface samples without CPU/RSS rather than
// failing the endpoint.
func readProcessStats(pid int) (cpuSeconds float64, rssBytes int64, identity uint64, ok bool) {
	return 0, 0, 0, false
}
