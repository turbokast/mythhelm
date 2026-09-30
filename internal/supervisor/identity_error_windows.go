//go:build windows

package supervisor

import (
	"errors"
	"syscall"
)

// Windows can deny a read of worker.json briefly while the worker replaces
// it atomically. Keep polling that exact sharing/lock failure; other read
// failures still prevent accepting an unverified worker.
func retryIdentityRead(err error) bool {
	return errors.Is(err, syscall.Errno(32)) || errors.Is(err, syscall.Errno(33))
}
