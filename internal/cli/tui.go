// Package cli wires launch for run, demo and review (design §2): the
// presentation flags (--colour plus alias, --motion, --icons, --accessible,
// --after), the launch rule choosing linear, accessible or TUI output, TTY
// detection through the tuiDeps seam, and the supervisor-backed Actions the
// TUI palette acts through (design §10).
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	isatty "github.com/mattn/go-isatty"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/tui"
	"github.com/turbokast/mythhelm/internal/tui/caps"
	"github.com/turbokast/mythhelm/internal/tui/theme"
)

// tuiDeps carries the launch helper's environment: TTY detection and the Tea
// program entry. Production uses the real terminal check and tui.Run; tests
// construct their own value to force either branch and record the selected
// entry without a PTY. It is threaded explicitly, never mutable package
// state.
type tuiDeps struct {
	isTerminal       func(w io.Writer) bool
	runTUI           func(ctx context.Context, cfg tui.Config) error
	lifecycle        func() *tuiLifecycle
	canonicalOutcome func(context.Context, string, string) (supervisor.Outcome, error)
}

// prodTUIDeps is the production seam value.
var prodTUIDeps = tuiDeps{
	isTerminal: func(w io.Writer) bool {
		f, ok := w.(interface{ Fd() uintptr })
		return ok && isatty.IsTerminal(f.Fd())
	},
	runTUI: tui.Run,
}

// tuiFlagSet holds the presentation flags addTUIFlags registers on run, demo
// and review (design §2). Only --colour is stored: its alias shares the
// same variable, so it needs no resolution — the last flag on the command
// line wins.
type tuiFlagSet struct {
	colour     *string
	motion     *string
	icons      *string
	accessible *bool
	after      *string
}

// addTUIFlags registers the TUI-slice presentation flags on fs.
func addTUIFlags(fs *flag.FlagSet) *tuiFlagSet {
	out := &tuiFlagSet{}
	out.colour = fs.String("colour", "auto", "colour mode: auto, always or never")
	fs.StringVar(out.colour, "color", "auto", "alias for --colour") //nolint:misspell // --color is the required alias spelling.
	out.motion = fs.String("motion", "auto", "motion mode: auto, full, reduced or off")
	out.icons = fs.String("icons", "auto", "icon set: auto, unicode or ascii")
	out.accessible = fs.Bool("accessible", false, "linear screen-reader stream instead of the TUI")
	out.after = fs.String("after", "", "resume the --accessible stream after this run sequence")
	return out
}

// tuiOptions is one invocation's validated presentation options.
type tuiOptions struct {
	prefs      caps.Prefs
	caps       caps.Caps
	tokens     theme.Tokens
	accessible bool
	after      int64
}

// resolve validates the flag values: capability modes through caps.Parse and
// --after as an integer. Every failure is a usageError (exit 2) naming its
// flag. Validation runs before the launch rule, so an invalid mode exits 2
// on every branch, including piped linear output.
func (f *tuiFlagSet) resolve(getenv func(string) string) (tuiOptions, error) {
	var out tuiOptions
	prefs, err := caps.Parse(*f.colour, *f.motion, *f.icons)
	if err != nil {
		return tuiOptions{}, usageError{err}
	}
	out.prefs = prefs
	out.caps = caps.Resolve(prefs, getenv)
	tokens, err := theme.BuiltIn("dark")
	if err != nil {
		return tuiOptions{}, err
	}
	out.tokens = tokens
	out.accessible = *f.accessible
	if *f.after != "" {
		after, err := strconv.ParseInt(*f.after, 10, 64)
		if err != nil {
			return tuiOptions{}, usageErrorf("--after must be an integer run sequence, got %q", *f.after)
		}
		out.after = after
	}
	return out, nil
}

// launchMode is where one invocation's output goes.
type launchMode int

const (
	// launchLinear keeps the existing plain renderer and launchJSONL the
	// existing JSONL one; both are the pre-TUI paths, unchanged.
	launchLinear launchMode = iota
	launchJSONL
	// launchAccessible selects the --accessible linear stream; launchTUI
	// the Bubble Tea program (live for run/demo, inspect for review).
	launchAccessible
	launchTUI
)

// selectLaunch evaluates the launch rule (design §2): --format jsonl keeps
// machine output byte-identical; --plain keeps explicit linear text;
// --accessible selects the screen-reader stream; otherwise piping or
// TERM=dumb stay linear and a TTY launches the TUI. format is the effective
// --format ("plain" for demo, which offers none); plain is --plain (run
// only), the sole explicit plain mode — --format plain still launches the
// TUI on a TTY.
//
// --accessible wins over piping and TERM=dumb because it is itself
// non-TTY-safe linear output with no cursor codes: a screen-reader stream
// that vanished down a pipe would be useless, and the task acceptance
// requires piped --accessible to stream. It still loses to --format jsonl
// (machine output stays machine output) and to explicit --plain.
func selectLaunch(format string, plain, accessible bool, out io.Writer, deps tuiDeps, getenv func(string) string) launchMode {
	switch {
	case format == "jsonl":
		return launchJSONL
	case plain:
		return launchLinear
	case accessible:
		return launchAccessible
	case getenv("TERM") == "dumb":
		return launchLinear
	case !deps.isTerminal(out):
		return launchLinear
	default:
		return launchTUI
	}
}

// wireActions builds the TUI palette's supervisor calls over stateDir
// (design §10): Stop opens the database read-only per call (as runStop),
// Recover opens read-write per call (as runRecover), Apply opens read-write
// and holds the run's owner lock for the call (as runApply). Every handle is
// closed before its call returns; no journal handle outlives the action that
// opened it.
func wireActions(stateDir string) tui.Actions {
	return tui.Actions{
		Stop: func(ctx context.Context, runID string) (string, error) {
			j, err := journal.OpenReadOnly(ctx, stateDir)
			if err != nil {
				return "", err
			}
			defer func() { _ = j.Close() }()
			return supervisor.Stop(ctx, j, runID)
		},
		Recover: func(ctx context.Context, runID string, h supervisor.Hooks) (supervisor.RecoveryOutcome, error) {
			j, err := journal.Open(ctx, stateDir)
			if err != nil {
				return supervisor.RecoveryOutcome{}, err
			}
			defer func() { _ = j.Close() }()
			return supervisor.RecoverWithHooks(ctx, j, runID, h)
		},
		Apply: func(ctx context.Context, runID, branch string, acceptFlags, acceptUnverified bool) (supervisor.Receipt, error) {
			j, err := journal.Open(ctx, stateDir)
			if err != nil {
				return nil, err
			}
			defer func() { _ = j.Close() }()
			if _, err := j.Run(ctx, runID); err != nil {
				return nil, err
			}
			// The palette's apply drains with the CLI's: refused while
			// migration owns the state directory or the ledger has moved
			// past previewed.
			if err := checkMigrationClear(ctx, j, "apply the run"); err != nil {
				return nil, err
			}
			release, err := supervisor.AcquireOwner(filepath.Join(stateDir, "runs", runID))
			if err != nil {
				return nil, err
			}
			defer release()
			return supervisor.ApplyRun(ctx, j, runID, branch, acceptFlags, acceptUnverified)
		},
	}
}

// superviseFunc runs an admitted run's pipeline to its outcome.
type superviseFunc func(ctx context.Context, hooks supervisor.Hooks) (supervisor.Outcome, error)

// liveOutcome is one finished pipeline.
type liveOutcome struct {
	out supervisor.Outcome
	err error
}

// outcomeMapped maps a pipeline result to the error its exit code needs,
// mirroring executeRun.
func outcomeMapped(res liveOutcome) error {
	if res.err != nil {
		return res.err
	}
	if code, category := runExit(res.out); code != ExitOK {
		return &outcomeError{code: code, category: category, err: errors.New(describe(res.out))}
	}
	return nil
}

// runLiveTUI joins the owner independently of the view's lifetime.
func runLiveTUI(stdio Stdio, deps tuiDeps, stateDir, runID string, opts tuiOptions, supervise superviseFunc) (supervisor.Outcome, error) {
	events, notices, hooks := tui.LiveFeed()
	owner := deps.lifecycleOwner()
	defer owner.close()
	hooks.Interrupt, hooks.Detach = owner.interrupt, owner.detach
	done := make(chan liveOutcome, 1)
	go func() {
		out, err := supervise(context.Background(), hooks)
		done <- liveOutcome{out: out, err: err}
	}()
	cfg := owner.config(stateDir, runID, opts, tui.ModeLive)
	cfg.Events, cfg.Notices = events, notices
	viewErr := deps.runTUI(context.Background(), cfg)
	owner.update(func(s *lifecycleState) { s.viewEnded = true })
	state := owner.snapshot()
	if viewErr != nil && !state.hasIntent {
		_, noticeErr := fmt.Fprintf(stdio.Err, "run %s continues in this process; inspect with: mythhelm review %s\n", cell(runID), cell(runID))
		viewErr = errors.Join(viewErr, noticeErr)
	}
	// Terminal loss before a snapshot still queues a single stop-less detach.
	// The supervisor retains it until canonical worker acceptance or conclusion.
	if state.lost && !state.hasAction && (!state.hasIntent || state.intent.Kind == tui.ExitDetach) {
		owner.requestDetach()
	}
	res := <-done
	state = owner.snapshot()
	mapped := outcomeMapped(res)
	projectionUnavailable := false
	if !res.out.Detached && !res.out.DetachedRunning {
		if state.hasAction {
			mapped = headlessActionError(state.action)
			validRecovery := state.action.Kind == "recover" && state.action.Err == nil && state.action.Recovery.RunID == runID && state.action.Recovery.State != ""
			if validRecovery {
				res.out = state.action.Recovery.Outcome
			} else {
				if state.action.Kind == "recover" && state.action.Err == nil {
					mapped = ownershipExit("recovery outcome unresolved")
				}
				var readErr error
				res.out, readErr = deps.finalOutcome(context.Background(), stateDir, runID)
				if readErr != nil {
					projectionUnavailable = true
					if mapped == nil {
						mapped = readErr
					} else {
						mapped = &outcomeError{code: exitCode(mapped), category: errorCategory(mapped), err: errors.Join(mapped, readErr)}
					}
				}
			}
		} else if state.hasIntent && (state.intent.Kind == tui.ExitView || state.intent.Kind == tui.ExitUnresolvedAction) {
			actual, readErr := deps.finalOutcome(context.Background(), stateDir, runID)
			res.out = actual
			if readErr != nil {
				projectionUnavailable = true
				mapped = errors.Join(readErr, res.err)
			} else {
				mapped = outcomeMapped(liveOutcome{out: actual, err: res.err})
			}
		}
	}
	mapped = lifecycleExit(mapped, state)
	if (exitCode(mapped) == ExitOwnership || projectionUnavailable) && !res.out.Detached && !res.out.DetachedRunning {
		mapped = &outcomeError{code: exitCode(mapped), category: errorCategory(mapped), err: fmt.Errorf("%w; %s", mapped, tuiRecoveryHint(stateDir, runID))}
	}
	if (state.detach && state.lost || state.hasIntent && state.intent.Kind == tui.ExitDetach) && !res.out.Detached && !res.out.DetachedRunning && !tuiDetachQualified(stateDir, runID) {
		_, writeErr := fmt.Fprintln(stdio.Out, "not qualified on this platform")
		mapped = errors.Join(mapped, writeErr)
	}
	if res.out.Detached || res.out.DetachedRunning {
		text := fmt.Sprintf(supervisor.RunningDetachedNotice, cell(runID), cell(runID))
		if res.out.Detached {
			text = fmt.Sprintf(supervisor.DetachedNotice, cell(runID))
		}
		text += "; " + tuiRecoveryHint(stateDir, runID)
		if !tuiDetachQualified(stateDir, runID) {
			text += "; not qualified on this platform"
		}
		_, writeErr := fmt.Fprintln(stdio.Out, text)
		mapped = errors.Join(ownershipExit("client detached"), writeErr)
	}
	return res.out, errors.Join(expectedViewError(viewErr, state), mapped)
}

// runInspectTUI owns signals and joins confirmed actions without a pipeline.
func runInspectTUI(deps tuiDeps, stateDir, runID string, opts tuiOptions) error {
	owner := deps.lifecycleOwner()
	defer owner.close()
	err := deps.runTUI(context.Background(), owner.config(stateDir, runID, opts, tui.ModeInspect))
	owner.update(func(s *lifecycleState) { s.viewEnded = true })
	state := owner.snapshot()
	var mapped error
	if state.hasAction {
		mapped = headlessActionError(state.action)
	}
	mapped = lifecycleExit(mapped, state)
	if exitCode(mapped) == ExitOwnership {
		message := mapped.Error()
		if state.hasAction {
			switch state.action.Kind {
			case "stop":
				switch {
				case state.action.Err == nil && state.action.StopState == "stopped":
					message = "stop confirmed"
				case state.action.Err == nil && state.action.StopState == "stop_requested":
					message = "stop requested; stop not yet confirmed"
				}
			case "apply":
				if state.action.Err == nil && headlessActionError(state.action) == nil {
					message = "apply completed; terminal lost; canonical outcome preserved"
				}
			case "recover":
				if state.action.Err == nil && state.action.Recovery.RunID == runID && state.action.Recovery.State != "" {
					message = "recovery result: " + describe(state.action.Recovery.Outcome)
				}
			}
		}
		mapped = ownershipExit(message + "; " + tuiRecoveryHint(stateDir, runID))
	}
	return errors.Join(expectedViewError(err, state), mapped)
}

// runAccessibleLive supervises an admitted run while the accessible stream
// follows it live, ending the stream (with its trailer) when the pipeline
// finishes. Pipeline events are dropped — the stream replays them from the
// journal — while unjournaled notices go to stderr so stdout stays a pure
// accessible stream. The caller must not finish the outcome: the stream's
// next-after trailer is its last line, and the returned error already
// carries the outcome's exit code.
func runAccessibleLive(stdio Stdio, stateDir, runID string, after int64, supervise superviseFunc) (supervisor.Outcome, error) {
	return runAccessibleLiveWith(stdio, stateDir, runID, after, supervise, waitForRunJournaled, finalRunJournaled)
}

// runAccessibleLiveWith is runAccessibleLive with the journal checks passed
// in, so a test can drive the fast-run fallback deterministically.
func runAccessibleLiveWith(stdio Stdio, stateDir, runID string, after int64, supervise superviseFunc,
	wait func(ctx context.Context, stateDir, runID string) error,
	final func(stateDir, runID string) (bool, error),
) (supervisor.Outcome, error) {
	interrupts := make(chan os.Signal, 2)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	hooks := supervisor.Hooks{
		Interrupt: interrupts,
		Event:     func(journal.Event) {},
		Notice:    func(s string) { _, _ = fmt.Fprintln(stdio.Err, cell(s)) },
	}
	done := make(chan liveOutcome, 1)
	go func() {
		out, err := supervise(context.Background(), hooks)
		done <- liveOutcome{out: out, err: err}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	// The stream runs on its own signal context, ended by pipeline end
	// once started or by Ctrl-C: pipeline end
	// must not cancel a stream whose initial Load has not run yet, or a
	// fast run's history collapses to a bare trailer.
	streamCtx, streamStop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer streamStop()
	started := make(chan struct{})
	// Only owner completion ends a live stream: ready_for_review can be
	// journaled before its final receipt.written event. Review keeps its
	// separate terminal watcher because it owns no pipeline to join.
	streamDone := make(chan error, 1)
	go func() {
		// The pipeline journals run.created after it starts; the stream's
		// initial Load must wait for that row, or a fast startup fails
		// with "not found" and the run exits 1 with no stream at all.
		if err := wait(ctx, stateDir, runID); err != nil {
			if ctx.Err() == nil {
				streamDone <- err
				return
			}
			// Supervision ended first: one final check, since a fast
			// run can finish inside the poll gap. Absent means the
			// pipeline failed before journaling, so there is nothing
			// to stream and the outcome below carries the exit; a read
			// error surfaces rather than an empty stream.
			found, ferr := final(stateDir, runID)
			if ferr != nil || !found {
				streamDone <- ferr
				return
			}
		}
		streamDone <- RunAccessible(streamCtx, AccessibleConfig{RunID: runID, StateDir: stateDir, Out: stdio.Out, After: after, Started: started})
	}()
	res := <-done
	stop()
	// Supervision is over: the journal will not change further, so once
	// the stream has started there is nothing left to follow. End it even
	// when the run never reaches a terminal state — a pipeline error
	// after CreateRun (a failed flush leaves it in created, which the
	// terminal watch never observes as terminal) — or shutdown wedges on
	// streamDone. Gating the stop on Started keeps a fast run's full
	// history instead of a bare trailer.
	var streamErr error
	select {
	case streamErr = <-streamDone:
	case <-started:
		streamStop()
		streamErr = <-streamDone
	}
	return res.out, errors.Join(streamErr, outcomeMapped(res))
}

// accessibleRunWaitPace paces the live stream's wait for the pipeline to
// journal its run: run.created normally lands within milliseconds.
const accessibleRunWaitPace = 20 * time.Millisecond

// waitForRunJournaled blocks until runID exists in stateDir's journal or ctx
// ends, so a live stream never loads a run the pipeline has not journaled
// yet. Absence (ErrNoDatabase, ErrNotFound) means "not yet"; any other
// journal error returns immediately instead of spinning, and callers always
// cancel ctx after the pipeline finishes, so the wait cannot outlive the run.
func waitForRunJournaled(ctx context.Context, stateDir, runID string) error {
	t := time.NewTicker(accessibleRunWaitPace)
	defer t.Stop()
	for {
		found, err := runJournaled(ctx, stateDir, runID)
		switch {
		case err == nil:
			if found {
				return nil
			}
		case errors.Is(err, journal.ErrNoDatabase) || errors.Is(err, journal.ErrNotFound):
			// Not journaled yet — keep waiting.
		case ctx.Err() != nil:
			return ctx.Err()
		default:
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// accessibleFinalCheckBound bounds the retry of the live stream's final
// check, so a read error from a writer still closing the journal (Windows
// file contention) clears instead of surfacing.
const accessibleFinalCheckBound = 500 * time.Millisecond

// finalRunJournaled is the live stream's check after supervision ended
// before the wait saw the run. Absence (ErrNoDatabase, ErrNotFound) reports
// false with no error; any other journal error is retried within
// accessibleFinalCheckBound and then returned, never read as absence.
func finalRunJournaled(stateDir, runID string) (bool, error) {
	return finalRunJournaledWith(stateDir, runID, runJournaled)
}

// finalRunJournaledWith keeps the bounded retry policy separate from the I/O
// boundary. Production always supplies the real read-only journal reader.
func finalRunJournaledWith(stateDir, runID string, read func(context.Context, string, string) (bool, error)) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), accessibleFinalCheckBound)
	defer cancel()
	t := time.NewTicker(accessibleRunWaitPace)
	defer t.Stop()
	for {
		found, err := read(ctx, stateDir, runID)
		if errors.Is(err, journal.ErrNoDatabase) || errors.Is(err, journal.ErrNotFound) {
			return false, nil
		}
		if err == nil {
			return found, nil
		}
		select {
		case <-ctx.Done():
			return false, err
		case <-t.C:
		}
	}
}

// runJournaled reports whether runID has a run row yet, distinguishing
// absence (ErrNoDatabase, ErrNotFound) from unexpected journal errors.
func runJournaled(ctx context.Context, stateDir, runID string) (bool, error) {
	j, err := journal.OpenReadOnly(ctx, stateDir)
	if err != nil {
		return false, err
	}
	defer func() { _ = j.Close() }()
	_, err = j.Run(ctx, runID)
	if err != nil {
		return false, err
	}
	return true, nil
}

// accessibleWatchInterval paces the review stream's terminal-state watch: a
// terminal run's stream ends about this long after its history page.
const accessibleWatchInterval = 100 * time.Millisecond

// runAccessibleReview streams an existing run's accessible history, following
// live only while the run is still active: a terminal run's stream ends on
// its own (prompt return), and Ctrl-C ends any stream with its trailer.
func runAccessibleReview(stdio Stdio, stateDir, runID string, after int64) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	started := make(chan struct{})
	go watchRunTerminal(ctx, stop, started, stateDir, runID)
	return RunAccessible(ctx, AccessibleConfig{RunID: runID, StateDir: stateDir, Out: stdio.Out, After: after, Started: started})
}

// watchRunTerminal ends a review stream once its run reaches a quiescent
// state: the complement of supervisor activeStates (blocked, failed,
// cancelled, completed, ready_for_review). Poll failures are ignored — a
// transient read error must not cut a live stream short. Polling starts
// only after the stream's first write: cancelling mid-startup would yield
// a bare trailer with the summary and history silently lost.
func watchRunTerminal(ctx context.Context, stop context.CancelFunc, started <-chan struct{}, stateDir, runID string) {
	select {
	case <-ctx.Done():
		return
	case <-started:
	}
	t := time.NewTicker(accessibleWatchInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if accessibleRunTerminal(ctx, stateDir, runID) {
				stop()
				return
			}
		}
	}
}

func accessibleRunTerminal(ctx context.Context, stateDir, runID string) bool {
	j, err := journal.OpenReadOnly(ctx, stateDir)
	if err != nil {
		return false
	}
	defer func() { _ = j.Close() }()
	run, err := j.Run(ctx, runID)
	if err != nil {
		return false
	}
	switch supervisor.RunState(run.State) {
	case supervisor.RunBlocked, supervisor.RunFailed, supervisor.RunCancelled, supervisor.RunCompleted,
		supervisor.RunReadyForReview:
		return true
	default:
		return false
	}
}
