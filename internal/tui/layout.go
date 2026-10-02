package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Layout breakpoints from the §15.4 table (design §6).
const (
	wideWidth    = 140 // three panes at or above
	twoPaneWidth = 100 // two panes at or above
	singleWidth  = 80  // single pane at or above; below is compact
	minHeight    = 10  // below is compact regardless of width (D14)
)

// layout is one rung of the §15.4 ladder.
type layout int

const (
	layoutCompact layout = iota
	layoutSingle
	layoutTwoPane
	layoutThreePane
)

// resolveLayout maps the terminal size to its ladder rung. Height is
// independent of width: a very short terminal is compact even when wide.
func resolveLayout(width, height int) layout {
	if height < minHeight || width < singleWidth {
		return layoutCompact
	}
	switch {
	case width >= wideWidth:
		return layoutThreePane
	case width >= twoPaneWidth:
		return layoutTwoPane
	default:
		return layoutSingle
	}
}

// applySize relays a WindowSizeMsg: the selected task ID, focused pane and
// pending dialog survive every resize; only the focus index clamps to the
// rows the focused pane still has, never resetting to zero while rows remain
// (AC-2.6, I06). Non-positive dimensions are ignored, never stored.
func (m *Model) applySize(msg tea.WindowSizeMsg) {
	if msg.Width > 0 {
		m.width = msg.Width
	}
	if msg.Height > 0 {
		m.height = msg.Height
	}
	m.focusIndex = clampFocus(m.focusIndex, m.focusedRowCount())
}

// clampFocus clamps i into the focused pane's rows.
func clampFocus(i, rows int) int {
	if rows < 1 || i < 0 {
		return 0
	}
	if i > rows-1 {
		return rows - 1
	}
	return i
}

// focusedRowCount is the focused pane's selectable row count. Stage 1 panes
// carry one row each (one task, one lane, one change); task 7 refines this
// per pane when keyboard navigation lands.
func (m *Model) focusedRowCount() int {
	if !m.ready {
		return 0
	}
	return 1
}
