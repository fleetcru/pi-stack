//go:build windows

package server

import (
	"os/exec"
	"syscall"
)

// applyProcessAttrs hides console windows for child processes on Windows.
// Without this, each Pi process and git command pops up a CMD window.
func applyProcessAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
