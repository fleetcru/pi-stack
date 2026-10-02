//go:build windows

package server

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestContinuationWithShortWindowsTempPath(t *testing.T) {
	directory := t.TempDir()
	path, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]uint16, 32768)
	length, err := windows.GetShortPathName(path, &buffer[0], uint32(len(buffer)))
	if err != nil || length == 0 || int(length) >= len(buffer) {
		t.Skipf("short paths unavailable: %v", err)
	}
	short := windows.UTF16ToString(buffer[:length])
	if short == directory {
		t.Skip("8.3 short names are disabled for the temporary directory")
	}
	t.Setenv("TMP", short)
	t.Setenv("TEMP", short)
	// Re-run both CI failures with the same short-path TempDir setup.
	t.Run("preserves identity", TestContinueExternalSessionPreservesIdentity)
	t.Run("missing history cleanup", func(t *testing.T) {
		s, stop := continuationFixture(t)
		stop()
		spec, _ := s.sessions.GetSpec("continue-test")
		if err := os.Remove(spec.SessionPath); err != nil {
			t.Fatal(err)
		}
		if response := continueRequest(s); response.Code != 409 {
			t.Fatalf("missing history response: %d %s", response.Code, response.Body.String())
		}
	})
}
