package cli

import (
	"errors"
	"flag"
	"fmt"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/security"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workspace"
)

// ExitCode is a process exit status with the §15.10 meaning.
type ExitCode int

const (
	ExitOK         ExitCode = 0   // command or deliverable succeeded
	ExitInternal   ExitCode = 1   // unexpected error (design §11)
	ExitInvalid    ExitCode = 2   // invalid arguments or configuration
	ExitBlocked    ExitCode = 3   // admission, policy or approval blocked
	ExitNative     ExitCode = 4   // native execution failed
	ExitVerify     ExitCode = 5   // verification failed or unavailable
	ExitOwnership  ExitCode = 6   // interrupted, recovery required or ownership unresolved
	ExitCapability ExitCode = 7   // required adapter or platform capability unavailable
	ExitCancelled  ExitCode = 130 // foreground cancellation completed
)

// usageError marks an invalid command line.
type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

func usageErrorf(format string, args ...any) error {
	return usageError{fmt.Errorf(format, args...)}
}

// outcomeError is a command outcome that carries its own exit code and JSON
// error category, such as a run that ended without its deliverable.
type outcomeError struct {
	code     ExitCode
	category string
	err      error
}

func (e *outcomeError) Error() string { return e.err.Error() }
func (e *outcomeError) Unwrap() error { return e.err }

// exitCode is the single place that maps a command's error to its exit code.
func exitCode(err error) ExitCode {
	var usage usageError
	var outcome *outcomeError
	var blocked *admission.BlockedError
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
		return ExitOK
	case errors.As(err, &outcome):
		return outcome.code
	case errors.As(err, &usage), errors.Is(err, journal.ErrSchemaTooNew), errors.Is(err, admission.ErrInvalid),
		errors.Is(err, admission.ErrProjectConfig), errors.Is(err, security.ErrDeniedPassthrough):
		return ExitInvalid
	case errors.As(err, &blocked) && blocked.Capability, errors.Is(err, workspace.ErrGitTooOld):
		return ExitCapability
	case errors.As(err, &blocked):
		return ExitBlocked
	case errors.Is(err, supervisor.ErrOwnerHeld), errors.Is(err, supervisor.ErrOwnership):
		return ExitOwnership
	default:
		return ExitInternal
	}
}

// errorCategory is the JSON error category of err (§15.10, design §10).
func errorCategory(err error) string {
	var outcome *outcomeError
	if errors.As(err, &outcome) {
		return outcome.category
	}
	return categories[exitCode(err)]
}

var categories = map[ExitCode]string{
	ExitOK:         "",
	ExitInternal:   "internal",
	ExitInvalid:    "invalid_arguments",
	ExitBlocked:    "admission_blocked",
	ExitNative:     "native_failed",
	ExitVerify:     "verification_failed",
	ExitOwnership:  "ownership_unresolved",
	ExitCapability: "capability_unavailable",
	ExitCancelled:  "cancelled",
}

// runExit maps where a run stopped to its exit code and error category
// (design §10).
func runExit(o supervisor.Outcome) (ExitCode, string) {
	if o.Detached {
		return ExitOwnership, categories[ExitOwnership]
	}
	switch o.State {
	case supervisor.RunCompleted:
		return ExitOK, ""
	case supervisor.RunReadyForReview:
		if o.Reason == "unverified" {
			return ExitVerify, "verification_unavailable"
		}
		return ExitOK, ""
	case supervisor.RunBlocked:
		return ExitBlocked, categories[ExitBlocked]
	case supervisor.RunCancelled:
		return ExitCancelled, categories[ExitCancelled]
	case supervisor.RunInterrupted, supervisor.RunStopping:
		return ExitOwnership, categories[ExitOwnership]
	case supervisor.RunFailed:
		switch o.Reason {
		case "native_failed", "protocol_error", "recovered_partial":
			return ExitNative, categories[ExitNative]
		case "verification_failed":
			return ExitVerify, "verification_failed"
		case "verification_unavailable":
			return ExitVerify, "verification_unavailable"
		}
	}
	return ExitInternal, categories[ExitInternal]
}
