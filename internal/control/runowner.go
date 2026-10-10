package control

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/turbokast/mythhelm/internal/ownerlock"
)

// acquireRunOwner takes the run's owner lock before stop or recover writes
// attempt state (v2 §6.4: acquire ownership, then act; N2, I18, I23). A
// live owner, such as a legacy pipeline that ingests the run's spool,
// refuses the intent with ownership_unresolved before anything is written:
// the refusal is retryable once the owner is gone. A run with no directory
// has no owner to hold it; the caller reports the unknown run.
func acquireRunOwner(stateDir, runID, intent string) (release func(), err error) {
	release, err = ownerlock.Acquire(filepath.Join(stateDir, "runs", runID))
	switch {
	case errors.Is(err, ownerlock.ErrHeld):
		return nil, newError(CodeOwnershipUnresolved, "run %q is owned by a live supervisor; %s refused", runID, intent)
	case errors.Is(err, os.ErrNotExist):
		return func() {}, nil
	case err != nil:
		return nil, err
	}
	return release, nil
}
