package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workspace"
)

// DemoCheckCommand is the hidden check helper the demo repository's
// mythhelm.toml runs: the binary re-executes itself with a pass/fail mode,
// so the demo needs no language toolchain beyond git.
const DemoCheckCommand = "__demo-check"

// demoLabel marks every demo screen: nothing shown ran a real agent or
// touched a real repository.
const demoLabel = "SCRIPTED DEMO"

// DemoCheckMain runs one demo check. It mirrors the supervisor test
// helper's contract: pass exits 0 silently, fail exits 1 with a line of
// evidence, anything else exits 2.
func DemoCheckMain(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		_, _ = fmt.Fprintln(stderr, "usage: mythhelm __demo-check pass|fail")
		return 2
	}
	switch args[0] {
	case "pass":
		return 0
	case "fail":
		_, _ = fmt.Fprintln(stdout, "demo check failed as requested")
		return 1
	default:
		_, _ = fmt.Fprintf(stderr, "unknown demo check mode %q\n", args[0])
		return 2
	}
}

func runDemo(args []string, stdio Stdio) error {
	fs := newFlagSet("demo")
	check := fs.String("check", "pass", "demo check outcome: pass or fail (fail ends the run at exit 5)")
	positional, err := parseFlags(fs, args, stdio)
	if err != nil {
		return err
	}
	if len(positional) > 0 {
		return usageErrorf("unexpected argument %q", positional[0])
	}
	if *check != "pass" && *check != "fail" {
		return usageErrorf("--check must be pass or fail, got %q", *check)
	}
	if _, err := workspace.Git(context.Background(), "", true, "--version"); err != nil {
		return fmt.Errorf("demo needs git: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating mythhelm for the demo check: %w", err)
	}

	repo, err := os.MkdirTemp("", "mythhelm-demo-repo-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(repo) }()
	state, err := os.MkdirTemp("", "mythhelm-demo-state-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(state) }()

	banner := func(screen string) {
		_, _ = fmt.Fprintf(stdio.Out, "\n=== %s: %s ===\n", demoLabel, screen)
	}
	banner("example repository")
	if _, err := fmt.Fprintf(stdio.Out, "working in %s (removed afterwards)\n", cell(repo)); err != nil {
		return err
	}
	task, digest, err := seedDemoRepo(repo, exe, *check)
	if err != nil {
		return err
	}

	banner("run")
	r := newRenderer("plain", stdio)
	out, runErr := executeRun(r, admission.Request{
		StateDir: state, Repo: repo, TaskFile: task,
		Adapter: "fake", Billing: "local-scripted", ExecutionProfile: "trusted-host",
		TrustProjectConfig: "sha256:" + digest, Env: os.Environ(),
	})

	// The run's own result line already streamed; the review screen says
	// what it produced. A run that never recorded has nothing to review.
	if out.RunID != "" {
		banner("review")
		if rerr := demoReview(stdio.Out, state, out.RunID); rerr != nil {
			return errors.Join(runErr, rerr)
		}
	}
	banner("done")
	if _, err := fmt.Fprintf(stdio.Out, "state in %s was removed afterwards\n", cell(state)); err != nil {
		return err
	}
	return runErr
}

// seedDemoRepo builds the disposable example: a committed README, a task
// and a mythhelm.toml whose check re-executes this binary. It returns the
// task path and the admitted config digest the run must trust.
func seedDemoRepo(repo, exe, mode string) (task, digest string, err error) {
	ctx := context.Background()
	git := func(args ...string) error {
		_, err := workspace.Git(ctx, repo, false, args...)
		return err
	}
	if err := git("init", "--quiet", "--initial-branch=main"); err != nil {
		return "", "", err
	}
	// Repo-local identity only: admission requires it, and the demo must
	// never touch the user's own git config.
	if err := git("config", "user.name", "Mythhelm Demo"); err != nil {
		return "", "", err
	}
	if err := git("config", "user.email", "demo@example.com"); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Scripted demo\n"), 0o600); err != nil {
		return "", "", err
	}
	task = filepath.Join(repo, "task.md")
	if err := os.WriteFile(task, []byte("# Demo task\n\nWrite demo.txt.\n"), 0o600); err != nil {
		return "", "", err
	}
	config := "schema_version = 1\n[[checks]]\nname = \"demo\"\nargv = [" +
		strconv.Quote(exe) + ", " + strconv.Quote(DemoCheckCommand) + ", " + strconv.Quote(mode) + "]\ntimeout = \"30s\"\n"
	if err := os.WriteFile(filepath.Join(repo, "mythhelm.toml"), []byte(config), 0o600); err != nil {
		return "", "", err
	}
	if err := git("add", "README.md", "task.md", "mythhelm.toml"); err != nil {
		return "", "", err
	}
	if err := git("commit", "--quiet", "-m", "scripted demo"); err != nil {
		return "", "", err
	}
	_, digest, err = admission.ParseProjectConfig([]byte(config))
	if err != nil {
		return "", "", err
	}
	return task, digest, nil
}

// demoReview prints the run's receipt summary and candidate diff without
// touching MYTHHELM_HOME: the demo's state dir is explicit throughout.
func demoReview(w io.Writer, state, runID string) error {
	if !strings.HasPrefix(runID, "run_") || strings.ContainsAny(runID, `/\.`) {
		return fmt.Errorf("invalid demo run ID %q", runID)
	}
	ctx := context.Background()
	j, err := journal.OpenReadOnly(ctx, state)
	if err != nil {
		return err
	}
	defer func() { _ = j.Close() }()
	run, err := j.Run(ctx, runID)
	if err != nil {
		return err
	}
	r, _, err := supervisor.ReadReceipt(ctx, j, run.RunID)
	if err != nil {
		return err
	}
	if r["state"] != run.State {
		return fmt.Errorf("receipt for %s does not match its run projection", run.RunID)
	}
	return writeReviewPlain(ctx, w, j, r, run.RunID, true)
}
