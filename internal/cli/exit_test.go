package cli

import (
	"fmt"
	"testing"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

func TestExitCodeControlHeldIsOwnership(t *testing.T) {
	t.Parallel()
	for name, err := range map[string]error{
		"root conflict": control.ErrRootConflict,
		"instance held": control.ErrInstanceHeld,
	} {
		if got := exitCode(err); got != ExitOwnership {
			t.Errorf("exitCode(%s) = %d, want %d", name, got, ExitOwnership)
		}
		if got := exitCode(fmt.Errorf("supervisor: %w", err)); got != ExitOwnership {
			t.Errorf("exitCode(wrapped %s) = %d, want %d", name, got, ExitOwnership)
		}
	}
}

func TestRunExitClassifiedFailures(t *testing.T) {
	t.Parallel()
	for reason, want := range map[string]ExitCode{
		"native_failed":       ExitNative,
		"protocol_error":      ExitNative,
		"provider_limit":      ExitNative,
		"verification_failed": ExitVerify,
	} {
		code, _ := runExit(supervisor.Outcome{State: supervisor.RunFailed, Reason: reason})
		if code != want {
			t.Fatalf("runExit(failed/%s) = %d, want %d", reason, code, want)
		}
	}
	for _, reason := range []string{"billing_route_mismatch", "native_auth_or_billing", "active_run_exists"} {
		code, category := runExit(supervisor.Outcome{State: supervisor.RunBlocked, Reason: reason})
		if code != ExitBlocked || category != "admission_blocked" {
			t.Fatalf("runExit(blocked/%s) = %d/%s, want 3/admission_blocked", reason, code, category)
		}
	}
}
