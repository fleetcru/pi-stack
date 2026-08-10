//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func stopProcess(process *os.Process) error {
	// Signal the process group so pi-server and any descendants receive SIGTERM.
	return syscall.Kill(-process.Pid, syscall.SIGTERM)
}
