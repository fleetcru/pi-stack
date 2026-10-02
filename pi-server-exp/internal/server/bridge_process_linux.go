//go:build linux

package server

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func readBridgeProcess(pid int) (identity, state string, err error) {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", "", err
	}
	// comm can contain spaces and parentheses. Fields after its last ')' start at field 3.
	end := strings.LastIndexByte(string(raw), ')')
	if end < 0 {
		return "", "", os.ErrInvalid
	}
	fields := strings.Fields(string(raw)[end+1:])
	if len(fields) < 20 {
		return "", "", os.ErrInvalid
	}
	return fields[19], fields[0], nil
}
func bridgeProcessStart(pid int) (string, time.Time, error) {
	identity, _, err := readBridgeProcess(pid)
	if err != nil {
		return "", time.Time{}, err
	}
	ticks, err := strconv.ParseFloat(identity, 64)
	if err != nil {
		return "", time.Time{}, err
	}
	raw, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return "", time.Time{}, err
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return "", time.Time{}, os.ErrInvalid
	}
	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "", time.Time{}, err
	}
	// Linux procfs exposes starttime in USER_HZ units (100 per second).
	started := time.Now().Add(time.Duration((ticks/100 - uptime) * float64(time.Second)))
	return identity, started, nil
}
func bridgeProcessExited(pid int, identity string) bool {
	if pid <= 0 || identity == "" {
		return false
	}
	current, state, err := readBridgeProcess(pid)
	if err != nil {
		return os.IsNotExist(err)
	}
	return current != identity || state == "Z" || state == "X"
}
