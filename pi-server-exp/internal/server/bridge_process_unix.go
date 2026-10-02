//go:build !windows && !linux

package server

import (
	"errors"
	"time"
)

func bridgeProcessStart(pid int) (string, time.Time, error) {
	return "", time.Time{}, errors.New("local bridge process verification is unsupported on this platform")
}
func bridgeProcessExited(pid int, identity string) bool { return false }
