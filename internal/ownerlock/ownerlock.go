// Package ownerlock holds the per-run owner lock shared by the supervisor
// and the control intents that write a run's attempt state.
package ownerlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// File is the run's owner lock inside its run directory.
const File = "owner.lock"

// ErrHeld reports that another live process owns the run (exit 6).
var ErrHeld = errors.New("the run is owned by another live process")

// Acquire takes the run's exclusive owner lock (design §3): flock on
// Unix, LockFileEx on Windows. Only the holder ingests the run's spool into
// the journal. The operating system releases the lock when its holder dies,
// so it cannot go stale while the holder lives and a live holder is never
// pre-empted. A lock held by any other open file, in this process or
// another, yields ErrHeld. release is idempotent.
func Acquire(runDir string) (release func(), err error) {
	f, err := os.OpenFile(filepath.Join(runDir, File), os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // G304: inside the run directory
	if err != nil {
		return nil, fmt.Errorf("opening the owner lock: %w", err)
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		if errors.Is(err, errLockHeld) {
			return nil, fmt.Errorf("%w: %s", ErrHeld, runDir)
		}
		return nil, fmt.Errorf("taking the owner lock: %w", err)
	}
	return sync.OnceFunc(func() {
		_ = unlockFile(f)
		_ = f.Close()
	}), nil
}

var errLockHeld = errors.New("lock held")
