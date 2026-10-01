//go:build windows

package workers

import (
	"os"
	"syscall"
	"testing"
)

func TestRetryIdentityWriteWindowsSharingOnly(t *testing.T) {
	// Unlike the read path, the write path retries ERROR_ACCESS_DENIED:
	// renaming over an open reader reports 5, observed in CI as
	// "rename ... worker.json: Access is denied." A genuine ACL denial
	// still fails after the bounded budget.
	for _, code := range []syscall.Errno{5, 32, 33} {
		if !retryIdentityWrite(&os.PathError{Op: "rename", Path: "worker.json", Err: code}) {
			t.Fatalf("Windows error %d must be retried", code)
		}
	}
	if retryIdentityWrite(&os.PathError{Op: "rename", Path: "worker.json", Err: syscall.Errno(2)}) {
		t.Fatal("missing path must not be retried")
	}
}
