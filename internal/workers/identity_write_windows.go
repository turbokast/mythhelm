//go:build windows

package workers

import (
	"errors"
	"syscall"
)

// Windows denies renaming over worker.json while the supervisor holds it
// open for its identity poll. Retry that exact sharing/lock failure; other
// rename failures are permanent.
func retryIdentityWrite(err error) bool {
	return errors.Is(err, syscall.Errno(32)) || errors.Is(err, syscall.Errno(33))
}
