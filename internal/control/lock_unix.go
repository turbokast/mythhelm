//go:build unix

package control

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// lockFile takes a non-blocking exclusive flock, following the ownerlock.go
// precedent. Separate opens in the same process conflict, so a second
// AcquireInstance in this process observes errLockHeld.
func lockFile(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return errLockHeld
	}
	return err
}

func unlockFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}

// userKey distinguishes this user's lock from other users' under a shared
// base dir.
func userKey() string {
	return "uid" + strconv.Itoa(os.Getuid())
}

// checkDirOwner refuses a lock directory owned by another user.
func checkDirOwner(dir string, fi os.FileInfo) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("control: cannot check owner of instance lock directory %s", dir)
	}
	if int(st.Uid) != os.Getuid() {
		return fmt.Errorf("control: instance lock directory %s is owned by another user", dir)
	}
	return nil
}

// sameRoot compares canonical roots. Unix paths compare byte-wise.
func sameRoot(a, b string) bool {
	return a == b
}
