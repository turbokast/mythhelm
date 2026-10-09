package control

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// lockSentinelOffset is the file offset of the one-byte sentinel range
// [lockSentinelOffset, lockSentinelOffset+1) carrying the LockFileEx
// exclusion lock. LockFileEx denies reads overlapping the locked range to
// other handles, so the lock must stay disjoint from the metadata bytes at
// [0, ...): locking byte 0 made every metadata read fail while a holder
// held the lock, and classifyHeld misclassified every refusal as
// ErrInstanceHeld on Windows.
const lockSentinelOffset = 1 << 30

// lockFile takes a non-blocking exclusive LockFileEx byte-range lock,
// following the ownerlock.go precedent. The _windows.go filename suffix
// constrains this file to Windows, as in internal/supervisor.
func lockFile(f *os.File) error {
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &windows.Overlapped{Offset: lockSentinelOffset})
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return errLockHeld
	}
	return err
}

func unlockFile(f *os.File) error {
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &windows.Overlapped{Offset: lockSentinelOffset})
}

// userKey distinguishes this user's lock from other users' under a shared
// base dir. %TEMP% is already per-user; the sanitised logon name keeps the
// path explicit about that.
func userKey() string {
	name := os.Getenv("USERNAME")
	if name == "" {
		return "user"
	}
	return "user" + sanitize(name)
}

func sanitize(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "user"
	}
	return b.String()
}

// checkDirOwner is a no-op on Windows: the lock lives under the per-user
// temp dir, whose ACLs already restrict it to the owning user.
func checkDirOwner(_ string, _ os.FileInfo) error {
	return nil
}

// sameRoot compares canonical roots case-insensitively: Windows paths are
// case-insensitive and EvalSymlinks may normalise case.
func sameRoot(a, b string) bool {
	return strings.EqualFold(a, b)
}
