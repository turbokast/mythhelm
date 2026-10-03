package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/statedir"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

// maxAnswer bounds one line typed at a prompt.
const maxAnswer = 256

// runRun is `mythhelm run`: admit a task, then supervise its one native
// attempt (design §3, §10).
func runRun(args []string, stdio Stdio) error {
	fs := newFlagSet("run")
	taskFile := fs.String("task-file", "", "the task, as Markdown, delivered to the agent on stdin (required)")
	adapterName := fs.String("adapter", "", "the native harness adapter: claudecode or fake (required; no default, no fallback)")
	billing := fs.String("billing", "", "billing posture: subscription-declared, subscription-only or local-scripted (required)")
	profile := fs.String("execution-profile", "", "trusted-host: native tools and checks run with your host authority, not contained")
	useCommitted := fs.Bool("use-committed", false, "run on the committed HEAD when the checkout has uncommitted changes")
	rev := fs.String("rev", "", "run on this committed revision instead of HEAD")
	trustConfig := fs.String("trust-project-config", "", "trust the admitted mythhelm.toml digest (sha256:<hex>)")
	trustNative := fs.String("trust-native-config", "", "trust the inventoried native config digest (sha256:<hex>; claudecode only)")
	stripCreds := fs.Bool("strip-credential-env", false, "remove credential-route variables from the native child only (claudecode only)")
	declareEntitlement := fs.String("declare-entitlement", "", "record plan=<pro|max|team|enterprise>,extra-usage=disabled for this identity (claudecode only)")
	allowUntested := fs.Bool("allow-untested-native-version", false, "run a native version without fixtures, labelled experimental (claudecode only)")
	noChecks := fs.Bool("no-checks", false, "waive checks; result is unverified and exits 5")
	keepGoing := fs.Bool("keep-going", false, "continue checks after an unavailable executable")
	format := fs.String("format", "plain", "output format: plain or jsonl")
	plain := fs.Bool("plain", false, "linear text output, no cursor movement or colour (the only text output in this build)")
	nonInteractive := fs.Bool("non-interactive", false, "never ask: a decision that needs you exits 3, naming the flag that answers it")
	scenario := fs.String("scenario", "", "fake adapter scenario (default happy)")
	host := fs.String("host", "", "standalone (herdr is not available in this build)")
	tuiFlags := addTUIFlags(fs)
	positional, err := parseFlags(fs, args, stdio)
	if errors.Is(err, flag.ErrHelp) {
		return err
	}
	r := newRenderer(*format, stdio)
	var opts tuiOptions
	switch {
	case err != nil:
	case len(positional) > 0:
		err = usageErrorf("unexpected argument %q", positional[0])
	case *format != "plain" && *format != "jsonl":
		r = newRenderer("plain", stdio)
		err = usageErrorf("--format must be plain or jsonl, got %q", *format)
	default:
		opts, err = tuiFlags.resolve(os.Getenv)
	}
	if err != nil {
		return finish(r, supervisor.Outcome{}, err)
	}

	stateDir, err := statedir.Resolve()
	if err != nil {
		return finish(r, supervisor.Outcome{}, usageError{err})
	}
	cwd, err := os.Getwd()
	if err != nil {
		return finish(r, supervisor.Outcome{}, err)
	}
	req := admission.Request{
		StateDir:                   stateDir,
		Repo:                       cwd,
		TaskFile:                   *taskFile,
		Adapter:                    *adapterName,
		Billing:                    *billing,
		ExecutionProfile:           *profile,
		UseCommitted:               *useCommitted,
		Rev:                        *rev,
		Host:                       *host,
		Scenario:                   *scenario,
		Env:                        os.Environ(),
		TrustProjectConfig:         *trustConfig,
		NoChecks:                   *noChecks,
		KeepGoing:                  *keepGoing,
		StripCredentialEnv:         *stripCreds,
		TrustNativeConfig:          *trustNative,
		DeclareEntitlement:         *declareEntitlement,
		AllowUntestedNativeVersion: *allowUntested,
	}
	if !*nonInteractive {
		req.Confirm = prompter(stdio)
	}
	switch selectLaunch(*format, *plain, opts.accessible, stdio.Out, prodTUIDeps, os.Getenv) {
	case launchTUI:
		d, err := admission.Decide(context.Background(), req)
		if err != nil {
			return finish(newRenderer("plain", stdio), supervisor.Outcome{}, err)
		}
		out, err := runLiveTUI(stdio, prodTUIDeps, stateDir, d.RunID, opts,
			func(ctx context.Context, hooks supervisor.Hooks) (supervisor.Outcome, error) {
				return supervisor.Run(ctx, d, hooks)
			})
		return finish(newRenderer("plain", stdio), out, err)
	case launchAccessible:
		d, err := admission.Decide(context.Background(), req)
		if err != nil {
			return finish(newRenderer("plain", stdio), supervisor.Outcome{}, err)
		}
		// No finish: the stream's next-after trailer is its last line,
		// and the returned error already carries the outcome's exit code.
		_, err = runAccessibleLive(stdio, stateDir, d.RunID, opts.after,
			func(ctx context.Context, hooks supervisor.Hooks) (supervisor.Outcome, error) {
				return supervisor.Run(ctx, d, hooks)
			})
		return err
	default:
		out, err := executeRun(r, req)
		return finish(r, out, err)
	}
}

// executeRun admits req and supervises its one native attempt, streaming
// events through r. `run` and `demo` share it; only the request's repo,
// state dir and pre-supplied grants differ.
func executeRun(r renderer, req admission.Request) (supervisor.Outcome, error) {
	ctx := context.Background()
	d, err := admission.Decide(ctx, req)
	if err != nil {
		return supervisor.Outcome{}, err
	}
	interrupts := make(chan os.Signal, 2)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	out, err := supervisor.Run(ctx, d, supervisor.Hooks{Interrupt: interrupts, Event: r.event, Notice: r.notice})
	if err == nil {
		code, category := runExit(out)
		if code != ExitOK {
			err = &outcomeError{code: code, category: category, err: errors.New(describe(out))}
		}
	}
	return out, err
}

// finish writes run.result for err (nil on success) and returns err, or the
// first output error.
func finish(r renderer, out supervisor.Outcome, err error) error {
	res := runResult{RunID: out.RunID, State: string(out.State), Reason: out.Reason,
		AttemptReason: out.AttemptReason, Code: exitCode(err), Category: errorCategory(err)}
	if out.State == "" {
		res.Reason = reasonOf(err)
	}
	r.result(res)
	if err == nil {
		return r.err()
	}
	return err
}

// reasonOf names why a run was never recorded.
func reasonOf(err error) string {
	var blocked *admission.BlockedError
	switch {
	case err == nil:
		return ""
	case errors.As(err, &blocked):
		return blocked.Code
	}
	return errorCategory(err)
}

func describe(o supervisor.Outcome) string {
	switch {
	case o.Detached:
		return fmt.Sprintf("run %s detached while stopping; the stop is not yet confirmed", o.RunID)
	case o.ActiveRun != "":
		return fmt.Sprintf("run %s blocked (active_run_exists): run %s is active; one run at a time in this build", o.RunID, o.ActiveRun)
	}
	msg := fmt.Sprintf("run %s ended %s", o.RunID, o.State)
	if o.Reason != "" {
		msg += " (" + o.Reason + ")"
	}
	if o.AttemptReason != "" {
		msg += fmt.Sprintf("; attempt %s %s (%s)", o.AttemptID, o.AttemptState, o.AttemptReason)
	}
	return msg
}

// prompter asks yes/no questions on stderr and reads the answer from stdin.
// Anything but y or yes, including end of input, is no.
func prompter(stdio Stdio) func(string) (bool, error) {
	in := bufio.NewReader(stdio.In)
	return func(question string) (bool, error) {
		if _, err := fmt.Fprintf(stdio.Err, "%s [y/N] ", question); err != nil {
			return false, err
		}
		line, err := readAnswer(in)
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		}
		return false, nil
	}
}

func readAnswer(r *bufio.Reader) (string, error) {
	var b strings.Builder
	for b.Len() < maxAnswer {
		c, err := r.ReadByte()
		if err != nil {
			return b.String(), err
		}
		if c == '\n' {
			return b.String(), nil
		}
		b.WriteByte(c)
	}
	return "", errors.New("answer too long")
}
