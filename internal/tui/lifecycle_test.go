package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

func TestExitModeTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		mode                     ClientMode
		phase, attempt           string
		stop, detach             bool
		stopReason, detachReason string
		rows                     string
	}{
		{ModeLive, "created", "", true, false, "", "no worker yet", "request stop"},
		{ModeLive, "admission", "", true, false, "", "no worker yet", "request stop"},
		{ModeLive, "executing", "pending", true, false, "", "waiting for the worker", "request stop"},
		{ModeLive, "executing", "running", true, true, "", "", "detach (leave running)"},
		{ModeLive, "stopping", "stop_requested", false, true, "stop already requested", "", "detach (leave running)"},
		{ModeLive, "verifying", "succeeded_native", false, false, "verifying in progress", "verifying in progress", "back"},
		{ModeLive, "applying", "succeeded_native", false, false, "applying in progress", "applying in progress", "back"},
		{ModeLive, "ready_for_review", "succeeded_native", false, false, "needs executing", "nothing to detach", "quit"},
		{ModeLive, "completed", "succeeded_native", false, false, "needs executing", "nothing to detach", "quit"},
		{ModeLive, "blocked", "", false, false, "needs executing", "nothing to detach", "quit"},
		{ModeLive, "failed", "failed_native", false, false, "needs executing", "nothing to detach", "quit"},
		{ModeLive, "cancelled", "stopped", false, false, "needs executing", "nothing to detach", "quit"},
		{ModeLive, "interrupted", "running", false, false, "needs executing", "nothing to detach", "quit"},
	}
	for _, phase := range []string{"created", "admission", "executing", "stopping", "verifying", "applying", "ready_for_review", "completed", "blocked", "failed", "cancelled", "interrupted"} {
		reason := "needs executing"
		if phase == "executing" {
			reason = ""
		}
		if phase == "stopping" {
			reason = "stop already requested"
		}
		if phase == "verifying" || phase == "applying" {
			reason = phase + " in progress"
		}
		cases = append(cases, struct {
			mode                     ClientMode
			phase, attempt           string
			stop, detach             bool
			stopReason, detachReason string
			rows                     string
		}{ModeInspect, phase, "running", phase == "executing", false, reason, "this view owns no run", "quit"})
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%d/%s/%s", tc.mode, tc.phase, tc.attempt), func(t *testing.T) {
			t.Parallel()
			makeModel := func() *Model { return lifecycleModel(t, tc.mode, tc.phase, tc.attempt) }
			m := makeModel()
			for _, gate := range []struct {
				id      Action
				enabled bool
				reason  string
			}{{actionStop, tc.stop, tc.stopReason}, {actionDetach, tc.detach, tc.detachReason}} {
				ok, reason := m.actionGate(gate.id)
				if ok != gate.enabled || !strings.Contains(reason, gate.reason) {
					t.Fatalf("action %d = %v %q", gate.id, ok, reason)
				}
				if !ok {
					if !strings.Contains(joinLines(m.helpLines()), reason) || !strings.Contains(joinLines(m.paletteLines()), reason) {
						t.Fatalf("reason not shared: %q", reason)
					}
				}
			}
			m, _ = pressKey(t, m, runeMsg("q"))
			if !strings.Contains(m.View(), tc.rows) || !strings.Contains(m.View(), focusGlyph(m.cfg.Caps)+" back") {
				t.Fatalf("q rows/focus: %s", m.View())
			}
			m, cmd := pressKey(t, m, specialMsg(tea.KeyEnter))
			if cmd != nil || m.dialog != "" {
				t.Fatal("Enter on back has effect")
			}
			for _, key := range []string{"a", "s", "d", "r", "e"} {
				m = makeModel()
				m.cfg.StateDir = t.TempDir()
				id, want := actionExport, dialogDone
				switch key {
				case "a":
					id, want = actionApply, dialogApply
				case "s":
					id, want = actionStop, dialogStop
				case "d":
					id, want = actionDetach, dialogDetach
				case "r":
					id, want = actionRecover, dialogRecover
				}
				enabled, _ := m.actionGate(id)
				m, cmd = pressKey(t, m, runeMsg(key))
				if cmd != nil {
					t.Fatal("one letter performed a destructive action")
				}
				if enabled && m.dialog != want {
					t.Fatalf("key %s dialog=%q want %q", key, m.dialog, want)
				}
				if !enabled && (m.dialog != "" || noticeText(m) == "") {
					t.Fatalf("disabled key %s lacks reason", key)
				}
				if enabled && key != "e" && m.dialogFocus != buttonCancel {
					t.Fatalf("%s did not start on Cancel", key)
				}
			}
		})
	}
	for _, capability := range []string{"unknown", "unsupported", "supported"} {
		t.Run("capability/"+capability, func(t *testing.T) {
			t.Parallel()
			m := lifecycleModel(t, ModeLive, "executing", "running")
			m.snap.Admission.WorkerDetachment = capability
			ok, reason := m.detachGate()
			if ok != (capability == "supported") {
				t.Fatalf("detach %q = %v", capability, ok)
			}
			if !ok && reason != "detach is not qualified on this platform" {
				t.Fatalf("reason=%q", reason)
			}
		})
	}
	for _, missing := range []string{"snapshot", "admission", "worker", "stop control", "detach control", "inspect stop", "recover", "apply"} {
		t.Run("missing/"+missing, func(t *testing.T) {
			t.Parallel()
			m := lifecycleModel(t, ModeLive, "executing", "running")
			id := actionDetach
			switch missing {
			case "snapshot":
				m.ready = false
				id = actionStop
			case "admission":
				m.snap.Admission = nil
			case "worker":
				m.snap.Attempt = nil
			case "stop control":
				m.cfg.RequestStop = nil
				id = actionStop
			case "detach control":
				m.cfg.Detach = nil
			case "inspect stop":
				m.cfg.Mode = ModeInspect
				m.cfg.Actions.Stop = nil
				id = actionStop
			case "recover":
				m.snap.Run.State = "interrupted"
				m.cfg.Actions.Recover = nil
				id = actionRecover
			case "apply":
				m.snap.Run.State = "ready_for_review"
				m.cfg.Actions.Apply = nil
				id = actionApply
			}
			if ok, reason := m.actionGate(id); ok || reason == "" {
				t.Fatalf("missing %s = %v %q", missing, ok, reason)
			}
		})
	}
	for _, key := range []tea.KeyMsg{specialMsg(tea.KeyUp), specialMsg(tea.KeyShiftTab), runeMsg("k")} {
		t.Run("select/"+key.String(), func(t *testing.T) {
			t.Parallel()
			m := lifecycleModel(t, ModeLive, "executing", "running")
			m, _ = pressKey(t, m, runeMsg("q"))
			m, _ = pressKey(t, m, key)
			m, cmd := pressKey(t, m, specialMsg(tea.KeyEnter))
			if cmd != nil || m.dialog != dialogStop {
				t.Fatal("navigation did not select stop confirmation")
			}
		})
	}
	for _, key := range []tea.KeyMsg{specialMsg(tea.KeyDown), specialMsg(tea.KeyTab), runeMsg("j")} {
		t.Run("detach/"+key.String(), func(t *testing.T) {
			t.Parallel()
			m := lifecycleModel(t, ModeLive, "executing", "running")
			m, _ = pressKey(t, m, runeMsg("q"))
			m, _ = pressKey(t, m, key)
			m, cmd := pressKey(t, m, specialMsg(tea.KeyEnter))
			if cmd == nil || m.exitIntent.Kind != ExitDetach {
				t.Fatal("explicit detach selection did not quit")
			}
		})
	}
	// Confirmation rechecks the current snapshot, never the stale enabled row.
	m := lifecycleModel(t, ModeLive, "executing", "running")
	m, _ = pressKey(t, m, runeMsg("s"))
	m.snap.Run.State = "verifying"
	m, _ = pressKey(t, m, specialMsg(tea.KeyLeft))
	m, cmd := pressKey(t, m, specialMsg(tea.KeyEnter))
	if cmd != nil || !strings.Contains(m.View(), "verifying in progress") {
		t.Fatal("confirmation failed to recheck gate")
	}
}
func joinLines(lines []line) string {
	var s string
	for _, ln := range lines {
		s += ln.text + "\n"
	}
	return s
}

// I06/I18 (v2 §§2,15.2): only explicit controls affect an owned run;
// canonical phase transitions do not renew the client's interrupt allowance.
func TestLiveCtrlCIntents(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		phase, attempt string
		firstStop      bool
		second         ExitKind
		quits          bool
		notice         string
	}{
		{"created", "", true, ExitView, false, "waiting for admission to finish"},
		{"admission", "", true, ExitView, false, "waiting for admission to finish"},
		{"executing", "pending", true, ExitView, false, "waiting for the worker"},
		{"executing", "running", true, ExitDetach, true, "stop requested"},
		{"stopping", "running", false, ExitDetach, true, "stop already requested"},
		{"verifying", "succeeded_native", false, ExitFinishHeadless, true, "verifying in progress"},
		{"applying", "succeeded_native", false, ExitFinishHeadless, true, "applying in progress"},
		{"ready_for_review", "succeeded_native", false, ExitView, true, ""},
		{"interrupted", "running", false, ExitView, true, ""},
		{"completed", "succeeded_native", false, ExitView, true, ""},
	} {
		t.Run(tc.phase+"/"+tc.attempt, func(t *testing.T) {
			t.Parallel()
			m := lifecycleModel(t, ModeLive, tc.phase, tc.attempt)
			stops, detaches, exits := 0, 0, []ExitIntent{}
			m.cfg.RequestStop = func() { stops++ }
			m.cfg.Detach = func() { detaches++ }
			m.cfg.OnExit = func(x ExitIntent) { exits = append(exits, x) }
			m, cmd := pressKey(t, m, specialMsg(tea.KeyCtrlC))
			immediate := tc.phase == "ready_for_review" || tc.phase == "interrupted" || tc.phase == "completed"
			if (cmd != nil) != immediate {
				t.Fatalf("first quit %v want %v", cmd != nil, immediate)
			}
			if (stops == 1) != tc.firstStop {
				t.Fatalf("stops=%d", stops)
			}
			if tc.notice != "" && (tc.phase == "stopping" || tc.phase == "verifying" || tc.phase == "applying" || tc.attempt == "running") && !strings.Contains(noticeText(m), tc.notice) {
				t.Fatalf("missing notice %q: %s", tc.notice, noticeText(m))
			}
			m, cmd = pressKey(t, m, specialMsg(tea.KeyCtrlC))
			if !tc.quits && !strings.Contains(noticeText(m), tc.notice) {
				t.Fatalf("second missing notice %q: %s", tc.notice, noticeText(m))
			}
			if !immediate && (cmd != nil) != tc.quits {
				t.Fatalf("second quit=%v want %v", cmd != nil, tc.quits)
			}
			if tc.quits && (len(exits) != 1 || exits[0].Kind != tc.second || exits[0].Phase != tc.phase) {
				t.Fatalf("exits=%+v", exits)
			}
			if (detaches == 1) != (tc.quits && tc.second == ExitDetach) {
				t.Fatalf("detaches=%d", detaches)
			}
			if stops > 1 {
				t.Fatalf("duplicate stop=%d", stops)
			}
		})
	}
	for _, to := range []string{"executing", "stopping", "verifying", "applying", "completed"} {
		t.Run("admission to "+to, func(t *testing.T) {
			t.Parallel()
			m := lifecycleModel(t, ModeLive, "admission", "")
			stops, detaches := 0, 0
			m.cfg.RequestStop = func() { stops++ }
			m.cfg.Detach = func() { detaches++ }
			m, _ = pressKey(t, m, specialMsg(tea.KeyCtrlC))
			next := withState(to, "running")
			next.Admission = &viewmodel.Admission{WorkerDetachment: "supported"}
			m.setSnapshot(t.Context(), next)
			m, cmd := pressKey(t, m, specialMsg(tea.KeyCtrlC))
			if cmd == nil || stops != 1 || m.interrupts != 2 {
				t.Fatalf("transition %s: quit=%v stops=%d count=%d", to, cmd != nil, stops, m.interrupts)
			}
			if (detaches == 1) != (to == "executing" || to == "stopping") {
				t.Fatalf("detaches=%d", detaches)
			}
		})
	}
}

func lifecycleModel(t *testing.T, mode ClientMode, phase, attempt string) *Model {
	t.Helper()
	m := actionModel(t, fullCaps, withState(phase, attempt), (&fakeActions{}).actions())
	m.cfg.Mode = mode
	m.cfg.RequestStop = func() {}
	m.cfg.Detach = func() {}
	m.snap.Admission = &viewmodel.Admission{WorkerDetachment: "supported"}
	return m
}
func noticeText(m *Model) string {
	var out string
	for _, ln := range m.noticeRows(100) {
		out += ln.text + "\n"
	}
	return out
}

func TestInFlightActionDoubleQuit(t *testing.T) {
	t.Parallel()
	for _, first := range []string{"q", "ctrl+c"} {
		t.Run(first, func(t *testing.T) {
			t.Parallel()
			started, cancelled, cleanup := make(chan struct{}), make(chan struct{}), make(chan struct{})
			m := lifecycleModel(t, ModeInspect, "executing", "running")
			m.cfg.Actions.Stop = func(ctx context.Context, _ string) (string, error) {
				close(started)
				<-ctx.Done()
				close(cancelled)
				<-cleanup
				return "", ctx.Err()
			}
			m.openDialog(dialogStop)
			cmd := m.confirmStop()
			<-started
			m, _ = pressKey(t, m, keyNamed(first))
			if !strings.Contains(m.View(), "stop in progress; press Ctrl-C again to quit") {
				t.Fatalf("missing armed warning: %s", m.View())
			}
			select {
			case <-cancelled:
				t.Fatal("first quit cancelled action")
			default:
			}
			m, quit := pressKey(t, m, specialMsg(tea.KeyCtrlC))
			if quit == nil || m.exitIntent.Kind != ExitUnresolvedAction {
				t.Fatalf("quit=%v intent=%+v", quit != nil, m.exitIntent)
			}
			<-cancelled
			select {
			case <-m.action.done:
				t.Fatal("action abandoned cancellation cleanup")
			default:
			}
			close(cleanup)
			result := cmd().(actionResultMsg)
			if !errors.Is(result.err, context.Canceled) {
				t.Fatalf("result=%+v", result)
			}
		})
	}
}
func keyNamed(s string) tea.KeyMsg {
	if s == "ctrl+c" {
		return specialMsg(tea.KeyCtrlC)
	}
	return runeMsg(s)
}

// The view has no authority to abandon an approved effect (I03/I06/I18).
func TestTerminalLossJoinsApply(t *testing.T) {
	t.Parallel()
	for _, mode := range []ClientMode{ModeLive, ModeInspect} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			t.Parallel()
			m := lifecycleModel(t, mode, "ready_for_review", "succeeded_native")
			release, started := make(chan struct{}), make(chan context.Context, 1)
			var calls atomic.Int32
			m.cfg.Actions.Apply = func(ctx context.Context, _, _ string, _, _ bool) (supervisor.Receipt, error) {
				calls.Add(1)
				started <- ctx
				select {
				case <-release:
					return supervisor.Receipt{"run_id": "exact"}, nil
				case <-ctx.Done():
					return supervisor.Receipt{}, ctx.Err()
				}
			}
			m.openDialog(dialogApply)
			m.applyBranch = "approved"
			cmd := m.confirmApply()
			teaResult := make(chan tea.Msg, 1)
			go func() { teaResult <- cmd() }()
			actCtx := <-started
			signals := make(chan ClientSignal, 32)
			m.cfg.Signals = signals
			completed := make(chan ActionCompletion, 2)
			m.cfg.OnHeadlessAction = func(x ActionCompletion) { completed <- x }
			exited := make(chan ExitIntent, 2)
			m.cfg.OnExit = func(x ExitIntent) { exited <- x }
			result := launchTestProgram(t, m)
			signals <- SignalTerminalLost
			awaitClosed(t, m.viewDone)
			if actCtx.Err() != nil {
				t.Fatalf("terminal loss cancelled approved action: %v", actCtx.Err())
			}
			select {
			case err := <-result:
				t.Fatalf("Run returned before action joined: %v", err)
			default:
			}
			close(release)
			if err := awaitResult(t, result); err != nil {
				t.Fatal(err)
			}
			out := <-completed
			if out.Kind != "apply" || out.Receipt["run_id"] != "exact" || out.Err != nil || calls.Load() != 1 {
				t.Fatalf("completion=%+v calls=%d", out, calls.Load())
			}
			if (<-exited).Kind != ExitFinishHeadless {
				t.Fatal("wrong exit intent")
			}
			select {
			case x := <-completed:
				t.Fatalf("duplicate completion %+v", x)
			default:
			}
			// The command and shutdown independently observe the exact same result.
			if got := (<-teaResult).(actionResultMsg); got.receipt["run_id"] != "exact" {
				t.Fatalf("Tea lost result %+v", got)
			}
		})
	}
}
func launchTestProgram(t *testing.T, m *Model) <-chan error {
	t.Helper()
	snap := m.snap
	m.loadSnapshot = func(context.Context, string, string) (viewmodel.Snapshot, error) { return snap, nil }
	result := make(chan error, 1)
	go func() {
		result <- runProgram(t.Context(), m, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer())
	}()
	return result
}
func awaitClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("lifecycle did not reach completion")
	}
}
func awaitResult(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("program did not join")
		return nil
	}
}

func TestExplicitActionQuitCancelsAndJoins(t *testing.T) {
	t.Parallel()
	for _, parentCancel := range []bool{false, true} {
		t.Run(fmt.Sprint(parentCancel), func(t *testing.T) {
			t.Parallel()
			m := lifecycleModel(t, ModeInspect, "interrupted", "running")
			started, cancelled, cleanup := make(chan struct{}), make(chan struct{}), make(chan struct{})
			m.cfg.Actions.Recover = func(ctx context.Context, _ string, _ supervisor.Hooks) (supervisor.RecoveryOutcome, error) {
				close(started)
				<-ctx.Done()
				close(cancelled)
				<-cleanup
				return supervisor.RecoveryOutcome{}, ctx.Err()
			}
			m.openDialog(dialogRecover)
			cmd := m.confirmRecover()
			<-started
			signals := make(chan ClientSignal, 2)
			m.cfg.Signals = signals
			callback := make(chan ExitIntent, 2)
			m.cfg.OnExit = func(x ExitIntent) { callback <- x }
			headless := make(chan ActionCompletion, 1)
			m.cfg.OnHeadlessAction = func(x ActionCompletion) { headless <- x }
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				result <- runProgram(ctx, m, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer())
			}()
			if parentCancel {
				cancel()
			} else {
				signals <- SignalInterrupt
				signals <- SignalInterrupt
			}
			awaitClosed(t, cancelled)
			awaitClosed(t, m.viewDone)
			select {
			case err := <-result:
				t.Fatalf("returned without cancellation cleanup: %v", err)
			default:
			}
			close(cleanup)
			if err := awaitResult(t, result); err != nil {
				t.Fatal(err)
			}
			if x := <-callback; x.Kind != ExitUnresolvedAction {
				t.Fatalf("exit=%+v", x)
			}
			if got := cmd().(actionResultMsg); !errors.Is(got.err, context.Canceled) {
				t.Fatalf("result=%+v", got)
			}
			select {
			case x := <-headless:
				t.Fatalf("deliberate cancellation mislabelled terminal loss: %+v", x)
			default:
			}
		})
	}
}

func TestHeadlessStopCompletionState(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"stopped", "stop_requested", "", "unrecognized"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			m := lifecycleModel(t, ModeInspect, "executing", "running")
			started, release := make(chan struct{}), make(chan struct{})
			m.cfg.Actions.Stop = func(context.Context, string) (string, error) { close(started); <-release; return state, nil }
			m.openDialog(dialogStop)
			cmd := m.confirmStop()
			<-started
			signals := make(chan ClientSignal, 1)
			m.cfg.Signals = signals
			completed := make(chan ActionCompletion, 1)
			m.cfg.OnHeadlessAction = func(x ActionCompletion) { completed <- x }
			result := launchTestProgram(t, m)
			signals <- SignalTerminalLost
			awaitClosed(t, m.viewDone)
			close(release)
			if err := awaitResult(t, result); err != nil {
				t.Fatal(err)
			}
			out := <-completed
			if out.Kind != "stop" || out.StopState != state || out.Err != nil {
				t.Fatalf("completion=%+v", out)
			}
			m.applyActionResult(cmd().(actionResultMsg))
			m.exiting = false
			if m.stopRequested != (state != "stopped") {
				t.Fatalf("state=%q pending=%v", state, m.stopRequested)
			}
			if (m.viewExit() != nil) && m.exitIntent.Kind == ExitView && state != "stopped" {
				t.Fatal("unconfirmed stop reported successful view exit")
			}
		})
	}
}

func TestLifecyclePumpsJoin(t *testing.T) {
	t.Parallel()
	m := lifecycleModel(t, ModeLive, "executing", "running")
	signals := make(chan ClientSignal, 256)
	events := make(chan journal.Event, 256)
	m.cfg.Signals = signals
	m.cfg.Events = events
	var stops, detaches, exits atomic.Int32
	m.cfg.RequestStop = func() { stops.Add(1) }
	m.cfg.Detach = func() { detaches.Add(1) }
	m.cfg.OnExit = func(ExitIntent) { exits.Add(1) }
	for range 256 {
		signals <- SignalInterrupt
		events <- journal.Event{Type: "unknown"}
	}
	result := launchTestProgram(t, m)
	if err := awaitResult(t, result); err != nil {
		t.Fatal(err)
	}
	awaitClosed(t, m.viewDone)
	if stops.Load() != 1 || detaches.Load() != 1 || exits.Load() != 1 {
		t.Fatalf("callbacks stop=%d detach=%d exit=%d", stops.Load(), detaches.Load(), exits.Load())
	}
	if m.liveDone != nil || m.liveQuit != nil {
		t.Fatal("live pump ownership survives Run")
	}
}

func TestTerminalLossModeTable(t *testing.T) {
	t.Parallel()
	for _, mode := range []ClientMode{ModeLive, ModeInspect} {
		for _, phase := range []string{"created", "admission", "executing", "stopping", "verifying", "applying", "ready_for_review", "completed", "blocked", "failed", "cancelled", "interrupted"} {
			t.Run(fmt.Sprintf("%d/%s", mode, phase), func(t *testing.T) {
				t.Parallel()
				m := lifecycleModel(t, mode, phase, "pending")
				var stops, detaches, exits int
				m.cfg.RequestStop = func() { stops++ }
				m.cfg.Detach = func() { detaches++ }
				m.cfg.OnExit = func(ExitIntent) { exits++ }
				_, cmd := m.Update(SignalTerminalLost)
				want := ExitFinishHeadless
				if mode == ModeLive && (phase == "created" || phase == "admission" || phase == "executing" || phase == "stopping") {
					want = ExitDetach
				}
				if cmd == nil || m.exitIntent.Kind != want || !m.terminalLoss || stops != 0 || exits != 1 {
					t.Fatalf("intent=%+v stops=%d exits=%d", m.exitIntent, stops, exits)
				}
				if (detaches == 1) != (want == ExitDetach) {
					t.Fatalf("detaches=%d", detaches)
				}
				m.Update(SignalTerminalLost)
				m.Update(SignalInterrupt)
				if exits != 1 || detaches > 1 || stops != 0 {
					t.Fatal("duplicate lifecycle callback")
				}
			})
		}
	}
}

func TestExitModeInspectCtrlC(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"created", "admission", "executing", "stopping", "verifying", "applying", "ready_for_review", "completed", "interrupted"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			m := lifecycleModel(t, ModeInspect, phase, "running")
			_, cmd := m.Update(SignalInterrupt)
			if cmd == nil || m.exitIntent.Kind != ExitView || m.stopSent || m.detachSent {
				t.Fatalf("intent=%+v stop=%v detach=%v", m.exitIntent, m.stopSent, m.detachSent)
			}
		})
	}
}

func TestLifecycleEOFIsNotTerminalLoss(t *testing.T) {
	t.Parallel()
	m := lifecycleModel(t, ModeInspect, "completed", "succeeded_native")
	signals := make(chan ClientSignal, 1)
	m.cfg.Signals = signals
	poll := make(chan struct{}, 1)
	snap := m.snap
	m.loadSnapshot = func(context.Context, string, string) (viewmodel.Snapshot, error) {
		select {
		case poll <- struct{}{}:
		default:
		}
		return snap, nil
	}
	result := make(chan error, 1)
	go func() {
		result <- runProgram(t.Context(), m, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard), tea.WithoutRenderer())
	}()
	select {
	case <-poll:
	case <-time.After(5 * time.Second):
		t.Fatal("no bootstrap poll")
	}
	select {
	case err := <-result:
		t.Fatalf("EOF ended view: %v", err)
	default:
	}
	signals <- SignalInterrupt
	if err := awaitResult(t, result); err != nil {
		t.Fatal(err)
	}
	if m.terminalLoss || m.exitIntent.Kind != ExitView {
		t.Fatalf("EOF treated as terminal loss: %+v", m.exitIntent)
	}
}

func TestTerminalLossThenParentCancellationJoinsAction(t *testing.T) {
	t.Parallel()
	for _, mode := range []ClientMode{ModeInspect, ModeLive} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			t.Parallel()
			m := lifecycleModel(t, mode, "ready_for_review", "succeeded_native")
			started := make(chan context.Context, 1)
			cancelled, cleanup, fallback := make(chan struct{}), make(chan struct{}), make(chan struct{})
			m.cfg.Actions.Apply = func(ctx context.Context, _, _ string, _, _ bool) (supervisor.Receipt, error) {
				started <- ctx
				select {
				case <-ctx.Done():
					close(cancelled)
					<-cleanup
					return nil, ctx.Err()
				case <-fallback:
					return supervisor.Receipt{"run_id": "finished"}, nil
				}
			}
			m.openDialog(dialogApply)
			m.applyBranch = "approved"
			cmd := m.confirmApply()
			actCtx := <-started
			signals := make(chan ClientSignal, 1)
			m.cfg.Signals = signals
			completed := make(chan ActionCompletion, 2)
			m.cfg.OnHeadlessAction = func(x ActionCompletion) { completed <- x }
			exits := make(chan ExitIntent, 2)
			m.cfg.OnExit = func(x ExitIntent) { exits <- x }
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				result <- runProgram(ctx, m, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer())
			}()
			signals <- SignalTerminalLost
			awaitClosed(t, m.viewDone)
			preserved := actCtx.Err() == nil
			cancel()
			observed := false
			select {
			case <-cancelled:
				observed = true
			case <-time.After(250 * time.Millisecond):
			}
			earlyReturn := false
			select {
			case <-result:
				earlyReturn = true
			default:
			}
			// Always release and join, including the broken input that ignores parent
			// cancellation, so failure cannot leave a test-owned operation behind.
			close(cleanup)
			close(fallback)
			if !earlyReturn {
				if err := awaitResult(t, result); err != nil {
					t.Fatal(err)
				}
			}
			actual := cmd().(actionResultMsg)
			out := <-completed
			if !preserved {
				t.Error("terminal loss itself cancelled the approved action")
			}
			if !observed {
				t.Error("parent cancellation after terminal loss never reached action context")
			}
			if earlyReturn {
				t.Error("Run returned before action cancellation cleanup joined")
			}
			if !errors.Is(actual.err, context.Canceled) || !errors.Is(out.Err, context.Canceled) || out.Kind != "apply" {
				t.Errorf("actual=%+v completion=%+v", actual, out)
			}
			if exit := <-exits; exit.Kind != ExitFinishHeadless {
				t.Errorf("exit=%+v", exit)
			}
			if len(exits) != 0 || len(completed) != 0 {
				t.Error("duplicate exit or completion callback")
			}
		})
	}
}

func TestInspectStopErrorExitIntent(t *testing.T) {
	t.Parallel()
	for _, confirmation := range []string{"none", "before result", "after result", "interrupted", "created", "admission", "recovering", "applying", "unknown", "stopped snapshot", "foreign stopped", "malformed stopped"} {
		t.Run(confirmation, func(t *testing.T) {
			t.Parallel()
			m := lifecycleModel(t, ModeInspect, "executing", "running")
			m.cfg.Actions.Stop = func(context.Context, string) (string, error) { return "", supervisor.ErrOwnership }
			m.openDialog(dialogStop)
			cmd := m.confirmStop()
			confirmed := eventMsg{Event: journal.Event{Type: "attempt.stopped", AttemptID: m.snap.Attempt.AttemptID, Payload: json.RawMessage(`{"confirmed":true,"unresolved_pids":[],"descendant_scan":"proc-environ"}`)}}
			if confirmation == "before result" {
				m.Update(confirmed)
			}
			m.applyActionResult(cmd().(actionResultMsg))
			if !strings.Contains(m.View(), "stop failed:") {
				t.Fatalf("original error missing: %s", m.View())
			}
			if m.stopRequested {
				t.Fatal("request delivery invented from an error")
			}
			if confirmation == "after result" {
				m.Update(confirmed)
			}
			switch confirmation {
			case "interrupted", "created", "admission", "recovering", "applying", "unknown":
				snap := m.snap
				snap.Run.State = confirmation
				m.setSnapshot(t.Context(), snap)
				m.Update(eventMsg{Event: journal.Event{Type: "ui.notice"}})
			case "foreign stopped":
				confirmed.Event.AttemptID = "att_other"
				m.Update(confirmed)
			case "malformed stopped":
				confirmed.Event.Payload = json.RawMessage(`{"confirmed":true`)
				m.Update(confirmed)
			case "stopped snapshot":
				snap := m.snap
				attempt := *snap.Attempt
				attempt.State = "stopped"
				snap.Attempt = &attempt
				m.setSnapshot(t.Context(), snap)
			}
			m.closeDialog()
			m, quit := pressKey(t, m, specialMsg(tea.KeyCtrlC))
			want := ExitUnresolvedAction
			if confirmation == "before result" || confirmation == "after result" || confirmation == "stopped snapshot" {
				want = ExitView
			}
			if quit == nil || m.exitIntent.Kind != want {
				t.Fatalf("exit=%+v want %d", m.exitIntent, want)
			}
		})
	}
}

func TestInspectUnconfirmedStopSurvivesPoll(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"stop_requested", "", "unrecognized"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			m := lifecycleModel(t, ModeInspect, "executing", "running")
			m.cfg.Actions.Stop = func(context.Context, string) (string, error) { return state, nil }
			m.openDialog(dialogStop)
			result := m.confirmStop()().(actionResultMsg)
			m.applyActionResult(result)
			snap := m.snap
			snap.Run.State = "interrupted"
			m.setSnapshot(t.Context(), snap)
			m, quit := pressKey(t, m, specialMsg(tea.KeyCtrlC))
			if quit == nil || m.exitIntent.Kind != ExitUnresolvedAction {
				t.Fatalf("state=%q exit=%+v", state, m.exitIntent)
			}
		})
	}
}

func TestLiveStopRemainsRequestedBeforeAdmission(t *testing.T) {
	t.Parallel()
	m := lifecycleModel(t, ModeLive, "created", "")
	calls := 0
	m.cfg.RequestStop = func() { calls++ }
	m.openDialog(dialogStop)
	m.confirmStop()
	m.setSnapshot(t.Context(), m.snap)
	if ok, reason := m.stopGate(); ok || reason != "stop already requested" {
		t.Fatalf("gate=%v %q after accepted owner intent", ok, reason)
	}
	m.openDialog(dialogStop)
	m.confirmStop()
	if calls != 1 {
		t.Fatalf("stop intents=%d", calls)
	}
	m, _ = pressKey(t, m, specialMsg(tea.KeyCtrlC))
	if !strings.Contains(noticeText(m), "stop already requested") {
		t.Fatalf("missing prior request notice: %s", noticeText(m))
	}
}
