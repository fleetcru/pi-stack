//go:build !windows

package server

import "os/exec"

func applyProcessAttrs(_ *exec.Cmd) {}
