// Launch wiring for run, demo and review (design §2): the presentation
// flags (--colour plus alias, --motion, --icons, --accessible, --after),
// the launch rule choosing linear, accessible or TUI output, TTY detection
// through the tuiDeps seam, and the supervisor-backed Actions the TUI
// palette acts through (design §10).
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
	isTerminal func(w io.Writer) bool
	runTUI     func(ctx context.Context, cfg tui.Config) error
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

// runLiveTUI supervises an admitted run under the TUI: the pipeline streams
// through a LiveFeed (design §13) while the Tea program renders it. When the
// TUI quits after the run ended, the run's outcome is returned for the
// caller to finish; when it quits while the run is still active, the run is
// detached exactly like the second Ctrl-C today — two interrupts, then the
// reattach line naming `review <run>` (I06) — and the detached outcome is
// returned. The pipeline is always joined before returning.
func runLiveTUI(stdio Stdio, deps tuiDeps, stateDir, runID string, opts tuiOptions, supervise superviseFunc) (supervisor.Outcome, error) {
	events, notices, hooks := tui.LiveFeed()
	interrupts := make(chan os.Signal, 2)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	hooks.Interrupt = interrupts
	done := make(chan liveOutcome, 1)
	go func() {
		out, err := supervise(context.Background(), hooks)
		done <- liveOutcome{out: out, err: err}
	}()
	cfg := tui.Config{
		RunID: runID, StateDir: stateDir,
		Caps: opts.caps, Tokens: opts.tokens,
		Events: events, Notices: notices,
		Actions: wireActions(stateDir),
	}
	tuiErr := deps.runTUI(context.Background(), cfg)
	select {
	case res := <-done:
		return res.out, errors.Join(tuiErr, outcomeMapped(res))
	default:
	}
	// The run is still active: detach before the stop is confirmed. The
	// sends run in a goroutine so a pipeline phase that is not listening
	// cannot wedge the join; a pipeline that finishes on its own simply
	// reports its outcome instead of a detach.
	go func() {
		interrupts <- os.Interrupt
		interrupts <- os.Interrupt
	}()
	res := <-done
	var werr error
	if res.out.Detached {
		_, werr = fmt.Fprintf(stdio.Out, "detached from run %s; reattach with: mythhelm review %s\n", cell(runID), cell(runID))
	}
	return res.out, errors.Join(tuiErr, outcomeMapped(res), werr)
}

// runInspectTUI renders an existing run's snapshot with polling (design §13
// inspect mode): no live channel, so quitting is instant.
func runInspectTUI(deps tuiDeps, stateDir, runID string, opts tuiOptions) error {
	cfg := tui.Config{
		RunID: runID, StateDir: stateDir,
		Caps: opts.caps, Tokens: opts.tokens,
		Actions: wireActions(stateDir),
	}
	return deps.runTUI(context.Background(), cfg)
}

// runAccessibleLive supervises an admitted run while the accessible stream
// follows it live, ending the stream (with its trailer) when the pipeline
// finishes. Pipeline events are dropped — the stream replays them from the
// journal — while unjournaled notices go to stderr so stdout stays a pure
// accessible stream. The caller must not finish the outcome: the stream's
// next-after trailer is its last line, and the returned error already
// carries the outcome's exit code.
func runAccessibleLive(stdio Stdio, stateDir, runID string, after int64, supervise superviseFunc) (supervisor.Outcome, error) {
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
	streamDone := make(chan error, 1)
	go func() {
		streamDone <- RunAccessible(ctx, AccessibleConfig{RunID: runID, StateDir: stateDir, Out: stdio.Out, After: after})
	}()
	res := <-done
	stop()
	streamErr := <-streamDone
	return res.out, errors.Join(streamErr, outcomeMapped(res))
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
	go watchRunTerminal(ctx, stop, stateDir, runID)
	return RunAccessible(ctx, AccessibleConfig{RunID: runID, StateDir: stateDir, Out: stdio.Out, After: after})
}

// watchRunTerminal ends a review stream once its run reaches a terminal
// state (state.go: blocked, failed, cancelled, completed). Poll failures are
// ignored — a transient read error must not cut a live stream short.
func watchRunTerminal(ctx context.Context, stop context.CancelFunc, stateDir, runID string) {
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
	case supervisor.RunBlocked, supervisor.RunFailed, supervisor.RunCancelled, supervisor.RunCompleted:
		return true
	default:
		return false
	}
}
