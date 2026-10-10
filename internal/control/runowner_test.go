package control

import (
	"os"
	"path/filepath"
	"testing"
)

// A run id names a directory under runs/, so stop and recover validate it
// before they use it as a path component. Each id below would resolve to a
// directory that exists, so only the validation refuses it.
func TestRunIDsAreValidatedBeforeUseAsPathComponents(t *testing.T) {
	t.Parallel()
	ids := []string{"..", "../escape", "run_x/..", "run_a/b", `run_a\b`, "/abs", "run_x/../../escape", ".", "run_é"}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			f := newRecoverState(t, "run_rid_ok", "att_rid_ok")
			dir := f.dir
			if err := os.MkdirAll(filepath.Join(dir, "runs", "run_a", "b"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(dir, "escape"), 0o700); err != nil {
				t.Fatal(err)
			}
			release, err := acquireRunOwner(dir, id, "stop")
			if release != nil {
				release()
			}
			requireCode(t, err, CodeInvalidContract)

			params := recoverIntent(t, "op_rec_badid", id)
			_, err = Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, params)
			requireCode(t, err, CodeInvalidContract)
		})
	}
}
