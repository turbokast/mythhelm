// Package tui confirmation handling (design §10): explicit labelled modal
// dialog prompts over the injected Actions seam. Every destructive action confirms only
// through its labelled button with initial focus on Cancel, so no single
// accidental keypress confirms (AC-3.3, I03); Esc cancels every prompt with
// no call made.
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Confirmation ids. dialogExitOptions is the q exit-options prompt (task 7);
// the rest are the task 9 palette-action prompts plus the generic result
// prompt an action reports through.
const (
	dialogExitOptions = "exit-options"
	dialogStop        = "stop"
	dialogRecover     = "recover"
	dialogApply       = "apply"
	dialogDone        = "done"
)

// Apply-dialog fields in Tab order: the branch input, the two acceptance
// toggles, then the button row.
const (
	applyFieldBranch = iota
	applyFieldFlags
	applyFieldUnverified
	applyFieldButtons
	applyFieldCount
)

// Button indexes shared by every confirmation prompt: confirm first, Cancel
// second. Prompts always open with focus on Cancel, so the first Enter
// cancels and confirming needs a focus move plus Enter — never one stray
// keypress.
const (
	buttonConfirm = 0
	buttonCancel  = 1
)

// maxBranchRunes bounds the apply dialog's branch input.
const maxBranchRunes = 128

// openDialog opens a confirmation with focus parked on Cancel and any stale
// dialog state cleared. The apply dialog prefills its branch from the run's
// source branch when the snapshot records one.
func (m *Model) openDialog(id string) {
	m.dialog = id
	m.dialogField = 0
	m.dialogFocus = buttonCancel
	m.dialogErr = ""
	m.resultTitle, m.resultBody = "", ""
	m.acceptFlags, m.acceptUnverified = false, false
	m.applyBranch = ""
	if id == dialogApply && m.snap.Run.SourceBranch != "" {
		m.applyBranch = m.snap.Run.SourceBranch
	}
}

// closeDialog dismisses any dialog and clears its transient state, making no
// call. Resize preservation (AC-2.6) only ever observes the id itself.
func (m *Model) closeDialog() {
	m.dialog = ""
	m.dialogField = 0
	m.dialogFocus = buttonCancel
	m.dialogErr = ""
	m.resultTitle, m.resultBody = "", ""
	m.applyBranch = ""
	m.acceptFlags, m.acceptUnverified = false, false
	m.working = ""
}

// showResult opens the generic result dialog: a title plus a plain body,
// dismissed with Enter or Esc.
func (m *Model) showResult(title, body string) {
	m.openDialog(dialogDone)
	m.resultTitle, m.resultBody = title, body
}

// dialogKey routes keys while a dialog is open. Esc cancels every dialog
// with no call made (AC-3.3); every other key is dialog-specific. While a
// confirmed call is in flight the dialog holds its working state and keys
// are inert: the call cannot be unmade, and quitting (ctrl+c) cancels it.
func (m *Model) dialogKey(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.working != "" {
		return m, nil
	}
	if key == "esc" {
		m.closeDialog()
		return m, nil
	}
	switch m.dialog {
	case dialogExitOptions:
		// Task 7 behaviour preserved: the exit-options prompt is
		// informational until a later task wires its rows; only Esc acts.
		return m, nil
	case dialogDone:
		if key == "enter" || key == " " {
			m.closeDialog()
		}
		return m, nil
	case dialogStop:
		return m, m.confirmKey(key, m.confirmStop)
	case dialogRecover:
		return m, m.confirmKey(key, m.confirmRecover)
	case dialogApply:
		return m, m.applyKey(key, msg)
	default:
		return m, nil
	}
}

// confirmKey moves between the two buttons or activates the focused one.
// Enter on Cancel closes with no call; only Enter on the confirm button —
// which never holds initial focus — runs the action, returning its command.
func (m *Model) confirmKey(key string, confirm func() tea.Cmd) tea.Cmd {
	switch key {
	case "left", "right", "tab", "shift+tab":
		m.dialogFocus = 1 - m.dialogFocus
	case "enter":
		if m.dialogFocus == buttonConfirm {
			return confirm()
		}
		m.closeDialog()
	}
	return nil
}

// applyKey drives the apply dialog: Tab cycles the branch field, the two
// toggles and the button row; typing edits the branch; Space/Enter toggles;
// Enter on the button row activates the focused button, returning the
// action's command when it confirms.
func (m *Model) applyKey(key string, msg tea.KeyMsg) tea.Cmd {
	switch key {
	case "tab", "down":
		m.dialogField = (m.dialogField + 1) % applyFieldCount
		return nil
	case "shift+tab", "up":
		m.dialogField = (m.dialogField + applyFieldCount - 1) % applyFieldCount
		return nil
	}
	switch m.dialogField {
	case applyFieldBranch:
		switch key {
		case "enter":
			m.dialogField = applyFieldFlags
		case "backspace", "delete":
			m.applyBranch = dropLastRune(m.applyBranch)
		default:
			m.applyBranch = appendKeyInput(m.applyBranch, key, msg, maxBranchRunes)
		}
	case applyFieldFlags:
		if key == " " || key == "enter" {
			m.acceptFlags = !m.acceptFlags
		}
	case applyFieldUnverified:
		if key == " " || key == "enter" {
			m.acceptUnverified = !m.acceptUnverified
		}
	case applyFieldButtons:
		return m.confirmKey(key, m.confirmApply)
	}
	return nil
}

// dialogLines renders the pending confirmation. An unknown id renders
// labelled, never blank.
func (m *Model) dialogLines() []line {
	switch m.dialog {
	case dialogExitOptions:
		return []line{
			{text: "Exit options", colour: m.cfg.Tokens.Text},
			{text: "detach (leave running)"},
			{text: "request stop"},
			{text: "cancel"},
			{text: "Esc cancels", colour: m.cfg.Tokens.TextMuted},
		}
	case dialogStop:
		rows := []line{
			{text: "Stop run", colour: m.cfg.Tokens.Text},
			{text: "run: " + cellOrUnknown(m.snap.Run.RunID)},
			{text: "effect: request the worker stop"},
			{text: "confirmed only when attempt.stopped is journaled", colour: m.cfg.Tokens.TextMuted},
			m.buttonLine("Request stop"),
		}
		return m.withDialogErr(rows)
	case dialogRecover:
		rows := []line{
			{text: "Recover run", colour: m.cfg.Tokens.Text},
			{text: "run: " + cellOrUnknown(m.snap.Run.RunID)},
			{text: "effect: recover the interrupted run"},
			m.buttonLine("Recover run"),
		}
		return m.withDialogErr(rows)
	case dialogApply:
		return m.withDialogErr(m.applyLines())
	case dialogDone:
		rows := []line{{text: cell(m.resultTitle), colour: m.cfg.Tokens.Text}}
		for b := range strings.SplitSeq(cell(m.resultBody), "\n") {
			rows = append(rows, line{text: b})
		}
		return append(rows, line{text: "Enter or Esc dismisses", colour: m.cfg.Tokens.TextMuted})
	default:
		return []line{
			{text: "dialog: " + cell(m.dialog), colour: m.cfg.Tokens.Text},
			{text: "Esc cancels", colour: m.cfg.Tokens.TextMuted},
		}
	}
}

// applyLines renders the apply dialog: run, candidate commit, branch input,
// acceptance toggles and verification state with [Apply to <branch>]
// [Cancel] (I03: the confirmation binds to these exact effects).
func (m *Model) applyLines() []line {
	branch := m.applyBranch
	if branch == "" {
		branch = "?"
	}
	rows := []line{
		{text: "Apply candidate", colour: m.cfg.Tokens.Text},
		{text: "run: " + cellOrUnknown(m.snap.Run.RunID)},
		{text: "candidate: " + cellOrUnknown(m.candidateCommit())},
		m.applyFieldLine(applyFieldBranch, "target branch: "+cell(m.applyBranch)+"_"),
		m.applyFieldLine(applyFieldFlags, "accept validation flags: "+onOff(m.acceptFlags)),
		m.applyFieldLine(applyFieldUnverified, "accept unverified: "+onOff(m.acceptUnverified)),
	}
	rows = append(rows, m.verificationLines()...)
	confirm := "Apply to " + cell(branch)
	if m.dialogField == applyFieldButtons {
		rows = append(rows, m.buttonLine(confirm))
	} else {
		rows = append(rows, line{text: "[" + confirm + "] [Cancel]"})
	}
	rows = append(rows, line{text: "Tab moves, Space toggles, Esc cancels", colour: m.cfg.Tokens.TextMuted})
	return rows
}

// applyFieldLine renders one apply field with the focus marker when it holds
// focus.
func (m *Model) applyFieldLine(field int, text string) line {
	if m.dialogField == field {
		return line{text: focusGlyph(m.cfg.Caps) + " " + text, colour: m.cfg.Tokens.Focus}
	}
	return line{text: "  " + text}
}

// buttonLine renders a confirmation's [Confirm] [Cancel] row with the focus
// marker before the focused button, which survives ColourNever by shape.
func (m *Model) buttonLine(confirm string) line {
	marker := focusGlyph(m.cfg.Caps) + " "
	confirmBtn, cancelBtn := "["+confirm+"]", "[Cancel]"
	if m.dialogFocus == buttonConfirm {
		confirmBtn = marker + confirmBtn
	} else {
		cancelBtn = marker + cancelBtn
	}
	return line{text: confirmBtn + " " + cancelBtn}
}

// withDialogErr appends the in-place action error, if any. The dialog stays
// open with state unchanged: errors surface where the action was invoked,
// never swallowed.
func (m *Model) withDialogErr(rows []line) []line {
	if m.working != "" {
		return append(rows, line{text: cell(m.working), colour: m.cfg.Tokens.TextMuted})
	}
	if m.dialogErr == "" {
		return rows
	}
	return append(rows, line{text: "error: " + cell(m.dialogErr), colour: m.cfg.Tokens.Err})
}

// candidateCommit is the frozen candidate commit, or "" when no candidate is
// frozen yet.
func (m *Model) candidateCommit() string {
	if m.snap.Candidate == nil {
		return ""
	}
	return m.snap.Candidate.Commit
}

// onOff renders an acceptance toggle.
func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
