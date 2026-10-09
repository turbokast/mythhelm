package integration

import (
	"errors"
	"testing"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/contain"
)

// TestCheckCommandMissingBinaryIsExplicitlyUnavailable pins the contained
// path's contract for a check binary that resolves to nothing: checkCommand
// reports the sentinel itself instead of a bare command whose Start happens
// to fail with exec.ErrNotFound.
func TestCheckCommandMissingBinaryIsExplicitlyUnavailable(t *testing.T) {
	t.Parallel()
	policy := &contain.Policy{Profile: "restricted"}
	_, err := checkCommand(admission.CheckConfig{
		Name: "missing", Argv: []string{"mythhelm-tool-that-does-not-exist"}, Timeout: "1s",
	}, t.TempDir(), nil, policy)
	if !errors.Is(err, errCheckExecutableNotFound) {
		t.Fatalf("checkCommand(missing) err = %v, want %v", err, errCheckExecutableNotFound)
	}
}
