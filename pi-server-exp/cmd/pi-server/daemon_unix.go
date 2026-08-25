//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// daemonize re-launches the current process detached from the terminal.
// On Unix it sets a new session so the child is not a foreground process
// group member. The child inherits the same command-line arguments minus --bg.
func daemonize() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("cannot resolve executable path: %w", err)
	}

	args := filteredArgs(os.Args[1:])

	logPath := stderrLogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := exec.Command(exe, args...)
	cmd.Dir = "."
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // new session, detach from terminal
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "pi-server started in background (pid %d)\n", cmd.Process.Pid)
	return nil
}
