package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// maxFilterRunes and maxPaletteRunes bound the / filter and : palette inputs.
const (
	maxFilterRunes  = 128
	maxPaletteRunes = 128
)

// paneOrder is the Tab cycle across the mission panes.
var paneOrder = []string{paneTasks, paneAgents, paneDetail}

// handleKey routes key messages to the focus, filter, palette, help, dialog
// and details flows (design §9). Only Ctrl-C quits; Esc backs out one level
// — dialog, palette, help, filter, previous pane — and never quits.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		m.cancelAction()
		return m, tea.Quit
	}
	if m.dialog != "" {
		return m.dialogKey(key, msg)
	}
	if m.palette.open {
		m.paletteKey(key, msg)
		return m, nil
	}
	if m.helpOpen {
		m.helpKey(key)
		return m, nil
	}
	if m.filterOn {
		m.filterKey(key, msg)
		return m, nil
	}
	switch key {
	case "esc":
		m.backOut()
	case "tab", "right":
		m.cyclePane(1)
	case "shift+tab", "left":
		m.cyclePane(-1)
	case "up", "k":
		m.moveIndex(-1)
	case "down", "j":
		m.moveIndex(1)
	case "/":
		m.filterOn, m.filterText = true, ""
	case ":":
		m.openPalette()
	case "?":
		m.helpOpen = true
	case "enter":
		m.openDetails()
	case "q":
		m.openDialog(dialogExitOptions)
	}
	return m, nil
}

// helpKey handles keys while the help overlay is open: Esc and ? close it,
// the other overlay keys switch overlays, and everything else is ignored.
func (m *Model) helpKey(key string) {
	switch key {
	case "esc", "?":
		m.helpOpen = false
	case ":":
		m.helpOpen = false
		m.openPalette()
	case "/":
		m.helpOpen = false
		m.filterOn, m.filterText = true, ""
	case "q":
		m.helpOpen = false
		m.openDialog(dialogExitOptions)
	}
}

// filterKey handles keys while the / filter input is active: printable input
// is literal (j/k included — a text field is not a list), Enter applies the
// filter, and Esc clears it and leaves input mode in one step.
func (m *Model) filterKey(key string, msg tea.KeyMsg) {
	switch key {
	case "esc":
		m.filterOn, m.filterText = false, ""
	case "enter":
		m.filterOn = false
	case "backspace", "delete":
		m.filterText = dropLastRune(m.filterText)
	default:
		m.filterText = appendKeyInput(m.filterText, key, msg, maxFilterRunes)
	}
}

// paletteKey handles keys while the : palette is open: typing filters the
// action list, arrows and j/k move the selection (the palette is a list, so
// j/k move rather than filter), Enter runs the selection, and Esc closes.
func (m *Model) paletteKey(key string, msg tea.KeyMsg) {
	switch key {
	case "esc":
		m.palette.open = false
	case "enter":
		m.runPaletteSelection()
	case "up", "k":
		if m.palette.sel > 0 {
			m.palette.sel--
		}
	case "down", "j":
		if m.palette.sel < len(m.filteredActions())-1 {
			m.palette.sel++
		}
	case "backspace", "delete":
		m.palette.input = dropLastRune(m.palette.input)
		m.palette.sel = 0
	default:
		before := m.palette.input
		m.palette.input = appendKeyInput(m.palette.input, key, msg, maxPaletteRunes)
		if m.palette.input != before {
			m.palette.sel = 0
		}
	}
}

// appendKeyInput appends one keypress to an overlay input, honouring the rune
// bound. Alt combinations and non-printable keys contribute nothing.
func appendKeyInput(s, key string, msg tea.KeyMsg, max int) string {
	if msg.Alt {
		return s
	}
	switch {
	case key == " ":
		return appendBounded(s, " ", max)
	case msg.Type == tea.KeyRunes:
		return appendBounded(s, string(msg.Runes), max)
	default:
		return s
	}
}

// appendBounded appends add to s up to max runes.
func appendBounded(s, add string, max int) string {
	if max < 1 {
		return s
	}
	out := []rune(s)
	for _, r := range add {
		if len(out) >= max {
			break
		}
		out = append(out, r)
	}
	return string(out)
}

// dropLastRune removes the final rune, if any.
func dropLastRune(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return string(r[:len(r)-1])
}

// cyclePane moves focus delta panes along the Tab order, recording the
// previous pane for Esc.
func (m *Model) cyclePane(delta int) {
	cur := -1
	for i, id := range paneOrder {
		if id == m.focusPane {
			cur = i
			break
		}
	}
	if cur < 0 {
		m.focusPane, m.focusPrev, m.focusIndex = paneTasks, paneTasks, 0
		return
	}
	next := (cur + delta + len(paneOrder)) % len(paneOrder)
	m.focusPrev = m.focusPane
	m.focusPane = paneOrder[next]
	m.focusIndex = clampFocus(m.focusIndex, m.focusedRowCount())
}

// moveIndex moves the selection within the focused pane, clamped to its rows.
func (m *Model) moveIndex(delta int) {
	m.focusIndex = clampFocus(m.focusIndex+delta, m.focusedRowCount())
}

// openDetails focuses the selected-change pane, recording the previous pane
// for Esc.
func (m *Model) openDetails() {
	if m.focusPane != paneDetail {
		m.focusPrev = m.focusPane
		m.focusPane = paneDetail
	}
	m.focusIndex = clampFocus(m.focusIndex, m.focusedRowCount())
}

// openPalette opens the command palette over a cleared query.
func (m *Model) openPalette() {
	m.palette = paletteState{open: true}
}

// backOut handles Esc in the mission view: it clears an applied filter, else
// restores the previous pane, else does nothing. It never quits.
func (m *Model) backOut() {
	if m.filterText != "" {
		m.filterText = ""
		return
	}
	if m.focusPane == paneTasks {
		return
	}
	if m.focusPrev == "" || m.focusPrev == m.focusPane {
		m.focusPrev = paneTasks
	}
	m.focusPane = m.focusPrev
	m.focusPrev = paneTasks
	m.focusIndex = clampFocus(m.focusIndex, m.focusedRowCount())
}

// applyFilter hides a tasks/agents pane's data rows when none of them match
// the applied filter, replacing them with a labelled no-matches row. The
// header and rule rows always survive; the selected-change pane never passes
// through here, so verification status stays visible under any filter (I07).
func (m *Model) applyFilter(rows []line) []line {
	if m.filterText == "" || len(rows) < 3 {
		return rows
	}
	needle := strings.ToLower(m.filterText)
	for _, ln := range rows[2:] {
		if strings.Contains(strings.ToLower(ln.text), needle) {
			return rows
		}
	}
	out := make([]line, 2, 3)
	copy(out, rows[:2])
	return append(out, line{text: "no matches for filter \"" + cell(m.filterText) + "\""})
}

// filterLine renders the active-filter status row: the / query with an input
// cursor while the filter input is active.
func (m *Model) filterLine() (line, bool) {
	if !m.filterOn && m.filterText == "" {
		return line{}, false
	}
	text := "filter /" + cell(m.filterText)
	if m.filterOn {
		text += "_"
	}
	return line{text: text, colour: m.cfg.Tokens.TextMuted}, true
}
