//go:build unix

package workers

import (
	"os"

	"golang.org/x/sys/unix"
)

// openHash opens path for hashing without blocking on FIFOs: O_NONBLOCK lets
// the open succeed so hashFile's fstat can reject the non-regular handle.
// Regular files read identically under O_NONBLOCK.
func openHash(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
