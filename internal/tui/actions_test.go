package tui

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/tui/caps"
	"github.com/turbokast/mythhelm/internal/tui/theme"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

// fakeActions records the injected supervisor calls task 9 dispatches,
// proving the seam: the TUI mutates only through Actions, never a second
// writer (I18). The *Err fields inject failures; StopState is the advisory
// state a Stop call reports.
type fakeActions struct {
	stops    int
	stopRun  string
	stopErr  error
	stopRet  string
	recovers int
	recOut   supervisor.RecoveryOutcome
	recErr   error
	applies  int
	applyRun string
	applyBr  string
	applyFl  bool
	applyUn  bool
	applyErr error
}

func (f *fakeActions) actions() Actions {
	return Actions{
		Stop: func(_ context.Context, runID string) (string, error) {
			f.stops++
			f.stopRun = runID
			return f.stopRet, f.stopErr
		},
		Recover: func(_ context.Context, runID string, _ supervisor.Hooks) (supervisor.RecoveryOutcome, error) {
			f.recovers++
			return f.recOut, f.recErr
		},
		Apply: func(_ context.Context, runID, branch string, acceptFlags, acceptUnverified bool) (supervisor.Receipt, error) {
			f.applies++
			f.applyRun, f.applyBr, f.applyFl, f.applyUn = runID, branch, acceptFlags, acceptUnverified
			return supervisor.Receipt{}, f.applyErr
		},
	}
}

// actionModel builds a sized model over snap with the given fake actions.
func actionModel(t *testing.T, c caps.Caps, snap viewmodel.Snapshot, acts Actions) *Model {
	t.Helper()
	cfg := testConfig(c, t)
	cfg.Actions = acts
	m := newModel(cfg)
	m.loadDiff = stubDiff(mustParseDiff(t, smallDiffInput))
	m.setSnapshot(t.Context(), snap)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	mm, ok := updated.(*Model)
	if !ok {
		t.Fatalf("Update returned %T, want *Model", updated)
	}
	return mm
}

// withState clones the rich snapshot into another run/attempt state.
func withState(run, attempt string) viewmodel.Snapshot {
	snap := richSnapshot()
	snap.Run.State = run
	snap.Attempt.State = attempt
	return snap
}

// openPaletteSelection opens the palette, types the query and presses Enter.
func openPaletteSelection(t *testing.T, m *Model, query string) *Model {
	t.Helper()
	m, _ = pressKey(t, m, runeMsg(":"))
	m = typeRunes(t, m, query)
	m, _ = pressKey(t, m, specialMsg(tea.KeyEnter))
	return m
}

// confirmDialog moves focus to the confirm button and presses Enter: the
// explicit confirm every dialog requires. It then runs the action command
// to completion and feeds the result back, as the Tea runtime would.
func confirmDialog(t *testing.T, m *Model) *Model {
	t.Helper()
	m, _ = pressKey(t, m, specialMsg(tea.KeyLeft))
	m, cmd := pressKey(t, m, specialMsg(tea.KeyEnter))
	return runActionCmd(t, m, cmd)
}

// runActionCmd executes an action command (nil when a gate refused) and
// feeds a resulting action message back through Update.
func runActionCmd(t *testing.T, m *Model, cmd tea.Cmd) *Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	if msg, ok := cmd().(actionResultMsg); ok {
		updated, _ := m.Update(msg)
		mm, ok := updated.(*Model)
		if !ok {
			t.Fatalf("Update returned %T, want *Model", updated)
		}
		return mm
	}
	return m
}

// TestStopLabelsRequested confirms stop renders stop requested — never
// stopped — until attempt.stopped arrives (I06), and that stop is disabled
// outside executing/verifying.
func TestStopLabelsRequested(t *testing.T) {
	t.Parallel()

	fakes := &fakeActions{stopRet: "stopping"}
	m := actionModel(t, fullCaps, withState("executing", "running"), fakes.actions())

	m = openPaletteSelection(t, m, "stop")
	if m.dialog != dialogStop {
		t.Fatalf("after Stop run dialog = %q, want stop", m.dialog)
	}
	for _, want := range []string{"Stop run", "run: run_t6", "[Request stop]", "[Cancel]"} {
		if view := m.View(); !strings.Contains(view, want) {
			t.Errorf("stop dialog lacks %q\n%s", want, view)
		}
	}
	m = confirmDialog(t, m)
	if fakes.stops != 1 || fakes.stopRun != "run_t6" {
		t.Fatalf("Stop calls = %d run %q, want 1 call for run_t6", fakes.stops, fakes.stopRun)
	}
	if m.dialog != "" {
		t.Fatalf("after confirm dialog = %q, want closed", m.dialog)
	}
	view := m.View()
	if !strings.Contains(view, "stop requested") {
		t.Errorf("view lacks %q after confirmed stop\n%s", "stop requested", view)
	}
	if strings.Contains(view, "stopped") {
		t.Errorf("view labels the request stopped before attempt.stopped\n%s", view)
	}

	// Only a proven confirmation retires the label: unconfirmed, unresolved,
	// failed-scan and malformed payloads keep it (I06).
	stopped := func(payload string) *Model {
		t.Helper()
		updated, _ := m.Update(eventMsg{Event: journal.Event{
			Type: "attempt.stopped", AttemptID: "att_1", RunSequence: 99,
			Payload: json.RawMessage(payload),
		}})
		mm, ok := updated.(*Model)
		if !ok {
			t.Fatalf("Update returned %T, want *Model", updated)
		}
		return mm
	}
	for _, tc := range []struct {
		name    string
		payload string
	}{
		{"unconfirmed", `{"confirmed":false,"unresolved_pids":[],"descendant_scan":"proc-environ"}`},
		{"unresolved", `{"confirmed":true,"unresolved_pids":[4242],"descendant_scan":"proc-environ"}`},
		{"scan failed", `{"confirmed":true,"unresolved_pids":[],"descendant_scan":"failed"}`},
		{"malformed", `{"confirmed":`},
		{"absent", ``},
	} {
		if view := stopped(tc.payload).View(); !strings.Contains(view, "stop requested") {
			t.Errorf("%s stopped event retired the label\n%s", tc.name, view)
		}
	}
	confirmed := `{"confirmed":true,"unresolved_pids":[],"descendant_scan":"proc-environ"}`
	if view := stopped(confirmed).View(); strings.Contains(view, "stop requested") {
		t.Errorf("stop requested survives a confirmed attempt.stopped\n%s", view)
	}

	for _, state := range []string{"ready_for_review", "interrupted", "failed", "completed"} {
		m := actionModel(t, fullCaps, withState(state, "succeeded_native"), fakes.actions())
		m, _ = pressKey(t, m, runeMsg(":"))
		view := m.View()
		if !strings.Contains(view, "Stop run (palette)") || !strings.Contains(view, "disabled") ||
			!strings.Contains(view, "needs executing or verifying") {
			t.Errorf("state %s: palette lacks the disabled stop reason\n%s", state, view)
		}
		m, _ = pressKey(t, m, specialMsg(tea.KeyEnter))
		if !m.palette.open || m.dialog != "" {
			t.Errorf("state %s: disabled stop acted: palette open=%v dialog=%q", state, m.palette.open, m.dialog)
		}
	}
	if fakes.stops != 1 {
		t.Errorf("Stop calls = %d, want exactly the 1 executing confirm", fakes.stops)
	}
}

// TestActionRunsOffLoop pins the async action contract: confirming returns a
// command while the dialog holds its working state, keys stay routed (Esc
// cannot unmake the in-flight call), and the result lands when fed back.
func TestActionRunsOffLoop(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	acts := Actions{
		Stop: func(ctx context.Context, runID string) (string, error) {
			select {
			case <-release:
				return "stopping", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
	}
	m := actionModel(t, fullCaps, withState("executing", "running"), acts)
	m = openPaletteSelection(t, m, "stop")
	m, _ = pressKey(t, m, specialMsg(tea.KeyLeft))
	m, cmd := pressKey(t, m, specialMsg(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("confirming stop returned no command")
	}
	// The call runs off-loop: the dialog holds working state, not a result.
	if view := m.View(); !strings.Contains(view, "Requesting stop...") {
		t.Errorf("working dialog lacks the working label\n%s", view)
	}
	if m.stopRequested {
		t.Error("stop requested before the call finished")
	}
	// Keys stay routed but inert: Esc cannot unmake the in-flight call.
	m, _ = pressKey(t, m, specialMsg(tea.KeyEsc))
	if m.dialog != dialogStop || m.working == "" {
		t.Errorf("Esc disturbed the in-flight call: dialog=%q working=%q", m.dialog, m.working)
	}
	// Quitting cancels the in-flight call and drops the working state.
	m, quit := pressKey(t, m, specialMsg(tea.KeyCtrlC))
	if quit == nil {
		t.Fatal("ctrl+c returned no quit command")
	}
	if m.working != "" || m.actCancel != nil {
		t.Errorf("quit left the call in flight: working=%q", m.working)
	}
	// The late result still lands cleanly: either the accepted stop or the
	// cancellation error in place, never a stuck working state.
	close(release)
	m = runActionCmd(t, m, cmd)
	if m.working != "" {
		t.Errorf("working survives the landed result: %q", m.working)
	}
	if !m.stopRequested && m.dialogErr == "" {
		t.Error("landed result neither requested the stop nor surfaced its error")
	}
}

// TestRecoverOnlyWhenInterrupted enables recover only in interrupted; every
// other state shows it disabled with the reason.
func TestRecoverOnlyWhenInterrupted(t *testing.T) {
	t.Parallel()

	fakes := &fakeActions{recOut: supervisor.RecoveryOutcome{
		Outcome: supervisor.Outcome{RunID: "run_t6", State: supervisor.RunState("recovering"), Reason: "reconciling"},
		Mode:    "reattached",
	}}
	m := actionModel(t, fullCaps, withState("interrupted", "interrupted"), fakes.actions())
	m = openPaletteSelection(t, m, "rec")
	if m.dialog != dialogRecover {
		t.Fatalf("after Recover run dialog = %q, want recover", m.dialog)
	}
	m = confirmDialog(t, m)
	if fakes.recovers != 1 {
		t.Fatalf("Recover calls = %d, want 1", fakes.recovers)
	}
	if view := m.View(); !strings.Contains(view, "reattached") {
		t.Errorf("recover result lacks the outcome\n%s", view)
	}

	for _, state := range []string{"executing", "verifying", "ready_for_review", "failed", "completed"} {
		m := actionModel(t, fullCaps, withState(state, "running"), fakes.actions())
		m, _ = pressKey(t, m, runeMsg(":"))
		view := m.View()
		if !strings.Contains(view, "Recover run (palette)") || !strings.Contains(view, "disabled") ||
			!strings.Contains(view, "needs interrupted") {
			t.Errorf("state %s: palette lacks the disabled recover reason\n%s", state, view)
		}
		m = typeRunes(t, m, "rec")
		m, _ = pressKey(t, m, specialMsg(tea.KeyEnter))
		if !m.palette.open || m.dialog != "" {
			t.Errorf("state %s: disabled recover acted: palette open=%v dialog=%q", state, m.palette.open, m.dialog)
		}
	}
	if fakes.recovers != 1 {
		t.Errorf("Recover calls = %d, want exactly the 1 interrupted confirm", fakes.recovers)
	}
}

// TestApplyShowsExactEffects shows run, candidate commit, target branch,
// acceptance toggles and verification state in the apply dialog, and calls
// Apply once with those exact values (I03).
func TestApplyShowsExactEffects(t *testing.T) {
	t.Parallel()

	fakes := &fakeActions{}
	m := actionModel(t, fullCaps, richSnapshot(), fakes.actions())
	m = openPaletteSelection(t, m, "appl")
	if m.dialog != dialogApply {
		t.Fatalf("after Apply candidate dialog = %q, want apply", m.dialog)
	}
	commit := strings.Repeat("c", 40)
	for _, want := range []string{"Apply candidate", "run: run_t6", "candidate: " + commit,
		"target branch:", "accept validation flags: off", "accept unverified: off",
		"verified " + commit[:7], "[Cancel]"} {
		if view := m.View(); !strings.Contains(view, want) {
			t.Errorf("apply dialog lacks %q\n%s", want, view)
		}
	}

	m = typeRunes(t, m, "release/v2")
	if view := m.View(); !strings.Contains(view, "[Apply to release/v2]") {
		t.Fatalf("apply button lacks the typed branch\n%s", view)
	}
	m, _ = pressKey(t, m, specialMsg(tea.KeyTab)) // flags field
	m, _ = pressKey(t, m, runeMsg(" "))           // flags on
	m, _ = pressKey(t, m, specialMsg(tea.KeyTab)) // unverified field
	m, _ = pressKey(t, m, specialMsg(tea.KeyTab)) // button row
	m = confirmDialog(t, m)                       // move to Apply, Enter
	if fakes.applies != 1 {
		t.Fatalf("Apply calls = %d, want 1", fakes.applies)
	}
	if fakes.applyRun != "run_t6" || fakes.applyBr != "release/v2" || !fakes.applyFl || fakes.applyUn {
		t.Errorf("Apply got (%q, %q, flags=%v, unverified=%v), want (run_t6, release/v2, true, false)",
			fakes.applyRun, fakes.applyBr, fakes.applyFl, fakes.applyUn)
	}
	if view := m.View(); !strings.Contains(view, "applied run_t6 to release/v2") {
		t.Errorf("apply result lacks the confirmation\n%s", view)
	}

	// An empty branch surfaces in place with no call made.
	m = actionModel(t, fullCaps, richSnapshot(), fakes.actions())
	m = openPaletteSelection(t, m, "appl")
	for i := 0; i < 3; i++ {
		m, _ = pressKey(t, m, specialMsg(tea.KeyTab))
	}
	m = confirmDialog(t, m)
	if fakes.applies != 1 {
		t.Errorf("empty-branch Apply calls = %d, want no new call", fakes.applies)
	}
	if m.dialog != dialogApply {
		t.Fatalf("after empty-branch confirm dialog = %q, want the apply dialog kept", m.dialog)
	}
	if view := m.View(); !strings.Contains(view, "target branch is required") {
		t.Errorf("empty-branch error missing\n%s", view)
	}
}

// TestSingleKeypressNeverConfirms presses one key into each fresh dialog and
// asserts no call is made; Esc cancels every dialog with no call (AC-3.3).
// The only confirm path is the explicit confirm: focus move plus Enter.
func TestSingleKeypressNeverConfirms(t *testing.T) {
	t.Parallel()

	open := map[string]func(*testing.T, *fakeActions) *Model{
		dialogStop: func(t *testing.T, f *fakeActions) *Model {
			return openPaletteSelection(t, actionModel(t, fullCaps, withState("executing", "running"), f.actions()), "stop")
		},
		dialogRecover: func(t *testing.T, f *fakeActions) *Model {
			return openPaletteSelection(t, actionModel(t, fullCaps, withState("interrupted", "interrupted"), f.actions()), "rec")
		},
		dialogApply: func(t *testing.T, f *fakeActions) *Model {
			return openPaletteSelection(t, actionModel(t, fullCaps, richSnapshot(), f.actions()), "appl")
		},
	}
	firstKeys := []tea.KeyMsg{
		specialMsg(tea.KeyEnter), runeMsg(" "), runeMsg("y"), runeMsg("Y"),
		specialMsg(tea.KeyLeft), specialMsg(tea.KeyRight), specialMsg(tea.KeyTab), runeMsg("q"),
	}
	for id, openDialog := range open {
		for _, key := range firstKeys {
			fakes := &fakeActions{recOut: supervisor.RecoveryOutcome{Mode: "reattached"}}
			m := openDialog(t, fakes)
			if m.dialog != id {
				t.Fatalf("dialog %s did not open for first-key %q", id, key.String())
			}
			_, _ = pressKey(t, m, key)
			if fakes.stops+fakes.recovers+fakes.applies != 0 {
				t.Errorf("dialog %s: first keypress %q made a call", id, key.String())
			}
		}
		// Esc cancels with no call.
		fakes := &fakeActions{recOut: supervisor.RecoveryOutcome{Mode: "reattached"}}
		m := openDialog(t, fakes)
		m, _ = pressKey(t, m, specialMsg(tea.KeyEsc))
		if m.dialog != "" {
			t.Errorf("dialog %s: Esc left %q open", id, m.dialog)
		}
		if fakes.stops+fakes.recovers+fakes.applies != 0 {
			t.Errorf("dialog %s: Esc made a call", id)
		}
		// Esc cancels the exit-options and result prompts too.
		m = actionModel(t, fullCaps, richSnapshot(), (&fakeActions{}).actions())
		m, _ = pressKey(t, m, runeMsg("q"))
		m, _ = pressKey(t, m, specialMsg(tea.KeyEsc))
		if m.dialog != "" {
			t.Errorf("Esc left %q open", m.dialog)
		}
		// The explicit confirm still works: focus move plus Enter.
		fakes = &fakeActions{}
		m = openPaletteSelection(t, actionModel(t, fullCaps, withState("executing", "running"), fakes.actions()), "stop")
		m = confirmDialog(t, m)
		if fakes.stops != 1 {
			t.Errorf("explicit confirm made %d Stop calls, want 1", fakes.stops)
		}
		if m.dialog != "" {
			t.Errorf("after the explicit confirm dialog = %q, want closed", m.dialog)
		}
	}
}

// TestExportWritesStateDir writes the focused pane's plain text to
// <stateDir>/exports/<run>-<pane>.txt and shows the path; nothing lands in
// any repository (AC-5.3).
func TestExportWritesStateDir(t *testing.T) {
	t.Parallel()

	snap := richSnapshot()
	repo := t.TempDir()
	snap.Attempt.WorkspacePath = repo
	m := actionModel(t, fullCaps, snap, (&fakeActions{}).actions())
	stateDir := m.cfg.StateDir

	m, _ = pressKey(t, m, specialMsg(tea.KeyEnter)) // focus the detail pane
	if m.focusPane != paneDetail {
		t.Fatalf("focus = %q, want detail", m.focusPane)
	}
	m = openPaletteSelection(t, m, "expo")
	if m.dialog != dialogDone {
		t.Fatalf("after Export view dialog = %q, want done", m.dialog)
	}
	wantPath := filepath.Join(stateDir, "exports", "run_t6-detail.txt")
	if view := m.View(); !strings.Contains(view, wantPath) {
		t.Errorf("export result lacks the path %q\n%s", wantPath, view)
	}
	raw, err := os.ReadFile(wantPath) //nolint:gosec // G304: the test reads the path it just exported
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, want := range []string{"SELECTED CHANGE", "new row", "verified"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("export lacks %q\n%s", want, raw)
		}
	}
	if strings.Contains(string(raw), "\x1b") {
		t.Errorf("export is not plain text\n%q", raw)
	}
	if entries, err := os.ReadDir(repo); err != nil || len(entries) != 0 {
		t.Errorf("repository gained %d entries, want none (err=%v)", len(entries), err)
	}

	// The tasks pane exports under its own name.
	m, _ = pressKey(t, m, specialMsg(tea.KeyEsc))
	m, _ = pressKey(t, m, specialMsg(tea.KeyTab)) // detail -> tasks
	if m.focusPane != paneTasks {
		t.Fatalf("focus = %q, want tasks", m.focusPane)
	}
	m = openPaletteSelection(t, m, "expo")
	if m.dialog != dialogDone {
		t.Fatalf("after the tasks export dialog = %q, want done", m.dialog)
	}
	tasksPath := filepath.Join(stateDir, "exports", "run_t6-tasks.txt")
	raw, err = os.ReadFile(tasksPath) //nolint:gosec // G304: the test reads the path it just exported
	if err != nil {
		t.Fatalf("ReadFile tasks: %v", err)
	}
	if !strings.Contains(string(raw), "TASKS") {
		t.Errorf("tasks export lacks its pane\n%s", raw)
	}
}

// TestActionErrorSurfaces renders a failing Stop in place with the run state
// unchanged; errors are never swallowed.
func TestActionErrorSurfaces(t *testing.T) {
	t.Parallel()

	fakes := &fakeActions{stopErr: errors.New("worker unreachable")}
	m := actionModel(t, fullCaps, withState("executing", "running"), fakes.actions())
	m = openPaletteSelection(t, m, "stop")
	m = confirmDialog(t, m)
	if fakes.stops != 1 {
		t.Fatalf("Stop calls = %d, want 1 attempted call", fakes.stops)
	}
	if m.dialog != dialogStop {
		t.Fatalf("after the error dialog = %q, want the stop dialog kept", m.dialog)
	}
	view := m.View()
	if !strings.Contains(view, "stop failed: worker unreachable") {
		t.Errorf("view lacks the in-place error\n%s", view)
	}
	if strings.Contains(view, "stop requested") {
		t.Errorf("failed stop renders requested\n%s", view)
	}
	if m.snap.Run.State != "executing" {
		t.Errorf("run state = %q, want executing", m.snap.Run.State)
	}
	m, _ = pressKey(t, m, specialMsg(tea.KeyEsc))
	if m.dialog != "" {
		t.Errorf("Esc left %q open", m.dialog)
	}
}

// TestRequiredControlsSurviveAdversarialTheme renders the approval prompt,
// spend warning and stop-state labels under a maximally adversarial token
// set and asserts their text survives (G08, I11). The set fails Validate —
// it could never load — yet the controls stay legible because they never
// depend on colour alone.
func TestRequiredControlsSurviveAdversarialTheme(t *testing.T) {
	t.Parallel()

	adv := theme.Tokens{
		Surface: "#000000", SurfaceRaised: "#000000",
		Text: "#000000", TextMuted: "#000000",
		Focus: "bogus", Attention: "", OK: "#000000", Warning: "#00", Err: "#000000",
		Border: "wavy",
	}
	if err := adv.Validate(); err == nil {
		t.Fatal("adversarial tokens Validate clean; the test proves nothing")
	}
	advModel := func(snap viewmodel.Snapshot) *Model {
		cfg := testConfig(fullCaps, t)
		cfg.Tokens = adv
		cfg.Actions = (&fakeActions{stopRet: "stopping"}).actions()
		m := newModel(cfg)
		m.loadDiff = stubDiff(mustParseDiff(t, smallDiffInput))
		m.setSnapshot(t.Context(), snap)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
		mm, ok := updated.(*Model)
		if !ok {
			t.Fatalf("Update returned %T, want *Model", updated)
		}
		return mm
	}
	m := advModel(withState("executing", "running"))

	m = openPaletteSelection(t, m, "stop")
	m = confirmDialog(t, m)
	view := m.View()
	for _, want := range []string{
		"stop requested",          // stop-state label
		"paid continuation: off",  // spend warning surface
		"Billing: local-scripted", // spend kind label
	} {
		if !strings.Contains(view, want) {
			t.Errorf("adversarial view lacks %q\n%s", want, view)
		}
		if !strings.Contains(stripANSI(view), want) {
			t.Errorf("adversarial stripped view lacks %q\n%s", want, stripANSI(view))
		}
	}

	m = openPaletteSelection(t, advModel(richSnapshot()), "appl")
	if m.dialog != dialogApply {
		t.Fatalf("after Apply candidate dialog = %q, want apply", m.dialog)
	}
	view = m.View()
	for _, want := range []string{
		"Apply candidate", "target branch:", "[Apply to", "[Cancel]", // approval prompt
	} {
		if !strings.Contains(view, want) {
			t.Errorf("adversarial approval prompt lacks %q\n%s", want, view)
		}
		if !strings.Contains(stripANSI(view), want) {
			t.Errorf("adversarial stripped approval prompt lacks %q\n%s", want, stripANSI(view))
		}
	}
}
