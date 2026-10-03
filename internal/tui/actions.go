package tui

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

// Actions carries the injected supervisor entry points the palette acts
// through (design §10). Tests supply fakes; production wiring (task 12)
// supplies closures over the same calls the CLI verbs use, so the TUI never
// grows a second writer (I18).
type Actions struct {
	Stop    func(ctx context.Context, runID string) (string, error)
	Recover func(ctx context.Context, runID string, h supervisor.Hooks) (supervisor.RecoveryOutcome, error)
	Apply   func(ctx context.Context, runID, branch string, acceptFlags, acceptUnverified bool) (supervisor.Receipt, error)
}

// actionGate reports whether a palette action is available for the current
// snapshot, with the reason when it is not. Stop runs in
// executing/verifying, recover only in interrupted, apply only in
// ready_for_review; the view-only actions are always available.
func (m *Model) actionGate(a Action) (bool, string) {
	switch a {
	case actionStop:
		return m.stopGate()
	case actionRecover:
		return m.recoverGate()
	case actionApply:
		return m.applyGate()
	case actionExport:
		if !m.ready {
			return false, "no snapshot loaded"
		}
		return true, ""
	default:
		return true, ""
	}
}

// stopGate enables stop in executing/verifying only.
func (m *Model) stopGate() (bool, string) {
	if !m.ready {
		return false, "no snapshot loaded"
	}
	switch m.snap.Run.State {
	case "executing", "verifying":
		return true, ""
	default:
		return false, "run is " + cellOrUnknown(m.snap.Run.State) + ", needs executing or verifying"
	}
}

// recoverGate enables recover in interrupted only.
func (m *Model) recoverGate() (bool, string) {
	if !m.ready {
		return false, "no snapshot loaded"
	}
	if m.snap.Run.State == "interrupted" {
		return true, ""
	}
	return false, "run is " + cellOrUnknown(m.snap.Run.State) + ", needs interrupted"
}

// applyGate enables apply in ready_for_review only.
func (m *Model) applyGate() (bool, string) {
	if !m.ready {
		return false, "no snapshot loaded"
	}
	if m.snap.Run.State == "ready_for_review" {
		return true, ""
	}
	return false, "run is " + cellOrUnknown(m.snap.Run.State) + ", needs ready_for_review"
}

// actionResultMsg carries a finished supervisor call back to the update
// loop. Actions run off-loop so the TUI keeps repainting while a call —
// recover reattaching to its worker, apply running git — is in flight; the
// call's context derives from the run context and dies with it (quit
// cancels an in-flight call). No invented timeout: the CLI runs these same
// entry points without one, and the TUI must not fail operations the CLI
// would complete.
type actionResultMsg struct {
	action      Action
	applyBranch string
	recoverOut  supervisor.RecoveryOutcome
	stopState   string
	err         error
}

// actionCtx derives one action's context from the run context, cancelling
// any previous in-flight call first (dialog prompts are modal, so at most
// one action runs at a time).
func (m *Model) actionCtx() context.Context {
	if m.actCancel != nil {
		m.actCancel()
	}
	base := m.runCtx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base)
	m.actCancel = cancel
	return ctx
}

// cancelAction releases an in-flight supervisor call, if any.
func (m *Model) cancelAction() {
	if m.actCancel != nil {
		m.actCancel()
		m.actCancel = nil
	}
	m.working = ""
}

// confirmStop runs the confirmed stop through the injected seam, off-loop.
// A "stopped" return means the stop was already confirmed, so nothing goes
// pending; otherwise the run renders stop requested until attempt.stopped is
// journaled — requested is never labelled stopped (I06). A failure surfaces
// in place with state unchanged.
func (m *Model) confirmStop() tea.Cmd {
	if m.cfg.Actions.Stop == nil {
		m.dialogErr = "stop unavailable: no supervisor seam wired"
		return nil
	}
	if ok, reason := m.stopGate(); !ok {
		m.dialogErr = "stop disabled: " + reason
		return nil
	}
	stop, runID, ctx := m.cfg.Actions.Stop, m.snap.Run.RunID, m.actionCtx()
	m.stopConfirmed = false
	m.working = "Requesting stop..."
	return func() tea.Msg {
		state, err := stop(ctx, runID)
		return actionResultMsg{action: actionStop, stopState: state, err: err}
	}
}

// confirmRecover runs the confirmed recover through the injected seam with
// LiveFeed hooks, off-loop, so recovery progress feeds the live view
// through the poll tick (design §10): journaled events reload the snapshot
// and notices drain into notice lines. Success reports the outcome;
// failure surfaces in place.
func (m *Model) confirmRecover() tea.Cmd {
	if m.cfg.Actions.Recover == nil {
		m.dialogErr = "recover unavailable: no supervisor seam wired"
		return nil
	}
	if ok, reason := m.recoverGate(); !ok {
		m.dialogErr = "recover disabled: " + reason
		return nil
	}
	doRecover, runID, ctx := m.cfg.Actions.Recover, m.snap.Run.RunID, m.actionCtx()
	events, notices, hooks := LiveFeed()
	m.recoverEvents = events
	m.recoverNotices = notices
	m.working = "Recovering run..."
	return func() tea.Msg {
		out, err := doRecover(ctx, runID, hooks)
		return actionResultMsg{action: actionRecover, recoverOut: out, err: err}
	}
}

// confirmApply runs the confirmed apply with the dialog's exact branch and
// toggles (I03), off-loop. An empty branch surfaces in place with no call
// made; any failure surfaces in place with state unchanged.
func (m *Model) confirmApply() tea.Cmd {
	if m.cfg.Actions.Apply == nil {
		m.dialogErr = "apply unavailable: no supervisor seam wired"
		return nil
	}
	if ok, reason := m.applyGate(); !ok {
		m.dialogErr = "apply disabled: " + reason
		return nil
	}
	if m.applyBranch == "" {
		m.dialogErr = "target branch is required"
		return nil
	}
	apply, runID, branch, flags, unverified, ctx :=
		m.cfg.Actions.Apply, m.snap.Run.RunID, m.applyBranch, m.acceptFlags, m.acceptUnverified, m.actionCtx()
	m.working = "Applying candidate..."
	return func() tea.Msg {
		_, err := apply(ctx, runID, branch, flags, unverified)
		return actionResultMsg{action: actionApply, applyBranch: branch, err: err}
	}
}

// applyActionResult lands a finished supervisor call: errors surface in the
// open dialog, successes close it or report through the result prompt.
func (m *Model) applyActionResult(msg actionResultMsg) {
	m.working = ""
	m.actCancel = nil
	switch msg.action {
	case actionStop:
		if msg.err != nil {
			m.dialogErr = "stop failed: " + msg.err.Error()
			return
		}
		// An already-confirmed stop ("stopped") leaves nothing pending:
		// the label would lie about a request still outstanding (I06).
		// A confirmation that arrived while the call ran also suppresses
		// the pending label; either way the retained flag is consumed.
		if msg.stopState != "stopped" && !m.stopConfirmed {
			m.stopRequested = true
		}
		m.stopConfirmed = false
		m.closeDialog()
	case actionRecover:
		m.finishRecoverFeed()
		if msg.err != nil {
			m.dialogErr = "recover failed: " + msg.err.Error()
			return
		}
		out := msg.recoverOut
		m.showResult("Recover run", "recovery "+cellOrUnknown(out.Mode)+": run "+
			cellOrUnknown(string(out.State))+" ("+cellOrUnknown(out.Reason)+")")
	case actionApply:
		if msg.err != nil {
			m.dialogErr = "apply failed: " + msg.err.Error()
			return
		}
		m.showResult("Apply candidate", "applied "+m.snap.Run.RunID+" to "+msg.applyBranch)
	}
}

// runExport writes the focused pane's plain text to
// <stateDir>/exports/<run>-<pane>.txt (D8: deterministic, outside any
// repository) and shows the path. Failures surface in a result dialog.
func (m *Model) runExport() {
	if !m.ready {
		m.showResult("Export failed", "no snapshot loaded")
		return
	}
	if m.cfg.StateDir == "" {
		m.showResult("Export failed", "no state dir configured")
		return
	}
	dir := filepath.Join(m.cfg.StateDir, "exports")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		m.showResult("Export failed", err.Error())
		return
	}
	path := filepath.Join(dir, sanitizeSegment(m.snap.Run.RunID)+"-"+m.focusPane+".txt")
	if err := os.WriteFile(path, []byte(m.exportText()), 0o600); err != nil {
		m.showResult("Export failed", err.Error())
		return
	}
	m.showResult("Export complete", path)
}

// exportText is the focused pane's full plain-text content: the same rows
// the view shows, stripped of styling sequences, one per line.
func (m *Model) exportText() string {
	width := m.width
	if width < 1 {
		width = 80
	}
	rows := m.focusedPaneLines(width)
	out := make([]string, 0, len(rows))
	for _, ln := range rows {
		out = append(out, stripANSI(ln.text))
	}
	return strings.Join(out, "\n") + "\n"
}

// ansiRE matches the SGR styling sequences the renderers emit.
var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]")

// stripANSI removes styling sequences from rendered text.
func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

// sanitizeSegment maps a run id to a safe filename segment: anything
// outside [A-Za-z0-9_.-] becomes "_", so the export path can never escape
// the exports directory.
func sanitizeSegment(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '_', r == '.', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "run"
	}
	return b.String()
}
