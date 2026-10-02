package server

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// A random on-disk proof binds registration to a process on this machine.
// A PID reported by a remote bridge is never enough to authorize takeover.
type bridgeProcessProof struct {
	BridgeID    string `json:"bridgeId"`
	PID         int    `json:"pid"`
	StartedAt   int64  `json:"startedAt"`
	SessionPath string `json:"sessionPath"`
}

func isLocalBridgeRequest(request *http.Request) bool {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return false
	}
	peer := net.ParseIP(host)
	if peer == nil {
		return false
	}
	if peer.IsLoopback() {
		return true
	}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err == nil && ip.Equal(peer) {
			return true
		}
	}
	return false
}

func localBridgeProcess(proof bridgeProcessProof) (int, string) {
	if !validSessionID(proof.BridgeID) || len(proof.BridgeID) > 80 || proof.PID <= 0 || proof.StartedAt <= 0 || proof.SessionPath == "" {
		return 0, ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, ""
	}
	path := filepath.Join(home, ".pi", "agent", "bridge-processes", proof.BridgeID+".json")
	file, err := os.Open(path)
	if err != nil {
		return 0, ""
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return 0, ""
	}
	var stored bridgeProcessProof
	if json.NewDecoder(file).Decode(&stored) != nil || stored != proof {
		return 0, ""
	}
	identity, started, err := bridgeProcessStart(proof.PID)
	if err != nil || time.Since(started) < -5*time.Second {
		return 0, ""
	}
	delta := started.Sub(time.UnixMilli(proof.StartedAt))
	if delta < -5*time.Second || delta > 5*time.Second {
		return 0, ""
	}
	return proof.PID, identity
}

func externalContinueBlockedReason(session *ExternalSession) string {
	switch {
	case session == nil:
		return "Session is unavailable"
	case session.transferring:
		return "Session ownership transfer is in progress"
	case session.RelayConnected:
		return "Quit the Pi TUI before continuing on the server"
	case session.bridgePID <= 0 || session.bridgeStartedAt == "":
		return "Restart the TUI with the updated bridge on this server's machine first"
	case len(session.commands) != 0:
		return "Pending relay commands must be resolved before continuing"
	case !bridgeProcessExited(session.bridgePID, session.bridgeStartedAt):
		return "The Pi TUI is still running, or its exit cannot be verified"
	default:
		return ""
	}
}
