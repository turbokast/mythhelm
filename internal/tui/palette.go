package tui

import (
	"strings"

	"github.com/turbokast/mythhelm/internal/tui/theme"
)

// Action identifies a command-palette entry (design §9–§10): stop, recover
// and apply open confirmations, export runs immediately, and the rest are
// view-only selections (actions.go gates each one).
type Action int

const (
	actionStop Action = iota
	actionRecover
	actionApply
	actionExport
	actionSwitchPane
	actionTheme
	actionHelp
	actionQuit
)

// actionDef is one registered palette entry: its full action name, the key
// hint shown beside it, and its one-line effect description.
type actionDef struct {
	id   Action
	name string
	keys string
	desc string
}

// actionRegistry lists every action the palette offers, in display order.
var actionRegistry = []actionDef{
	{actionStop, "Stop run", "palette", "Request the worker stop (confirmation)"},
	{actionRecover, "Recover run", "palette", "Recover the interrupted run (confirmation)"},
	{actionApply, "Apply candidate", "palette", "Apply the candidate to its branch (confirmation)"},
	{actionExport, "Export view", "palette", "Export the focused pane as plain text"},
	{actionSwitchPane, "Switch pane", "Tab", "Move focus to the next pane"},
	{actionTheme, "Toggle theme", "palette", "Switch between the dark and light themes"},
	{actionHelp, "Help", "?", "Show contextual help"},
	{actionQuit, "Exit options", "q", "Detach, request stop, or cancel"},
}

// paletteState is the command-palette overlay: whether it is open, its
// filter input, and the selection index into the filtered list.
type paletteState struct {
	open  bool
	input string
	sel   int
}

// filteredActions returns the registry entries whose full name contains the
// palette input as a case-insensitive substring; an empty query lists every
// registered action.
func (m *Model) filteredActions() []actionDef {
	if m.palette.input == "" {
		return actionRegistry
	}
	needle := strings.ToLower(m.palette.input)
	var out []actionDef
	for _, def := range actionRegistry {
		if strings.Contains(strings.ToLower(def.name), needle) {
			out = append(out, def)
		}
	}
	return out
}

// runPaletteSelection runs the selected action and closes the palette.
// Disabled actions are inert and leave the palette open on their visible
// reason; stop, recover and apply open their confirmations, export runs
// immediately, and the view-only actions take effect at once.
func (m *Model) runPaletteSelection() {
	list := m.filteredActions()
	if m.palette.sel < 0 || m.palette.sel >= len(list) {
		m.palette.open = false
		return
	}
	id := list[m.palette.sel].id
	if ok, _ := m.actionGate(id); !ok {
		return
	}
	m.palette.open = false
	switch id {
	case actionStop:
		m.openDialog(dialogStop)
	case actionRecover:
		m.openDialog(dialogRecover)
	case actionApply:
		m.openDialog(dialogApply)
	case actionExport:
		m.runExport()
	case actionSwitchPane:
		m.cyclePane(1)
	case actionTheme:
		m.toggleTheme()
	case actionHelp:
		m.helpOpen = true
	case actionQuit:
		m.openDialog(dialogExitOptions)
	}
}

// toggleTheme swaps the active built-in theme between dark and light.
func (m *Model) toggleTheme() {
	next := "light"
	if m.themeName == "light" {
		next = "dark"
	}
	if tokens, err := theme.BuiltIn(next); err == nil {
		m.cfg.Tokens, m.themeName = tokens, next
	}
}

// paletteLines renders the command-palette overlay: its query with an input
// cursor, then every matching action with its key hint. The selected row
// carries the focus marker, which survives ColourNever by shape (AC-5.2).
func (m *Model) paletteLines() []line {
	t := m.cfg.Tokens
	rows := []line{
		{text: "COMMANDS", colour: t.Text},
		{text: ": " + cell(m.palette.input) + "_", colour: t.Text},
	}
	list := m.filteredActions()
	if len(list) == 0 {
		return append(rows, line{text: "no matching actions"})
	}
	for i, def := range list {
		marker, colour := "  ", ""
		if i == m.palette.sel {
			marker, colour = focusGlyph(m.cfg.Caps)+" ", t.Focus
		}
		text := marker + def.name + " (" + def.keys + ")"
		if ok, reason := m.actionGate(def.id); !ok {
			text += " " + dashGlyph(m.cfg.Caps) + " disabled: " + reason
		}
		rows = append(rows, line{text: text, colour: colour})
	}
	return rows
}
