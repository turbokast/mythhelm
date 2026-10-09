package cli

import (
	"strings"
	"testing"
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
