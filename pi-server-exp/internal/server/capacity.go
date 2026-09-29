package server

import "fmt"

// ensureSessionCapacity verifies that starting p will not exceed MaxSessions.
//
// When the limit is already reached, a fully idle session is stopped to free a
// slot before failing. Evicting the coldest dormant process is a better
// outcome than rejecting new work, because the evicted session relaunches
// transparently on its next prompt. Only processes idle past a grace period are
// eligible, so an in-flight run is never the victim.
func (s *Server) ensureSessionCapacity(p *PiProcess) error {
	status := p.Status()
	if running, _ := status["running"].(bool); running {
		return nil
	}
	// checkCanStart atomically counts running processes under the registry
	// read lock, eliminating the TOCTOU race between ActiveCount() and the
	// subsequent p.Start() that auto-starts the idle process.
	if s.sessions.checkCanStart() {
		return nil
	}
	if s.reaper != nil && s.reaper.reapOldestIdle(s.reaper.timeout) && s.sessions.checkCanStart() {
		return nil
	}
	return fmt.Errorf("max active Pi sessions reached")
}
