//go:build !windows

package workers

import (
	"errors"
	"os"
	"testing"
)

func TestRetryIdentityWriteNeverRetriesOffWindows(t *testing.T) {
	t.Parallel()
	sharing := &os.PathError{Op: "rename", Path: "worker.json", Err: errors.New("sharing violation")}
	if retryIdentityWrite(sharing) {
		t.Fatal("off Windows every rename failure must fail fast")
	}
}
