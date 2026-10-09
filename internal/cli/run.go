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
	"strconv"
	"strings"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	billingpkg "github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/qualify"
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
	profile := fs.String("execution-profile", "", "trusted-host ("+admission.TrustedHostDisclosure+"), restricted (the default; contained where a boundary is recorded for this OS) or inspect (read-only; not yet available)")
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
	plain := fs.Bool("plain", false, "linear text output, no cursor movement or colour (overrides the TUI and --accessible)")
	nonInteractive := fs.Bool("non-interactive", false, "never ask: a decision that needs you exits 3, naming the flag that answers it")
	scenario := fs.String("scenario", "", "fake adapter scenario (default happy)")
	host := fs.String("host", "", "standalone (herdr is not available in this build)")
	addEnvelopeFlags(fs)
	tuiFlags := addTUIFlags(fs)
	positional, err := parseFlags(fs, args, stdio)
	if errors.Is(err, flag.ErrHelp) {
		return err
	}
	r := newRenderer(*format, stdio)
	var opts tuiOptions
	var envelopeFlags *billingpkg.Ceilings
	switch {
	case err != nil:
	case len(positional) > 0:
		err = usageErrorf("unexpected argument %q", positional[0])
	case *format != "plain" && *format != "jsonl":
		r = newRenderer("plain", stdio)
		err = usageErrorf("--format must be plain or jsonl, got %q", *format)
	default:
		if envelopeFlags, err = envelopeFlagCeilings(fs); err == nil {
			opts, err = tuiFlags.resolve(os.Getenv)
		}
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
		EnvelopeFlags:              envelopeFlags,
	}
	if !*nonInteractive {
		req.Confirm = prompter(stdio)
	}
	switch selectLaunch(*format, *plain, opts.accessible, stdio.Out, prodTUIDeps, os.Getenv) {
	case launchTUI:
		d, err := admission.Decide(context.Background(), req)
		if err != nil {
			err = persistDriftInvalidation(context.Background(), stateDir, err, stdio.Err)
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
			err = persistDriftInvalidation(context.Background(), stateDir, err, stdio.Err)
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
		err = persistDriftInvalidation(context.Background(), stateDir, err, stdio.Err)
		return finish(r, out, err)
	}
}

// addEnvelopeFlags registers the four envelope ceilings. Whether one was
// given is read back from the flag set, so a zero count stays a set value.
func addEnvelopeFlags(fs *flag.FlagSet) {
	fs.String("envelope-execution", "", "execution time ceiling for this run, a positive duration such as 45m")
	fs.Int("envelope-repairs", 0, "repair attempts allowed for this run (0 or more)")
	fs.Int("envelope-replans", 0, "replans allowed for this run (0 or more)")
	fs.Int("envelope-transport-retries", 0, "transport retries allowed for this run (0 or more)")
}

// envelopeFlagCeilings returns the explicit flag layer, nil when no envelope
// flag was given. Fields not given are negative (unset); a given value that
// is not a positive duration or a non-negative count is a usage error.
func envelopeFlagCeilings(fs *flag.FlagSet) (*billingpkg.Ceilings, error) {
	c := billingpkg.Ceilings{Execution: -1, Repairs: -1, Replans: -1, TransportRetries: -1}
	given := false
	var err error
	fs.Visit(func(f *flag.Flag) {
		count := func(dst *int) {
			n, convErr := strconv.Atoi(f.Value.String())
			if convErr != nil || n < 0 {
				err = errors.Join(err, usageErrorf("--%s must be 0 or more, got %s", f.Name, f.Value))
				return
			}
			*dst = n
		}
		switch f.Name {
		case "envelope-execution":
			d, parseErr := time.ParseDuration(f.Value.String())
			if parseErr != nil || d <= 0 {
				err = errors.Join(err, usageErrorf("--%s must be a positive duration, got %q", f.Name, f.Value))
				return
			}
			c.Execution = d
		case "envelope-repairs":
			count(&c.Repairs)
		case "envelope-replans":
			count(&c.Replans)
		case "envelope-transport-retries":
			count(&c.TransportRetries)
		default:
			return
		}
		given = true
	})
	if err != nil || !given {
		return nil, err
	}
	return &c, nil
}

// persistDriftInvalidation stores an invalidation revision for the record a
// strict admission blocked as drifted, so restoring the matching
// configuration cannot silently re-admit it without a new proof (I20). It is
// best-effort: a persist failure is logged to diag and never replaces the
// block, and it returns err unchanged whatever happens (I06). Any other
// error, drift under a non-strict mode included, writes nothing.
func persistDriftInvalidation(ctx context.Context, stateDir string, err error, diag io.Writer) error {
	drift, ok := errors.AsType[*admission.DriftError](err)
	if !ok {
		return err
	}
	if perr := invalidateDrifted(ctx, stateDir, drift); perr != nil {
		_, _ = fmt.Fprintf(diag, "mythhelm run: could not record the drift invalidation for key %s: %v\n", drift.KeyHash, perr)
	}
	return err
}

func invalidateDrifted(ctx context.Context, stateDir string, drift *admission.DriftError) error {
	reg, err := qualify.Open(ctx, stateDir)
	if err != nil {
		return err
	}
	defer func() { _ = reg.Close() }()
	return reg.InvalidateByHash(ctx, drift.KeyHash, drift.Reason)
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
