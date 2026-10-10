package supervisor

import "github.com/turbokast/mythhelm/internal/ownerlock"

// OwnerLockFile is the run's owner lock inside its run directory.
const OwnerLockFile = ownerlock.File

// ErrOwnerHeld reports that another live process owns the run (exit 6).
var ErrOwnerHeld = ownerlock.ErrHeld

// AcquireOwner takes the run's exclusive owner lock (design §3); see
// ownerlock.Acquire. Only the holder ingests the run's spool into the
// journal.
func AcquireOwner(runDir string) (release func(), err error) {
	return ownerlock.Acquire(runDir)
}
