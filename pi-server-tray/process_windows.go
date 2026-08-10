//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x08000000, // CREATE_NO_WINDOW
		HideWindow:    true,
	}
}

func stopProcess(process *os.Process) error {
	// Windows does not support os.Interrupt for arbitrary child processes.
	// Kill is reliable and prevents a hidden server from surviving tray exit.
	return process.Kill()
}
