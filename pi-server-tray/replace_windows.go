//go:build windows

package main

import (
	"errors"
	"os"
)

func replaceFile(source, destination string) error {
	backup := destination + ".previous"
	_ = os.Remove(backup)
	hadDestination := true
	if err := os.Rename(destination, backup); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		hadDestination = false
	}
	if err := os.Rename(source, destination); err != nil {
		if hadDestination {
			_ = os.Rename(backup, destination)
		}
		return err
	}
	if hadDestination {
		_ = os.Remove(backup)
	}
	return nil
}
