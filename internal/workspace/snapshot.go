package workspace

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// Snapshot clones the committed revision rev of the repository at src into
// the new directory dst (AC-3.3). The clone copies objects instead of
// hardlinking them, is checked out detached at rev, and has every remote
// removed so nothing in it can push back into src. Untracked and ignored files
// in src never reach dst, because only committed objects are cloned.
func Snapshot(ctx context.Context, src, rev, dst string) error {
	src, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	if dst, err = filepath.Abs(dst); err != nil {
		return err
	}
	oid, err := gitLine(ctx, src, "rev-parse", "--verify", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return fmt.Errorf("resolve %q: %w", rev, err)
	}
	if _, err := Git(ctx, src, true, "clone", "--quiet", "--no-hardlinks", "--no-checkout", "--", src, dst); err != nil {
		return err
	}
	if _, err := Git(ctx, dst, false, "checkout", "--quiet", "--detach", oid); err != nil {
		return err
	}
	remotes, err := Git(ctx, dst, false, "remote")
	if err != nil {
		return err
	}
	for _, name := range strings.Fields(string(remotes)) {
		if _, err := Git(ctx, dst, false, "remote", "remove", name); err != nil {
			return err
		}
	}
	return nil
}
