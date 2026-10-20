package tui

import (
	"context"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

// ClientMode identifies whether this view owns a running pipeline.
type ClientMode uint8

const (
	ModeInspect ClientMode = iota
	ModeLive
)

// ClientSignal is an input from the client's sole OS signal owner.
type ClientSignal uint8

const (
	SignalInterrupt ClientSignal = iota
	SignalTerminalLost
)

// ExitKind distinguishes closing a view from relinquishing live ownership.
type ExitKind uint8

const (
	ExitView ExitKind = iota
	ExitDetach
	ExitFinishHeadless
	ExitUnresolvedAction
)

// ExitIntent describes the client disposition without changing canonical state.
type ExitIntent struct {
	Kind  ExitKind
	Phase string
}

// ActionCompletion preserves the exact result of one confirmed operation.
type ActionCompletion struct {
	Kind      string
	StopState string
	Receipt   supervisor.Receipt
	Recovery  supervisor.RecoveryOutcome
	Err       error
}

// actionCall publishes result before closing done. Both the Tea command and
// shutdown may observe it; neither consumes the other's result.
type actionCall struct {
	done   chan struct{}
	result actionResultMsg
	cancel context.CancelFunc
}

func (m *Model) startAction(call func(context.Context) actionResultMsg) tea.Cmd {
	ctx := m.actionCtx()
	a := &actionCall{done: make(chan struct{}), cancel: m.actCancel}
	m.action = a
	go func() {
		defer close(a.done)
		defer a.cancel()
		a.result = call(ctx)
	}()
	return func() tea.Msg { <-a.done; return a.result }
}

func (m *Model) phase() string {
	if !m.ready {
		return "unknown"
	}
	return m.snap.Run.State
}
func (m *Model) workerRunning() bool {
	return m.snap.Attempt != nil && (m.snap.Attempt.State == "running" || m.snap.Attempt.State == "stop_requested")
}
func (m *Model) phaseBusy() bool { return m.phase() == "verifying" || m.phase() == "applying" }
func (m *Model) liveActive() bool {
	if m.cfg.Mode != ModeLive {
		return false
	}
	switch m.phase() {
	case "created", "admission", "executing", "stopping":
		return true
	}
	return false
}
func (m *Model) detachGate() (bool, string) {
	if m.cfg.Mode != ModeLive {
		return false, "this view owns no run"
	}
	if !m.ready {
		return false, "no snapshot loaded"
	}
	if m.phaseBusy() {
		return false, m.phase() + " in progress"
	}
	switch m.phase() {
	case "created", "admission":
		return false, "no worker yet"
	case "executing", "stopping":
		if !m.workerRunning() {
			return false, "waiting for the worker"
		}
	default:
		return false, "nothing to detach"
	}
	if m.snap.Admission == nil || m.snap.Admission.WorkerDetachment != "supported" {
		return false, "detach is not qualified on this platform"
	}
	if m.cfg.Detach == nil {
		return false, "detach unavailable: no owner control wired"
	}
	return true, ""
}
func (m *Model) lifecycleNotice(s string) { m.notices = append(m.notices, noticeEvent(s, time.Now())) }
func (m *Model) exit(kind ExitKind) tea.Cmd {
	if m.exiting {
		return nil
	}
	m.exiting = true
	m.exitIntent = ExitIntent{Kind: kind, Phase: m.phase()}
	if m.cfg.OnExit != nil {
		m.cfg.OnExit(m.exitIntent)
	}
	return tea.Quit
}
func (m *Model) viewExit() tea.Cmd {
	if m.stopRequested && m.cfg.Mode == ModeInspect {
		return m.exit(ExitUnresolvedAction)
	}
	return m.exit(ExitView)
}
func (m *Model) detach() tea.Cmd {
	if m.cfg.Detach == nil {
		m.lifecycleNotice("detach unavailable: no owner control wired")
		return nil
	}
	if !m.detachSent {
		m.detachSent = true
		m.cfg.Detach()
	}
	return m.exit(ExitDetach)
}
func (m *Model) requestLiveStop() {
	if m.stopSent || m.stopRequested {
		return
	}
	m.stopSent = true
	m.stopRequested = true
	m.cfg.RequestStop()
	m.lifecycleNotice("stop requested — waiting for worker confirmation")
}
func (m *Model) interrupt() tea.Cmd {
	if m.exiting {
		return nil
	}
	// Action arming is intentionally separate from the session interrupt count.
	m.interrupts++
	if m.working != "" {
		if !m.actionQuitArmed {
			m.armActionQuit()
			return nil
		}
		m.cancelAction()
		return m.exit(ExitUnresolvedAction)
	}
	if m.cfg.Mode == ModeInspect {
		return m.viewExit()
	}
	if m.phaseBusy() {
		if m.interrupts == 1 {
			m.lifecycleNotice(m.phase() + " in progress")
			return nil
		}
		m.lifecycleNotice("finishing " + m.phase() + "; the run continues without a view until it ends")
		return m.exit(ExitFinishHeadless)
	}
	if !m.ready {
		m.lifecycleNotice("no snapshot loaded")
		return nil
	}
	if !m.liveActive() {
		return m.viewExit()
	}
	if m.interrupts == 1 {
		if m.phase() == "stopping" || m.stopRequested {
			m.lifecycleNotice("stop already requested")
			return nil
		}
		if ok, reason := m.stopGate(); ok {
			m.requestLiveStop()
		} else {
			m.lifecycleNotice(reason)
		}
		return nil
	}
	if m.phase() == "created" || m.phase() == "admission" {
		m.lifecycleNotice("waiting for admission to finish")
		return nil
	}
	if !m.workerRunning() {
		m.lifecycleNotice("waiting for the worker")
		return nil
	}
	if m.snap.Admission == nil || m.snap.Admission.WorkerDetachment != "supported" {
		m.lifecycleNotice("not qualified on this platform")
	}
	return m.detach()
}
func (m *Model) armActionQuit() {
	m.actionQuitArmed = true
	m.dialogErr = stringsAction(m.dialog) + " in progress; press Ctrl-C again to quit"
}
func stringsAction(dialog string) string {
	switch dialog {
	case dialogStop:
		return "stop"
	case dialogRecover:
		return "recover"
	case dialogApply:
		return "apply"
	}
	return "action"
}
func (m *Model) terminalLost() tea.Cmd {
	if m.exiting {
		return nil
	}
	m.terminalLoss = true
	if m.working != "" {
		return m.exit(ExitFinishHeadless)
	}
	if m.cfg.Mode == ModeLive && m.liveActive() {
		// The owner latches this typed request until worker acceptance.
		if m.cfg.Detach != nil && !m.detachSent {
			m.detachSent = true
			m.cfg.Detach()
		}
		return m.exit(ExitDetach)
	}
	return m.exit(ExitFinishHeadless)
}

type parentCancelledMsg struct{}

// runProgram owns pumps until Tea has cancelled its Send context. Never join
// from Update: Send may be waiting synchronously for that very Update loop.
func runProgram(ctx context.Context, m *Model, options ...tea.ProgramOption) error {
	m.runCtx = ctx
	m.managedPoll = true
	options = append(options, tea.WithoutSignalHandler())
	p := tea.NewProgram(m, options...)
	quit := make(chan struct{})
	var pumps sync.WaitGroup
	pump := func(f func()) { pumps.Add(1); go func() { defer pumps.Done(); f() }() }
	if m.cfg.Events != nil {
		m.startLivePump(func(ev journal.Event) { p.Send(eventMsg{Event: ev}) })
	}
	pump(func() {
		ticker := time.NewTicker(livePollInterval)
		defer ticker.Stop()
		p.Send(pollTickMsg{})
		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				p.Send(pollTickMsg{})
			}
		}
	})
	pump(func() {
		signals := m.cfg.Signals
		for {
			select {
			case <-quit:
				return
			case <-ctx.Done():
				p.Send(parentCancelledMsg{})
				return
			case signal, ok := <-signals:
				if !ok {
					signals = nil
					continue
				}
				p.Send(signal)
			}
		}
	})
	_, err := p.Run()
	close(quit)
	pumps.Wait()
	m.stopLivePump()
	if m.action != nil && !m.terminalLoss {
		m.action.cancel()
	}
	if m.viewDone != nil {
		close(m.viewDone)
	}
	// Terminal loss preserves the same operation; every other unfinished
	// shutdown explicitly cancels it, then joins its cleanup.
	if m.action != nil {
		<-m.action.done
		if m.terminalLoss && m.cfg.OnHeadlessAction != nil {
			m.cfg.OnHeadlessAction(m.action.result.completion())
		}
	}
	return err
}

func (m *Model) directAction(key string) tea.Cmd {
	id := actionExport
	switch key {
	case "a":
		id = actionApply
	case "s":
		id = actionStop
	case "d":
		id = actionDetach
	case "r":
		id = actionRecover
	}
	if ok, reason := m.actionGate(id); !ok {
		m.lifecycleNotice(reason)
		return nil
	}
	switch id {
	case actionApply:
		m.openDialog(dialogApply)
	case actionStop:
		m.openDialog(dialogStop)
	case actionDetach:
		m.openDialog(dialogDetach)
	case actionRecover:
		m.openDialog(dialogRecover)
	case actionExport:
		m.runExport()
	}
	return nil
}
func (m *Model) confirmDetach() tea.Cmd {
	if ok, reason := m.detachGate(); !ok {
		m.dialogErr = reason
		return nil
	}
	return m.detach()
}

type exitRow struct {
	label  string
	action Action
	quit   bool
	back   bool
}

func (m *Model) exitRows() []exitRow {
	if m.cfg.Mode == ModeInspect || (m.ready && !m.liveActive() && !m.phaseBusy()) {
		return []exitRow{{label: "quit", quit: true}, {label: "back", back: true}}
	}
	return []exitRow{{label: "detach (leave running)", action: actionDetach}, {label: "request stop", action: actionStop}, {label: "back", back: true}}
}
func (m *Model) exitKey(key string) tea.Cmd {
	rows := m.exitRows()
	if m.exitRow >= len(rows) {
		m.exitRow = len(rows) - 1
	}
	switch key {
	case "up", "k", "shift+tab":
		m.exitRow = (m.exitRow + len(rows) - 1) % len(rows)
	case "down", "j", "tab":
		m.exitRow = (m.exitRow + 1) % len(rows)
	case "enter":
		row := rows[m.exitRow]
		if row.back {
			m.closeDialog()
			return nil
		}
		if row.quit {
			return m.viewExit()
		}
		if ok, _ := m.actionGate(row.action); !ok {
			return nil
		}
		if row.action == actionDetach {
			return m.detach()
		}
		m.openDialog(dialogStop)
	}
	return nil
}
func (m *Model) exitLines() []line {
	rows := []line{{text: "Exit options", colour: m.cfg.Tokens.Text}}
	for i, row := range m.exitRows() {
		text := "  " + row.label
		if i == min(m.exitRow, len(m.exitRows())-1) {
			text = focusGlyph(m.cfg.Caps) + " " + row.label
		}
		if !row.back && !row.quit {
			if ok, reason := m.actionGate(row.action); !ok {
				text += " — disabled: " + reason
			}
		}
		rows = append(rows, line{text: text})
	}
	return append(rows, line{text: "Esc goes back", colour: m.cfg.Tokens.TextMuted})
}
