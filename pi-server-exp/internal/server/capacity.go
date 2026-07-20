package server

import "fmt"

func (s *Server) ensureSessionCapacity(p *PiProcess) error {
	status := p.Status()
	if running, _ := status["running"].(bool); running {
		return nil
	}
	// checkCanStart atomically counts running processes under the registry
	// read lock, eliminating the TOCTOU race between ActiveCount() and the
	// subsequent p.Start() that auto-starts the idle process.
	if !s.sessions.checkCanStart() {
		return fmt.Errorf("max active Pi sessions reached")
	}
	return nil
}
