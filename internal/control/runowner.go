package control

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/turbokast/mythhelm/internal/ownerlock"
	"github.com/turbokast/mythhelm/internal/workers"
)

// acquireRunOwner takes the run's owner lock before stop or recover writes
// attempt state (v2 §6.4: acquire ownership, then act; N2, I18, I23). A
// live owner, such as a legacy pipeline that ingests the run's spool,
// refuses the intent with ownership_unresolved before anything is written:
// the refusal is retryable once the owner is gone. A run with no directory
// cannot be owned, so it fails closed the same way rather than reasoning
// over absent evidence. The run id is validated before it is used as a path
// component, and a malformed one is invalid_contract.
func acquireRunOwner(stateDir, runID, intent string) (release func(), err error) {
	if !workers.ValidID(runID) {
		return nil, newError(CodeInvalidContract, "%s needs a valid run id", intent)
	}
	release, err = ownerlock.Acquire(filepath.Join(stateDir, "runs", runID))
	switch {
	case errors.Is(err, ownerlock.ErrHeld):
		return nil, newError(CodeOwnershipUnresolved, "run %q is owned by a live supervisor; %s refused", runID, intent)
	case errors.Is(err, os.ErrNotExist):
		return nil, newError(CodeOwnershipUnresolved, "run %q has no run directory to own; %s refused", runID, intent)
	case err != nil:
		return nil, err
	}
	return release, nil
}
