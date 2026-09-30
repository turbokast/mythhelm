//go:build windows

package supervisor

import (
	"os"
	"syscall"
	"testing"
)

func TestRetryIdentityReadWindowsSharingOnly(t *testing.T) {
	for _, code := range []syscall.Errno{32, 33} {
		if !retryIdentityRead(&os.PathError{Op: "open", Path: "worker.json", Err: code}) {
			t.Fatalf("Windows error %d must be retried", code)
		}
	}
	if retryIdentityRead(&os.PathError{Op: "open", Path: "worker.json", Err: syscall.Errno(5)}) {
		t.Fatal("ACL denial must not be retried")
	}
}
