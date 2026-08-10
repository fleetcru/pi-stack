//go:build darwin

package main

import "os/exec"

func openTarget(target string) error {
	return exec.Command("open", target).Start()
}
