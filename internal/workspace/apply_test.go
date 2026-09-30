package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyRefusesBranchCreatedDuringFetch(t *testing.T) {
	repo, clone, ref, candidate := applyFixture(t)
	base := applyGit(t, repo, "rev-parse", "HEAD")
	fetch := func(ctx context.Context, dir string, userRepo bool, args ...string) ([]byte, error) {
		// The competing writer creates the branch after ApplyBranch checked
		// it, while its objects are being imported. It is a fast-forward
		// ancestor of the candidate, so a no-force destination fetch would
		// have overwritten it.
		applyGit(t, repo, "update-ref", "refs/heads/competing", base)
		return Git(ctx, dir, userRepo, args...)
	}
	err := applyBranch(t.Context(), repo, clone, ref, "competing", candidate, fetch)
	if !errors.Is(err, ErrBranchExists) {
		t.Fatalf("apply error = %v, want branch refusal", err)
	}
	if got := applyGit(t, repo, "rev-parse", "refs/heads/competing"); got != base {
		t.Fatalf("concurrent branch overwritten: %s -> %s", base, got)
	}
}

func TestApplyRefusesReflogShorthand(t *testing.T) {
	repo, clone, ref, candidate := applyFixture(t)
	applyGit(t, repo, "checkout", "-qb", "other")
	applyGit(t, repo, "checkout", "-q", "main")
	if err := CheckBranch(t.Context(), repo, "@{-1}"); !errors.Is(err, ErrInvalidBranch) {
		t.Fatalf("shorthand accepted: %v", err)
	}
	if err := ApplyBranch(t.Context(), repo, clone, ref, "@{-1}", candidate); !errors.Is(err, ErrInvalidBranch) {
		t.Fatalf("apply shorthand: %v", err)
	}
}

func TestApplyRefusesSymbolicDestination(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		t.Run(fmt.Sprint(concurrent), func(t *testing.T) {
			repo, clone, ref, candidate := applyFixture(t)
			plant := func() { applyGit(t, repo, "symbolic-ref", "refs/heads/destination", "refs/heads/untouched") }
			if !concurrent {
				plant()
			}
			fetch := func(ctx context.Context, dir string, userRepo bool, args ...string) ([]byte, error) {
				if concurrent {
					plant()
				}
				return Git(ctx, dir, userRepo, args...)
			}
			if err := applyBranch(t.Context(), repo, clone, ref, "destination", candidate, fetch); !errors.Is(err, ErrBranchExists) {
				t.Fatalf("symbolic destination accepted: %v", err)
			}
			if got := applyGit(t, repo, "symbolic-ref", "refs/heads/destination"); got != "refs/heads/untouched" {
				t.Fatalf("symbolic ref replaced: %s", got)
			}
			if got, err := BranchCommit(t.Context(), repo, "untouched"); err != nil || got != "" {
				t.Fatalf("symbolic target changed: %q %v", got, err)
			}
		})
	}
}

func applyGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...) // #nosec G204 -- test invokes git with fixed fixture arguments
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func applyFixture(t *testing.T) (userRepo, clone, ref, candidate string) {
	t.Helper()
	userRepo = t.TempDir()
	applyGit(t, userRepo, "init", "--quiet", "--initial-branch=main")
	applyGit(t, userRepo, "config", "user.name", "Test")
	applyGit(t, userRepo, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(userRepo, "README.md"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	applyGit(t, userRepo, "add", "README.md")
	applyGit(t, userRepo, "commit", "--quiet", "-m", "base")
	applyGit(t, userRepo, "tag", "base")
	clone = filepath.Join(t.TempDir(), "clone")
	applyGit(t, userRepo, "clone", "--quiet", userRepo, clone)
	applyGit(t, clone, "config", "user.name", "Test")
	applyGit(t, clone, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(clone, "candidate.txt"), []byte("candidate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	applyGit(t, clone, "add", "candidate.txt")
	applyGit(t, clone, "commit", "--quiet", "-m", "candidate")
	candidate = applyGit(t, clone, "rev-parse", "HEAD")
	ref = "refs/mythhelm/candidates/att_test"
	applyGit(t, clone, "update-ref", ref, candidate)
	return
}

func TestApplyCreatesOnlyTheBranch(t *testing.T) {
	repo, clone, ref, candidate := applyFixture(t)
	before, err := SourceFingerprint(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyBranch(t.Context(), repo, clone, ref, "review/candidate", candidate); err != nil {
		t.Fatal(err)
	}
	if got := applyGit(t, repo, "rev-parse", "refs/heads/review/candidate"); got != candidate {
		t.Fatalf("branch = %s, want %s", got, candidate)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "FETCH_HEAD")); !os.IsNotExist(err) {
		t.Fatalf("FETCH_HEAD was written: %v", err)
	}
	applyGit(t, repo, "update-ref", "-d", "refs/heads/review/candidate")
	after, err := SourceFingerprint(repo)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("source checkout changed beyond new branch: %s -> %s", before, after)
	}
}

func TestApplyDoesNotRunUserHooks(t *testing.T) {
	repo, clone, ref, candidate := applyFixture(t)
	hooks := filepath.Join(repo, ".git", "hooks")
	marker := filepath.Join(t.TempDir(), "hook-ran")
	script := "#!/bin/sh\ntouch '" + filepath.ToSlash(marker) + "'\n"
	hook := filepath.Join(hooks, "reference-transaction")
	if err := os.WriteFile(hook, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(hook, 0o700); err != nil { // #nosec G302 -- the hook fixture must be executable to test suppression
		t.Fatal(err)
	}
	if err := ApplyBranch(t.Context(), repo, clone, ref, "candidate", candidate); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("user hook ran: %v", err)
	}
}

func TestApplyRefusesInvalidRefName(t *testing.T) {
	repo, clone, ref, candidate := applyFixture(t)
	for _, branch := range []string{"../escape", "-option", "bad..name", "main.lock"} {
		if err := ApplyBranch(t.Context(), repo, clone, ref, branch, candidate); err == nil {
			t.Errorf("accepted invalid branch %q", branch)
		}
	}
	if err := ApplyBranch(t.Context(), repo, clone, "refs/mythhelm/candidates/../bad", "valid", candidate); err == nil {
		t.Fatal("accepted invalid candidate ref")
	}
}
