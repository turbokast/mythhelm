// Package workspace runs git safely against the user's repository and
// MYTHHELM's managed clones: preflight, snapshot and source fingerprints.
package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

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
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.Bytes(), &GitError{Args: args, ExitCode: exitErr.ExitCode(), Stderr: strings.TrimSpace(stderr.String())}
	}
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return stdout.Bytes(), nil
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
