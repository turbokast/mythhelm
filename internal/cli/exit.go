package cli

import (
	"errors"
	"flag"
	"fmt"
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

// exitCode is the single place that maps a command's error to its exit code.
func exitCode(err error) ExitCode {
	var usage usageError
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
		return ExitOK
	case errors.As(err, &usage):
		return ExitInvalid
	default:
		return ExitInternal
	}
}
