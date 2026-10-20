package cli

import (
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/supervisor"
)

func TestRunHelpListsThreeProfiles(t *testing.T) {
	code, stdout, stderr := runMain("run", "--help")
	if code != int(ExitOK) {
		t.Fatalf("exit code = %d, want 0 (stderr %q)", code, stderr)
	}
	// The execution-profile flag's own help names all three profiles.
	_, usage, ok := strings.Cut(stdout, "-execution-profile")
	if !ok {
		t.Fatalf("run --help lacks -execution-profile:\n%s", stdout)
	}
	flagHelp, _, _ := strings.Cut(usage, "\n  -")
	for _, profile := range []string{"trusted-host", "restricted", "inspect"} {
		if !strings.Contains(flagHelp, profile) {
			t.Errorf("-execution-profile help does not name %s:\n%s", profile, flagHelp)
		}
	}
}

func TestDetachedOutcomeDescription(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name        string
		out         supervisor.Outcome
		description string
		code        ExitCode
	}{
		{"running", supervisor.Outcome{RunID: "run_synthetic", State: supervisor.RunExecuting, DetachedRunning: true}, "detached from run run_synthetic; run is not complete; worker keeps running; reattach with: mythhelm recover run_synthetic", ExitOwnership},
		{"stopping", supervisor.Outcome{RunID: "run_synthetic", State: supervisor.RunStopping, Detached: true}, "run run_synthetic detached while stopping; the stop is not yet confirmed", ExitOwnership},
		{"terminal", supervisor.Outcome{RunID: "run_synthetic", State: supervisor.RunCompleted}, "run run_synthetic ended completed", ExitOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := describe(tt.out); got != tt.description {
				t.Errorf("description %q, want %q", got, tt.description)
			}
			code, category := runExit(tt.out)
			if code != tt.code {
				t.Errorf("exit %d, want %d", code, tt.code)
			}
			if tt.code == ExitOwnership && category != "ownership_unresolved" {
				t.Errorf("category %q", category)
			}
		})
	}
}
