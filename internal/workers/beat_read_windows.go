//go:build windows

package workers

import (
	"errors"
	"syscall"
)

// Opening supervisor.beat while the supervisor replaces it fails with
// ERROR_SHARING_VIOLATION (32) or ERROR_LOCK_VIOLATION (33). Retry those
// only: unlike the rename side, ERROR_ACCESS_DENIED (5) on an open is a
// genuine ACL denial and reads stale at once.
func beatReadTransient(err error) bool {
	return errors.Is(err, syscall.Errno(32)) || errors.Is(err, syscall.Errno(33))
}
