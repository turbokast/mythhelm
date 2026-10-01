//go:build windows

package workers

import (
	"errors"
	"syscall"
)

// Windows denies renaming over worker.json while the supervisor holds it
// open for its identity poll. That conflict surfaces as ERROR_ACCESS_DENIED
// (5), unlike a read conflict (32/33): a rename over an open handle is
// denied even though the file itself is writable. Retry 5 with the
// sharing/lock failures; the bounded budget still fails a genuine ACL
// denial, only 500ms later. Other rename failures are permanent.
func retryIdentityWrite(err error) bool {
	return errors.Is(err, syscall.Errno(5)) || errors.Is(err, syscall.Errno(32)) || errors.Is(err, syscall.Errno(33))
}
