// Package workspace runs git safely against the user's repository and
// MYTHHELM's managed clones: preflight, snapshot and source fingerprints.
package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// ErrOutputTooLarge means git wrote more than maxStdout bytes to stdout.
var ErrOutputTooLarge = errors.New("git output exceeds the size limit")

// Output limits. Stdout over the limit fails the call rather than being cut
// short; stderr, which only explains a failure, is truncated.
var (
	maxStdout = 256 << 20
	maxStderr = 16 << 10
)

const truncatedMark = " …[truncated]"

// GitError is a git invocation that ran and exited non-zero.
type GitError struct {
	Args     []string
	ExitCode int
	Stderr   string
}

func (e *GitError) Error() string {
	return fmt.Sprintf("git %s: exit status %d: %s", strings.Join(e.Args, " "), e.ExitCode, e.Stderr)
}

// Git runs git with args in dir and returns its stdout. Every invocation gets
// fixed safety configuration: no fsmonitor, an empty hooks directory, and no
// automatic gc or maintenance, so a hook or fsmonitor planted by an agent (or
// set in repository configuration) never runs. Inherited GIT_* variables are
// dropped so the parent environment cannot redirect git to another
// repository, index or object store. When userRepo is true, git also gets
// GIT_OPTIONAL_LOCKS=0, so read commands such as status never rewrite the
// user's index (I08).
func Git(ctx context.Context, dir string, userRepo bool, args ...string) ([]byte, error) {
	return git(ctx, dir, userRepo, "", nil, args...)
}

// GitWithIndex runs git against an explicit temporary index. The index path
// is set only after inherited GIT_* variables have been removed.
func GitWithIndex(ctx context.Context, dir, index string, args ...string) ([]byte, error) {
	return git(ctx, dir, false, index, nil, args...)
}

// GitPatchSHA256 streams a binary patch into SHA-256 without the normal
// captured-output limit. The patch can be larger than the Git stdout cap.
func GitPatchSHA256(ctx context.Context, dir, base, commit string, dst io.Writer) error {
	_, err := git(ctx, dir, false, "", dst, "diff", "--binary", "--no-color", "--no-ext-diff", "--no-textconv", base, commit) //nolint:misspell // Git's flag is --no-color.
	return err
}

// GitBlob streams one object for bounded-memory validation scanning.
func GitBlob(ctx context.Context, dir, oid string, dst io.Writer) error {
	_, err := git(ctx, dir, false, "", dst, "cat-file", "blob", oid)
	return err
}

func git(ctx context.Context, dir string, userRepo bool, index string, stream io.Writer, args ...string) ([]byte, error) {
	hooks, err := emptyHooksDir()
	if err != nil {
		return nil, err
	}
	argv := append([]string{
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath=" + hooks,
		"-c", "gc.auto=0",
		"-c", "maintenance.auto=false",
	}, args...)
	cmd := exec.CommandContext(ctx, "git", argv...) // #nosec G204 -- fixed binary, argv array, no shell
	cmd.Dir = dir
	cmd.Env = gitEnv(os.Environ(), userRepo)
	if index != "" {
		cmd.Env = append(cmd.Env, "GIT_INDEX_FILE="+index)
	}
	stdout := &cappedBuffer{max: maxStdout}
	stderr := &cappedBuffer{max: maxStderr}
	if stream != nil {
		cmd.Stdout = stream
	} else {
		cmd.Stdout = stdout
	}
	cmd.Stderr = stderr
	err = cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		msg := strings.TrimSpace(stderr.buf.String())
		if stderr.over {
			msg += truncatedMark
		}
		return nil, &GitError{Args: args, ExitCode: exitErr.ExitCode(), Stderr: msg}
	case err != nil:
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	case stdout.over:
		return nil, fmt.Errorf("git %s: %w (%d bytes)", strings.Join(args, " "), ErrOutputTooLarge, maxStdout)
	}
	return stdout.buf.Bytes(), nil
}

// cappedBuffer keeps at most max bytes and records whether more arrived. It
// accepts every write, so git is never blocked on a full pipe.
type cappedBuffer struct {
	buf  bytes.Buffer
	max  int
	over bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	room := c.max - c.buf.Len()
	if len(p) > room {
		c.over = true
		c.buf.Write(p[:max(room, 0)])
		return len(p), nil
	}
	return c.buf.Write(p)
}

func gitEnv(parent []string, userRepo bool) []string {
	env := make([]string, 0, len(parent)+2)
	for _, kv := range parent {
		if !strings.HasPrefix(strings.ToUpper(kv), "GIT_") {
			env = append(env, kv)
		}
	}
	env = append(env, "GIT_TERMINAL_PROMPT=0")
	if userRepo {
		env = append(env, "GIT_OPTIONAL_LOCKS=0")
	}
	return env
}

var emptyHooksDir = sync.OnceValues(func() (string, error) {
	if runtime.GOOS != "windows" {
		// Not a directory and not creatable as one, so nothing can be
		// planted in it.
		return os.DevNull, nil
	}
	dir, err := os.MkdirTemp("", "mythhelm-hooks-")
	if err != nil {
		return "", fmt.Errorf("create empty hooks directory: %w", err)
	}
	return dir, nil
})
