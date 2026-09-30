package workspace

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// ErrBranchExists means the requested destination is already occupied by a
// different commit. Apply never overwrites that branch.
var ErrBranchExists = errors.New("destination branch already exists")

// ErrInvalidBranch means git rejected the requested local branch name.
var ErrInvalidBranch = errors.New("invalid destination branch")

var revisionID = regexp.MustCompile(`^[0-9a-fA-F]{40,64}$`)
var candidateRefID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// BranchCommit returns the target of a local branch, or an empty string when
// it does not exist. The read cannot refresh the user's index.
func BranchCommit(ctx context.Context, userRepo, branch string) (string, error) {
	if err := CheckBranch(ctx, userRepo, branch); err != nil {
		return "", err
	}
	if _, err := Git(ctx, userRepo, true, "symbolic-ref", "--quiet", "refs/heads/"+branch); err == nil {
		return "", fmt.Errorf("%w: symbolic destination %s", ErrBranchExists, branch)
	} else {
		var gitErr *GitError
		if !errors.As(err, &gitErr) || gitErr.ExitCode != 1 {
			return "", err
		}
	}
	out, err := Git(ctx, userRepo, true, "rev-parse", "--verify", "--quiet", "--end-of-options", "refs/heads/"+branch)
	if err != nil {
		var gitErr *GitError
		if errors.As(err, &gitErr) && gitErr.ExitCode == 1 {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// CheckBranch rejects names git cannot safely use as local branch names.
func CheckBranch(ctx context.Context, userRepo, branch string) error {
	if branch == "" || strings.HasPrefix(branch, "-") {
		return fmt.Errorf("%w %q", ErrInvalidBranch, branch)
	}
	if _, err := Git(ctx, userRepo, true, "check-ref-format", "refs/heads/"+branch); err != nil {
		return fmt.Errorf("%w %q", ErrInvalidBranch, branch)
	}
	return nil
}

// ApplyBranch creates only refs/heads/branch in the user's repository. It
// reconciles an already-created branch before fetching, which makes a retry
// after a crash safe. The caller journals intent before invoking this method.
func ApplyBranch(ctx context.Context, userRepo, workspace, candidateRef, branch, candidateCommit string) error {
	return applyBranch(ctx, userRepo, workspace, candidateRef, branch, candidateCommit, Git)
}

func applyBranch(ctx context.Context, userRepo, workspace, candidateRef, branch, candidateCommit string,
	fetchGit func(context.Context, string, bool, ...string) ([]byte, error)) error {
	id, ok := strings.CutPrefix(candidateRef, "refs/mythhelm/candidates/")
	if !ok || !candidateRefID.MatchString(id) || !revisionID.MatchString(candidateCommit) {
		return fmt.Errorf("invalid candidate ref or commit")
	}
	commit, err := BranchCommit(ctx, userRepo, branch)
	if err != nil {
		return err
	}
	if commit == candidateCommit {
		return nil
	}
	if commit != "" {
		return fmt.Errorf("%w: %s", ErrBranchExists, branch)
	}
	if _, err := Git(ctx, workspace, false, "check-ref-format", candidateRef); err != nil {
		return fmt.Errorf("invalid candidate ref: %w", err)
	}
	got, err := Git(ctx, workspace, false, "rev-parse", "--verify", candidateRef+"^{commit}")
	if err != nil || strings.TrimSpace(string(got)) != candidateCommit {
		return fmt.Errorf("candidate ref does not match frozen commit")
	}
	// Import objects without updating any ref. A fetch destination refspec
	// can fast-forward a branch created concurrently after our preflight,
	// even without force. The separate compare-and-swap below forbids that.
	_, err = fetchGit(ctx, userRepo, false, "-c", "fetch.writeCommitGraph=false", "fetch", "--no-write-fetch-head", "--no-tags", "--no-recurse-submodules", "--refmap=",
		workspace, candidateRef)
	if err != nil {
		return fmt.Errorf("fetch candidate: %w", err)
	}
	if err := createBranch(ctx, userRepo, branch, candidateCommit); err != nil {
		current, readErr := BranchCommit(ctx, userRepo, branch)
		if errors.Is(err, ErrBranchExists) || errors.Is(readErr, ErrBranchExists) || (readErr == nil && current != "") {
			return fmt.Errorf("%w: %s", ErrBranchExists, branch)
		}
		return fmt.Errorf("create candidate branch: %w", err)
	}
	commit, err = BranchCommit(ctx, userRepo, branch)
	if err != nil {
		return err
	}
	if commit != candidateCommit {
		return fmt.Errorf("applied branch %s does not match candidate", branch)
	}
	return nil
}

// createBranch prepares a no-deref Git transaction, checks the locked ref
// for a dangling symbolic destination, then commits. A zero expected OID
// alone also accepts dangling symrefs, so it cannot protect their identity.
func createBranch(ctx context.Context, repo, branch, commit string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd, err := gitCommand(ctx, repo, false, "update-ref", "--stdin")
	if err != nil {
		return err
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		_ = in.Close()
		return err
	}
	stderr := &cappedBuffer{max: maxStderr}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = in.Close()
		return err
	}
	defer func() { _ = in.Close(); _ = cmd.Wait() }()
	reader := bufio.NewReader(out)
	command := func(request, expected string) error {
		if _, err := io.WriteString(in, request); err != nil {
			return err
		}
		response, err := reader.ReadString('\n')
		if err != nil || response != expected+"\n" {
			return fmt.Errorf("Git ref transaction %s: %w", expected, errors.Join(err, errors.New("unexpected acknowledgement")))
		}
		return nil
	}
	if err := command("start\n", "start: ok"); err != nil {
		return err
	}
	if err := command("option no-deref\ncreate refs/heads/"+branch+" "+commit+"\nprepare\n", "prepare: ok"); err != nil {
		return err
	}
	// Git holds this exact ref's lock until commit or EOF. No Git writer
	// can replace it between this identity check and the atomic creation.
	if current, err := BranchCommit(ctx, repo, branch); err != nil {
		return err
	} else if current != "" {
		return ErrBranchExists
	}
	return command("commit\n", "commit: ok")
}
