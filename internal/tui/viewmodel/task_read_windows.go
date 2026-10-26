//go:build windows

package viewmodel

import (
	"errors"

	"golang.org/x/sys/windows"
)

// transientReadErr reports a sharing or lock violation: another handle
// denied the read for now. Access denied and every other error are final.
func transientReadErr(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
