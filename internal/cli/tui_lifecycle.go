package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/tui"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

// lifecycleState is guarded by tuiLifecycle.mu. Callbacks only update this
// bounded mailbox; the single pump owns delivery and is joined by close.
type lifecycleState struct {
	stop, detach bool
	lost         bool
	viewEnded    bool
	hasIntent    bool
	intent       tui.ExitIntent
	hasAction    bool
	action       tui.ActionCompletion
}

// latchTerminalDetach is called only under the mailbox lock. A late loss
// cannot override an already chosen view/action disposition.
func (s *lifecycleState) latchTerminalDetach() {
	if s.viewEnded && s.lost && !s.hasAction && (!s.hasIntent || s.intent.Kind == tui.ExitDetach) {
		s.detach = true
	}
}

type tuiLifecycle struct {
	mu        sync.Mutex
	state     lifecycleState
	wake      chan struct{}
	quit      chan struct{}
	done      chan struct{}
	signals   chan tui.ClientSignal
	osSignals chan os.Signal
	interrupt chan os.Signal
	detach    chan struct{}
}

func (d tuiDeps) lifecycleOwner() *tuiLifecycle {
	if d.lifecycle != nil {
		return d.lifecycle()
	}
	return newTUILifecycle()
}
func newTUILifecycle() *tuiLifecycle {
	c := &tuiLifecycle{wake: make(chan struct{}, 1), quit: make(chan struct{}), done: make(chan struct{}), signals: make(chan tui.ClientSignal, 16), osSignals: make(chan os.Signal, 16), interrupt: make(chan os.Signal, 2), detach: make(chan struct{}, 1)}
	signal.Notify(c.osSignals, tuiOSSignals()...)
	go c.pump()
	return c
}
func (c *tuiLifecycle) close() { signal.Stop(c.osSignals); close(c.quit); <-c.done }
func (c *tuiLifecycle) update(f func(*lifecycleState)) {
	c.mu.Lock()
	f(&c.state)
	c.mu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default:
	}
}
func (c *tuiLifecycle) snapshot() lifecycleState { c.mu.Lock(); defer c.mu.Unlock(); return c.state }
func (c *tuiLifecycle) endView() {
	c.update(func(s *lifecycleState) { s.viewEnded = true; s.latchTerminalDetach() })
}
func (c *tuiLifecycle) requestDetach() { c.update(func(s *lifecycleState) { s.detach = true }) }
func (c *tuiLifecycle) pump() {
	defer close(c.done)
	var stopSent, detachSent bool
	pending := make([]tui.ClientSignal, 0, 3)
	for {
		var output chan tui.ClientSignal
		var next tui.ClientSignal
		if len(pending) > 0 {
			output = c.signals
			next = pending[0]
		}
		select {
		case <-c.quit:
			s := c.snapshot()
			if s.stop && !stopSent {
				c.interrupt <- os.Interrupt
			}
			if s.detach && !detachSent {
				c.detach <- struct{}{}
			}
			return
		case output <- next:
			pending = pending[1:]
		case sig := <-c.osSignals:
			typed := tui.SignalInterrupt
			if tuiTerminalSignal(sig) {
				typed = tui.SignalTerminalLost
				c.update(func(s *lifecycleState) {
					s.lost = true
					s.latchTerminalDetach()
				})
			}
			// The state machine needs at most two interrupts and one terminal loss.
			count := 0
			for _, p := range pending {
				if p == typed {
					count++
				}
			}
			limit := 2
			if typed == tui.SignalTerminalLost {
				limit = 1
			}
			if count < limit {
				pending = append(pending, typed)
			}
		case <-c.wake:
			s := c.snapshot()
			if s.stop && !stopSent {
				c.interrupt <- os.Interrupt
				stopSent = true
			}
			if s.detach && !detachSent {
				c.detach <- struct{}{}
				detachSent = true
			}
		}
	}
}
func (c *tuiLifecycle) config(stateDir, runID string, opts tuiOptions, mode tui.ClientMode) tui.Config {
	return tui.Config{RunID: runID, StateDir: stateDir, Caps: opts.caps, Tokens: opts.tokens, Mode: mode, Actions: wireActions(stateDir), Signals: c.signals,
		RequestStop: func() { c.update(func(s *lifecycleState) { s.stop = true }) }, Detach: c.requestDetach,
		OnExit: func(intent tui.ExitIntent) {
			c.update(func(s *lifecycleState) {
				if !s.hasIntent {
					s.hasIntent = true
					s.intent = intent
				}
			})
		},
		OnHeadlessAction: func(action tui.ActionCompletion) {
			c.update(func(s *lifecycleState) {
				if !s.hasAction {
					s.hasAction = true
					s.action = action
				}
			})
		}}
}
func expectedViewError(err error, s lifecycleState) error {
	if s.lost && (errors.Is(err, tea.ErrProgramKilled) || errors.Is(err, context.Canceled)) {
		return nil
	}
	return err
}
func tuiDetachQualified(ctx context.Context, stateDir, runID string) bool {
	snap, err := viewmodel.Load(ctx, stateDir, runID)
	return err == nil && snap.Admission != nil && snap.Admission.WorkerDetachment == "supported"
}

// tuiRecoveryHint binds the actual state root. Task 9 also consumes this
// formatting boundary for retained demo resources.
func tuiRecoveryHint(stateDir, runID string) string {
	if !filepath.IsAbs(stateDir) || !regexp.MustCompile(`^run_[A-Za-z0-9_]{1,128}$`).MatchString(runID) || strings.ContainsFunc(stateDir, unicode.IsControl) {
		return "recovery command unavailable: invalid state directory or run ID"
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	if runtime.GOOS == "windows" {
		quote = func(s string) string {
			return "'" + strings.NewReplacer("'", "''", "‘", "‘‘", "’", "’’", "‚", "‚‚", "‛", "‛‛").Replace(s) + "'"
		}
		return fmt.Sprintf("PowerShell: $env:MYTHHELM_HOME = %s; mythhelm recover %s", quote(stateDir), quote(runID))
	}
	return fmt.Sprintf("POSIX sh/bash/zsh: env %s mythhelm recover %s", quote("MYTHHELM_HOME="+stateDir), quote(runID))
}

// canonicalTUIOutcome reads run and latest-attempt projections after the owner
// and action have joined. It never opens a writable journal or retains old claims.
func canonicalTUIOutcome(ctx context.Context, stateDir, runID string) (supervisor.Outcome, error) {
	j, err := journal.OpenReadOnly(ctx, stateDir)
	if err != nil {
		return supervisor.Outcome{}, err
	}
	defer func() { _ = j.Close() }()
	run, err := j.Run(ctx, runID)
	if err != nil {
		return supervisor.Outcome{}, err
	}
	out := supervisor.Outcome{RunID: run.RunID, State: supervisor.RunState(run.State), Reason: run.Reason}
	attempt, err := j.LatestAttempt(ctx, runID)
	if errors.Is(err, journal.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return supervisor.Outcome{}, err
	}
	out.AttemptID, out.AttemptState, out.AttemptReason = attempt.AttemptID, supervisor.AttemptState(attempt.State), attempt.Reason
	return out, nil
}

func (d tuiDeps) finalOutcome(ctx context.Context, stateDir, runID string) (supervisor.Outcome, error) {
	read := d.canonicalOutcome
	if read == nil {
		read = canonicalTUIOutcome
	}
	out, err := read(ctx, stateDir, runID)
	if err != nil {
		// This is a client-only unavailable projection, never a journal state.
		return supervisor.Outcome{RunID: runID, State: "unknown"}, &outcomeError{code: ExitOwnership, category: categories[ExitOwnership], err: fmt.Errorf("canonical state unavailable: %w", err)}
	}
	return out, nil
}
