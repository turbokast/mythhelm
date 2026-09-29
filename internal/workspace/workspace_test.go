package workspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// newRepo returns a fresh repository with one commit of a.txt on branch main.
// HOME and XDG_CONFIG_HOME point at a temp directory so the developer's own
// git configuration never leaks into the test.
func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH; workspace tests need the git CLI")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	repo := filepath.Join(t.TempDir(), "src repo")
	run(t, "", "init", "--quiet", "--initial-branch=main", repo)
	write(t, repo, "a.txt", "alpha\n")
	commit(t, repo, "first")
	return repo
}

// run executes git directly, not through the runner under test.
func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	base := []string{"-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false"}
	cmd := exec.Command("git", append(base, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, repo, name, content string) {
	t.Helper()
	p := filepath.Join(repo, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// readFile returns a file's content with CRLF line endings normalised, since
// a checkout honours the platform's core.autocrlf.
func readFile(t *testing.T, repo, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(name))) // #nosec G304 -- a test temp dir
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

func commit(t *testing.T, repo, msg string) string {
	t.Helper()
	run(t, repo, "add", "-A")
	run(t, repo, "commit", "--quiet", "-m", msg)
	return run(t, repo, "rev-parse", "HEAD")
}

func preflight(t *testing.T, repo string) PreflightResult {
	t.Helper()
	p, err := Preflight(context.Background(), repo)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	return p
}

func fingerprint(t *testing.T, repo string) string {
	t.Helper()
	fp, err := SourceFingerprint(repo)
	if err != nil {
		t.Fatalf("SourceFingerprint: %v", err)
	}
	return fp
}

func TestPreflightRecordsTopBranchAndHead(t *testing.T) {
	repo := newRepo(t)
	head := run(t, repo, "rev-parse", "HEAD")
	write(t, repo, "sub/b.txt", "beta\n")
	commit(t, repo, "second")
	head2 := run(t, repo, "rev-parse", "HEAD")

	p := preflight(t, filepath.Join(repo, "sub"))
	if !sameDir(t, p.Top, repo) {
		t.Errorf("Top = %q, want %q", p.Top, repo)
	}
	if p.Branch != "main" || p.HeadRev != head2 || len(p.HeadRev) != 40 {
		t.Errorf("Branch, HeadRev = %q, %q; want main, %s", p.Branch, p.HeadRev, head2)
	}

	run(t, repo, "checkout", "--quiet", "--detach", head)
	p = preflight(t, repo)
	if p.Branch != "" || p.HeadRev != head {
		t.Errorf("detached: Branch, HeadRev = %q, %q; want \"\", %s", p.Branch, p.HeadRev, head)
	}
}

func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	fa, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	fb, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(fa, fb)
}

func TestPreflightErrorsOutsideARepository(t *testing.T) {
	newRepo(t)
	if _, err := Preflight(context.Background(), t.TempDir()); err == nil {
		t.Fatal("Preflight outside a repository succeeded")
	}
}

func TestPreflightDetectsDirtyTrackedAndUntracked(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, repo string)
		dirty bool
	}{
		{"clean", func(*testing.T, string) {}, false},
		{"tracked modification", func(t *testing.T, repo string) { write(t, repo, "a.txt", "changed\n") }, true},
		{"staged modification", func(t *testing.T, repo string) {
			write(t, repo, "a.txt", "changed\n")
			run(t, repo, "add", "a.txt")
		}, true},
		{"untracked file", func(t *testing.T, repo string) { write(t, repo, "new.txt", "x\n") }, true},
		{"ignored file only", func(t *testing.T, repo string) {
			write(t, repo, ".gitignore", "*.log\n")
			commit(t, repo, "ignore logs")
			write(t, repo, "build.log", "x\n")
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRepo(t)
			tt.setup(t, repo)
			if got := preflight(t, repo).Dirty; got != tt.dirty {
				t.Errorf("Dirty = %v, want %v", got, tt.dirty)
			}
		})
	}
}

func TestPreflightRejectsSubmoduleLFSSparseShallow(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, repo string) string // returns the repository to inspect
		want  string
	}{
		{"submodule", func(t *testing.T, repo string) string {
			head := run(t, repo, "rev-parse", "HEAD")
			run(t, repo, "update-index", "--add", "--cacheinfo", "160000,"+head+",vendor/lib")
			run(t, repo, "commit", "--quiet", "-m", "add gitlink")
			return repo
		}, FeatureSubmodules},
		{"lfs", func(t *testing.T, repo string) string {
			write(t, repo, "assets/.gitattributes", "# media\n*.bin filter=lfs diff=lfs merge=lfs -text\n")
			commit(t, repo, "track bins with lfs")
			return repo
		}, FeatureLFS},
		{"sparse", func(t *testing.T, repo string) string {
			run(t, repo, "config", "core.sparseCheckout", "true")
			return repo
		}, FeatureSparse},
		{"shallow", func(t *testing.T, repo string) string {
			write(t, repo, "a.txt", "second\n")
			commit(t, repo, "second")
			clone := filepath.Join(t.TempDir(), "shallow")
			run(t, "", "clone", "--quiet", "--depth=1", fileURL(repo), clone)
			return clone
		}, FeatureShallow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRepo(t)
			if got := preflight(t, repo).Unsupported; len(got) != 0 {
				t.Fatalf("baseline Unsupported = %v, want none", got)
			}
			got := preflight(t, tt.setup(t, repo)).Unsupported
			if !slices.Equal(got, []string{tt.want}) {
				t.Errorf("Unsupported = %v, want [%s]", got, tt.want)
			}
		})
	}
}

// fileURL makes a file:// URL; --depth is ignored for plain local paths.
func fileURL(p string) string {
	p = filepath.ToSlash(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // C:/x becomes /C:/x
	}
	return "file://" + p
}

func TestPreflightIgnoresCommentedLFSAttribute(t *testing.T) {
	repo := newRepo(t)
	write(t, repo, ".gitattributes", "# *.bin filter=lfs\n*.txt text\n")
	commit(t, repo, "attributes")
	if got := preflight(t, repo).Unsupported; len(got) != 0 {
		t.Errorf("Unsupported = %v, want none", got)
	}
}

func TestRequireGitVersion(t *testing.T) {
	tests := []struct {
		out     string
		tooOld  bool
		invalid bool
	}{
		{out: "git version 2.43.0\n"},
		{out: "git version 2.30.0"},
		{out: "git version 2.39.3 (Apple Git-146)"},
		{out: "git version 2.47.1.windows.2"},
		{out: "git version 3.0.0"},
		{out: "git version 2.29.2", tooOld: true},
		{out: "git version 1.9.5", tooOld: true},
		{out: "hub version 2.14", invalid: true},
	}
	for _, tt := range tests {
		err := requireGitVersion(tt.out)
		switch {
		case tt.tooOld && !errors.Is(err, ErrGitTooOld):
			t.Errorf("%q: err = %v, want ErrGitTooOld", tt.out, err)
		case tt.invalid && (err == nil || errors.Is(err, ErrGitTooOld)):
			t.Errorf("%q: err = %v, want an unrecognised-version error", tt.out, err)
		case !tt.tooOld && !tt.invalid && err != nil:
			t.Errorf("%q: err = %v, want nil", tt.out, err)
		}
	}
}

func TestPreflightDoesNotWriteUserIndex(t *testing.T) {
	repo := newRepo(t)
	index := filepath.Join(repo, ".git", "index")
	// Make the index's stat cache stale: a.txt keeps its content but gets a
	// new mtime, and the index is older than it. A plain `git status` would
	// refresh the cache and rewrite the index.
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(index, past, past); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(filepath.Join(repo, "a.txt"), now, now); err != nil {
		t.Fatal(err)
	}
	before := fingerprint(t, repo)
	mtime := modTime(t, index)

	if p := preflight(t, repo); p.Dirty {
		t.Error("Dirty = true for an unchanged file with a fresh mtime")
	}

	if after := fingerprint(t, repo); after != before {
		t.Error("source fingerprint changed during Preflight")
	}
	if got := modTime(t, index); !got.Equal(mtime) {
		t.Errorf(".git/index mtime changed from %v to %v", mtime, got)
	}
}

func modTime(t *testing.T, p string) time.Time {
	t.Helper()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi.ModTime()
}

func TestSnapshotHasNoRemotesAndDetachedAtRev(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	first := run(t, repo, "rev-parse", "HEAD")
	write(t, repo, "a.txt", "second\n")
	commit(t, repo, "second")
	before := fingerprint(t, repo)

	dst := filepath.Join(t.TempDir(), "run", "workspace")
	if err := Snapshot(ctx, repo, first, dst); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	if got := run(t, dst, "rev-parse", "HEAD"); got != first {
		t.Errorf("snapshot HEAD = %s, want %s", got, first)
	}
	cmd := exec.Command("git", "symbolic-ref", "--quiet", "HEAD")
	cmd.Dir = dst
	if out, err := cmd.Output(); err == nil {
		t.Errorf("snapshot HEAD is a symbolic ref to %s, want detached", out)
	}
	if got := run(t, dst, "remote"); got != "" {
		t.Errorf("snapshot remotes = %q, want none", got)
	}
	if got := run(t, dst, "for-each-ref", "refs/remotes"); got != "" {
		t.Errorf("snapshot remote-tracking refs = %q, want none", got)
	}
	if got := readFile(t, dst, "a.txt"); got != "alpha\n" {
		t.Errorf("snapshot a.txt = %q, want the first commit's content", got)
	}
	// The runner sees the same git configuration the checkout used (on
	// Windows, the system core.autocrlf), so it is the one to ask.
	if out, err := Git(ctx, dst, false, "status", "--porcelain"); err != nil || len(out) != 0 {
		t.Errorf("snapshot is not clean: %q, %v", out, err)
	}
	if after := fingerprint(t, repo); after != before {
		t.Error("source fingerprint changed during Snapshot")
	}
}

func TestSnapshotExcludesUntrackedAndIgnored(t *testing.T) {
	repo := newRepo(t)
	write(t, repo, ".gitignore", "*.log\n")
	head := commit(t, repo, "ignore logs")
	write(t, repo, "untracked.txt", "u\n")
	write(t, repo, "build.log", "ignored\n")
	write(t, repo, "a.txt", "uncommitted edit\n")

	dst := filepath.Join(t.TempDir(), "workspace")
	if err := Snapshot(context.Background(), repo, "HEAD", dst); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	for _, name := range []string{"untracked.txt", "build.log"} {
		if _, err := os.Lstat(filepath.Join(dst, name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s in snapshot: err = %v, want not-exist", name, err)
		}
	}
	if got := readFile(t, dst, "a.txt"); got != "alpha\n" {
		t.Errorf("snapshot a.txt = %q, want the committed content", got)
	}
	if got := run(t, dst, "rev-parse", "HEAD"); got != head {
		t.Errorf("snapshot HEAD = %s, want %s", got, head)
	}
}

func TestSnapshotRejectsUnknownRevision(t *testing.T) {
	repo := newRepo(t)
	dst := filepath.Join(t.TempDir(), "workspace")
	for _, rev := range []string{"no-such-branch", "--upload-pack=touch pwned"} {
		if err := Snapshot(context.Background(), repo, rev, dst); err == nil {
			t.Errorf("Snapshot(%q) succeeded", rev)
		}
	}
	if _, err := os.Lstat(dst); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("destination created for an unknown revision: %v", err)
	}
}

func TestSnapshotRefusesExistingDestination(t *testing.T) {
	repo := newRepo(t)
	t.Run("empty directory", func(t *testing.T) {
		dst := t.TempDir()
		if err := Snapshot(context.Background(), repo, "HEAD", dst); err == nil {
			t.Fatal("Snapshot into an existing directory succeeded")
		}
		if entries, err := os.ReadDir(dst); err != nil || len(entries) != 0 {
			t.Errorf("destination after a refused Snapshot: %d entries, %v", len(entries), err)
		}
	})
	t.Run("directory with caller data", func(t *testing.T) {
		dst := t.TempDir()
		write(t, dst, "keep.txt", "caller data\n")
		if err := Snapshot(context.Background(), repo, "HEAD", dst); err == nil {
			t.Fatal("Snapshot into an existing directory succeeded")
		}
		if got := readFile(t, dst, "keep.txt"); got != "caller data\n" {
			t.Errorf("keep.txt = %q after a refused Snapshot", got)
		}
	})
}

func TestSnapshotRemovesDestinationWhenCheckoutFails(t *testing.T) {
	repo := newRepo(t)
	// A commit whose tree holds a ".git" entry: clone copies it, checkout
	// refuses it.
	blob := run(t, repo, "hash-object", "-w", "a.txt")
	cmd := exec.Command("git", "mktree")
	cmd.Dir = repo
	cmd.Stdin = strings.NewReader("100644 blob " + blob + "\t.git\n")
	tree, err := cmd.Output()
	if err != nil {
		t.Fatalf("mktree: %v", err)
	}
	bad := run(t, repo, "commit-tree", strings.TrimSpace(string(tree)), "-m", "bad")
	run(t, repo, "branch", "bad", bad)

	dst := filepath.Join(t.TempDir(), "workspace")
	err = Snapshot(context.Background(), repo, bad, dst)
	var gitErr *GitError
	if !errors.As(err, &gitErr) || gitErr.Args[0] != "checkout" {
		t.Fatalf("Snapshot err = %v, want a checkout *GitError", err)
	}
	if _, err := os.Lstat(dst); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("partial snapshot left behind: %v", err)
	}
}

func TestAgentPlantedHookNotRun(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	head := run(t, repo, "rev-parse", "HEAD")
	clone := filepath.Join(t.TempDir(), "workspace")
	if err := Snapshot(ctx, repo, head, clone); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "hook-ran")
	hook := "#!/bin/sh\necho ran >> '" + filepath.ToSlash(marker) + "'\n"
	for _, dir := range []string{filepath.Join(clone, ".git", "hooks"), filepath.Join(clone, "planted")} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "post-checkout"), []byte(hook), 0o700); err != nil { // #nosec G306 -- a hook must be executable
			t.Fatal(err)
		}
	}

	// Control: plain git runs the planted hook, so the assertions below can fail.
	run(t, clone, "worktree", "add", "--quiet", "--detach", filepath.Join(t.TempDir(), "control"), head)
	if _, err := os.Stat(marker); err != nil {
		t.Skipf("hooks do not run in this environment (%v); nothing to test", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}

	t.Run("default hooks directory", func(t *testing.T) {
		wt := filepath.Join(t.TempDir(), "verify")
		if _, err := Git(ctx, clone, false, "worktree", "add", "--quiet", "--detach", wt, head); err != nil {
			t.Fatalf("worktree add: %v", err)
		}
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("planted post-checkout hook ran (marker: %v)", err)
		}
	})
	t.Run("hooks directory set in the clone's config", func(t *testing.T) {
		run(t, clone, "config", "core.hooksPath", filepath.ToSlash(filepath.Join(clone, "planted")))
		wt := filepath.Join(t.TempDir(), "verify")
		if _, err := Git(ctx, clone, false, "worktree", "add", "--quiet", "--detach", wt, head); err != nil {
			t.Fatalf("worktree add: %v", err)
		}
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("planted post-checkout hook ran (marker: %v)", err)
		}
	})
}

func TestGitIgnoresInheritedRepositoryEnvironment(t *testing.T) {
	repo := newRepo(t)
	other := newRepo(t)
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	out, err := Git(context.Background(), repo, true, "rev-parse", "--show-toplevel")
	if err != nil {
		t.Fatalf("Git: %v", err)
	}
	if top := filepath.FromSlash(strings.TrimSpace(string(out))); !sameDir(t, top, repo) {
		t.Errorf("rev-parse --show-toplevel = %q, want %q", top, repo)
	}
}

func TestGitErrorCarriesExitCodeAndStderr(t *testing.T) {
	repo := newRepo(t)
	_, err := Git(context.Background(), repo, true, "rev-parse", "--verify", "no-such-ref")
	var gitErr *GitError
	if !errors.As(err, &gitErr) {
		t.Fatalf("err = %v, want *GitError", err)
	}
	if gitErr.ExitCode == 0 || gitErr.Stderr == "" {
		t.Errorf("GitError = %+v, want a non-zero exit code and stderr", gitErr)
	}
}

func TestGitOutputIsBounded(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	t.Run("stdout over the limit is an error", func(t *testing.T) {
		defer func(n int) { maxStdout = n }(maxStdout)
		maxStdout = 10
		if _, err := Git(ctx, repo, true, "rev-parse", "HEAD"); !errors.Is(err, ErrOutputTooLarge) {
			t.Errorf("err = %v, want ErrOutputTooLarge", err)
		}
	})
	t.Run("stderr is truncated", func(t *testing.T) {
		defer func(n int) { maxStderr = n }(maxStderr)
		maxStderr = 16
		_, err := Git(ctx, repo, true, "rev-parse", "--verify", "no-such-ref-"+strings.Repeat("x", 100))
		var gitErr *GitError
		if !errors.As(err, &gitErr) {
			t.Fatalf("err = %v, want *GitError", err)
		}
		if len(gitErr.Stderr) > 16+len(truncatedMark) || !strings.HasSuffix(gitErr.Stderr, truncatedMark) {
			t.Errorf("Stderr = %q, want at most 16 bytes plus %q", gitErr.Stderr, truncatedMark)
		}
	})
}

func TestSourceFingerprintTracksCheckoutState(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, repo string)
	}{
		{"tracked file content", func(t *testing.T, repo string) { write(t, repo, "a.txt", "changed\n") }},
		{"untracked file", func(t *testing.T, repo string) { write(t, repo, "new.txt", "n\n") }},
		{"ignored file", func(t *testing.T, repo string) { write(t, repo, "build/out.log", "o\n") }},
		{"index", func(t *testing.T, repo string) {
			write(t, repo, "new.txt", "n\n")
			run(t, repo, "add", "new.txt")
			if err := os.Remove(filepath.Join(repo, "new.txt")); err != nil {
				t.Fatal(err)
			}
		}},
		{"HEAD", func(t *testing.T, repo string) { run(t, repo, "checkout", "--quiet", "--detach", "HEAD") }},
		{"new ref", func(t *testing.T, repo string) { run(t, repo, "branch", "feature") }},
		{"packed refs", func(t *testing.T, repo string) { run(t, repo, "pack-refs", "--all") }},
		{"config", func(t *testing.T, repo string) { run(t, repo, "config", "user.name", "Someone") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRepo(t)
			before := fingerprint(t, repo)
			if again := fingerprint(t, repo); again != before {
				t.Fatalf("fingerprint unstable: %s then %s", before, again)
			}
			if !strings.HasPrefix(before, "sha256:") || len(before) != len("sha256:")+64 {
				t.Fatalf("fingerprint %q is not sha256:<hex>", before)
			}
			tt.change(t, repo)
			if after := fingerprint(t, repo); after == before {
				t.Error("fingerprint unchanged")
			}
		})
	}
}

func TestSourceFingerprintOfLinkedWorktree(t *testing.T) {
	repo := newRepo(t)
	wt := filepath.Join(t.TempDir(), "linked")
	run(t, repo, "worktree", "add", "--quiet", "-b", "side", wt)
	before := fingerprint(t, wt)
	run(t, repo, "branch", "another")
	if after := fingerprint(t, wt); after == before {
		t.Error("linked worktree fingerprint ignores refs in the common directory")
	}
}
