//go:build windows

package workers

import (
	"os"
	"syscall"
	"testing"
)

func TestBeatReadTransientWindowsSharingOnly(t *testing.T) {
	t.Parallel()
	for _, code := range []syscall.Errno{32, 33} {
		if !beatReadTransient(&os.PathError{Op: "open", Path: "supervisor.beat", Err: code}) {
			t.Fatalf("Windows error %d must be retried", code)
		}
	}
	for _, code := range []syscall.Errno{2, 5} {
		if beatReadTransient(&os.PathError{Op: "open", Path: "supervisor.beat", Err: code}) {
			t.Fatalf("Windows error %d must not be retried", code)
		}
	}
}
