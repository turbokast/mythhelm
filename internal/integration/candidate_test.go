package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/integration"
)

type repoFixture struct {
	dir, base string
}

func fixture(t *testing.T) repoFixture {
	t.Helper()
	f := repoFixture{dir: t.TempDir()}
	f.git(t, "init", "--quiet", "--initial-branch=main")
	f.git(t, "config", "user.name", "Test User")
	f.git(t, "config", "user.email", "test@example.com")
	f.write(t, "README.md", "base\n")
	f.write(t, ".gitignore", "ignored/\n")
	f.git(t, "add", "README.md", ".gitignore")
	f.git(t, "commit", "--quiet", "-m", "base")
	f.base = f.git(t, "rev-parse", "HEAD")
	return f
}

func (f repoFixture) git(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f repoFixture) write(t *testing.T, name, data string) {
	t.Helper()
	p := filepath.Join(f.dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (f repoFixture) freeze(t *testing.T) integration.Candidate {
	t.Helper()
	c, err := integration.Freeze(t.Context(), f.dir, f.base, integration.CommitMeta{
		RunID: "run_test", AttemptID: "att_test", Title: "# Demo task",
		Name: "Test User", Email: "test@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func flagExists(c integration.Candidate, name string) bool {
	return slices.ContainsFunc(c.Flags, func(f integration.Flag) bool { return f.Name == name })
}

func TestFreezeIncludesUncommittedAndUntracked(t *testing.T) {
	f := fixture(t)
	f.write(t, "README.md", "modified\n")
	f.write(t, "new.txt", "untracked\n")
	f.write(t, "ignored/output.log", "ignored\n")
	head := f.git(t, "rev-parse", "HEAD")
	c := f.freeze(t)
	if c.BaseRev != f.base || c.Commit == "" || c.Tree == "" || len(c.PatchSHA256) != 64 {
		t.Fatalf("incomplete candidate: %+v", c)
	}
	if f.git(t, "rev-parse", "HEAD") != head || f.git(t, "rev-parse", "HEAD^{tree}") == c.Tree {
		t.Fatal("freeze changed HEAD or failed to capture the working tree")
	}
	if got := f.git(t, "show", c.Commit+":README.md"); got != "modified" {
		t.Fatalf("tracked content = %q", got)
	}
	if got := f.git(t, "show", c.Commit+":new.txt"); got != "untracked" {
		t.Fatalf("untracked content = %q", got)
	}
	if !flagExists(c, "ignored_outputs") {
		t.Fatalf("missing ignored_outputs: %+v", c.Flags)
	}
	if got := f.git(t, "rev-parse", "refs/mythhelm/candidates/att_test"); got != c.Commit {
		t.Fatalf("candidate ref = %s, commit = %s", got, c.Commit)
	}
}

func TestFreezeIgnoresAgentMovedHEAD(t *testing.T) {
	f := fixture(t)
	f.write(t, "discard.txt", "discarded\n")
	f.git(t, "add", "discard.txt")
	f.git(t, "commit", "--quiet", "-m", "agent commit")
	f.git(t, "reset", "--hard", f.base)
	f.write(t, "final.txt", "kept\n")
	c := f.freeze(t)
	if got := f.git(t, "rev-parse", c.Commit+"^"); got != f.base {
		t.Fatalf("candidate parent = %s, want admitted base %s", got, f.base)
	}
	if got := f.git(t, "show", c.Commit+":final.txt"); got != "kept" {
		t.Fatalf("final content = %q", got)
	}
	if slices.ContainsFunc(c.Changed, func(p integration.ChangedPath) bool { return p.Path == "discard.txt" }) {
		t.Fatalf("discarded agent commit leaked into candidate: %+v", c.Changed)
	}
}

func TestFreezeAutocrlfDoesNotChangeCleanSnapshot(t *testing.T) {
	f := fixture(t)
	f.git(t, "config", "core.autocrlf", "true")
	f.git(t, "checkout-index", "--force", "--all")
	c := f.freeze(t)
	if len(c.Changed) != 0 || c.Tree != f.git(t, "rev-parse", f.base+"^{tree}") {
		t.Fatalf("clean checkout changed by line endings: %+v", c.Changed)
	}
}

func TestFlagsSymlinkEscapeBinaryLargeSecretConfigChange(t *testing.T) {
	for _, tc := range []struct {
		name, flag string
		setup      func(*testing.T, repoFixture)
	}{
		{"symlink_escape", "symlink_escape", func(t *testing.T, f repoFixture) {
			if runtime.GOOS == "windows" {
				t.Skip("symlink creation needs Windows developer mode")
			}
			if err := os.Symlink(filepath.Join(t.TempDir(), "outside"), filepath.Join(f.dir, "escape")); err != nil {
				t.Fatal(err)
			}
		}},
		{"binary", "binary", func(t *testing.T, f repoFixture) { f.write(t, "binary.dat", "one\x00two") }},
		{"large_file", "large_file", func(t *testing.T, f repoFixture) { f.write(t, "large.dat", strings.Repeat("x", (1<<20)+1)) }},
		{"secret_pattern", "secret_pattern", func(t *testing.T, f repoFixture) { f.write(t, "secret.txt", "token=obviously_fake_fixture_value\n") }},
		{"check_config_changed", "check_config_changed", func(t *testing.T, f repoFixture) { f.write(t, "mythhelm.toml", "schema_version = 1\n") }},
		{"test_files_changed", "test_files_changed", func(t *testing.T, f repoFixture) { f.write(t, "feature_test.go", "package demo\n") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fixture(t)
			tc.setup(t, f)
			c := f.freeze(t)
			if !flagExists(c, tc.flag) {
				t.Fatalf("missing %s: %+v", tc.flag, c.Flags)
			}
		})
	}
}

func TestCandidateHasNoSignedOffBy(t *testing.T) {
	f := fixture(t)
	f.write(t, "new.txt", "hello\n")
	c := f.freeze(t)
	message := f.git(t, "log", "-1", "--format=%B", c.Commit)
	if strings.Contains(message, "Signed-off-by:") || !strings.Contains(message, "MYTHHELM-Run: run_test") {
		t.Fatalf("candidate message = %q", message)
	}
	if got := f.git(t, "log", "-1", "--format=%an <%ae>", c.Commit); got != "Test User <test@example.com>" {
		t.Fatalf("candidate author = %q", got)
	}
}
