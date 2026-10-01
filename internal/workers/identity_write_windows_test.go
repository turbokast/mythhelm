//go:build windows

package workers

import (
	"os"
	"syscall"
	"testing"
)

func TestRetryIdentityWriteWindowsSharingOnly(t *testing.T) {
	for _, code := range []syscall.Errno{32, 33} {
		if !retryIdentityWrite(&os.PathError{Op: "rename", Path: "worker.json", Err: code}) {
			t.Fatalf("Windows error %d must be retried", code)
		}
	}
	if retryIdentityWrite(&os.PathError{Op: "rename", Path: "worker.json", Err: syscall.Errno(5)}) {
		t.Fatal("ACL denial must not be retried")
	}
}
