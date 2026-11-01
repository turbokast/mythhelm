package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestLayoutBreakpoints renders the §15.4 ladder: compact below 80 columns,
// single at 80–99, two panes at 100–139, three at 140 and above.
func TestLayoutBreakpoints(t *testing.T) {
	t.Parallel()
	viewAt := func(width int) string {
		m := readyModel(t, fullCaps, width, 30)
		m.snap.Run.State = "executing"
		return stripANSISequences(m.View())
	}
	t.Run("compact at 60", func(t *testing.T) {
		t.Parallel()
		view := viewAt(60)
		for _, want := range []string{"now ", "use ", "--accessible: linear"} {
			if !strings.Contains(view, want) {
				t.Errorf("60-column view missing %q\n%s", want, view)
			}
		}
		for _, bad := range []string{"TASKS", "AGENT", "EVIDENCE"} {
			if strings.Contains(view, bad) {
				t.Errorf("60-column view renders pane %q\n%s", bad, view)
			}
		}
		assertWidthSafe(t, view, 60, 30)
	})
	t.Run("single at 85", func(t *testing.T) {
		t.Parallel()
		view := viewAt(85)
		for _, want := range []string{"Tasks", "TASKS", "q exit"} {
			if !strings.Contains(view, want) {
				t.Errorf("85-column view missing %q\n%s", want, view)
			}
		}
		for _, bad := range []string{"AGENT", "EVIDENCE"} {
			if strings.Contains(view, bad) {
				t.Errorf("85-column view renders pane %q\n%s", bad, view)
			}
		}
		assertWidthSafe(t, view, 85, 30)
	})
	t.Run("two panes at 120", func(t *testing.T) {
		t.Parallel()
		view := viewAt(120)
		for _, want := range []string{"TASKS", "AGENT"} {
			if !strings.Contains(view, want) {
				t.Errorf("120-column view missing %q\n%s", want, view)
			}
		}
		for _, bad := range []string{"EVIDENCE", "Tasks"} {
			if strings.Contains(view, bad) {
				t.Errorf("120-column view renders %q\n%s", bad, view)
			}
		}
		assertWidthSafe(t, view, 120, 30)
	})
	t.Run("two panes at 139", func(t *testing.T) {
		t.Parallel()
		view := viewAt(139)
		if strings.Contains(view, "EVIDENCE") {
			t.Errorf("139-column view renders three panes\n%s", view)
		}
		if !strings.Contains(view, "AGENT") {
			t.Errorf("139-column view missing the agents pane\n%s", view)
		}
		assertWidthSafe(t, view, 139, 30)
	})
	t.Run("three panes at 140", func(t *testing.T) {
		t.Parallel()
		view := viewAt(140)
		for _, want := range []string{"TASKS", "AGENT", "EVIDENCE"} {
			if !strings.Contains(view, want) {
				t.Errorf("140-column view missing %q\n%s", want, view)
			}
		}
		assertWidthSafe(t, view, 140, 30)
	})
	t.Run("three panes at 160", func(t *testing.T) {
		t.Parallel()
		view := viewAt(160)
		for _, want := range []string{"TASKS", "AGENT", "EVIDENCE"} {
			if !strings.Contains(view, want) {
				t.Errorf("160-column view missing %q\n%s", want, view)
			}
		}
		if strings.Contains(view, "Tasks") {
			t.Errorf("160-column view renders the single-pane tab bar\n%s", view)
		}
		assertWidthSafe(t, view, 160, 30)
	})
	t.Run("detail panel switches", func(t *testing.T) {
		t.Parallel()
		m := readyModel(t, fullCaps, 120, 30)
		m.openDetails()
		view := stripANSISequences(m.View())
		for _, want := range []string{"TASKS", "DIFF"} {
			if !strings.Contains(view, want) {
				t.Errorf("switched two-pane view missing %q\n%s", want, view)
			}
		}
		if strings.Contains(view, "AGENT") {
			t.Errorf("switched two-pane view still renders agents\n%s", view)
		}
	})
}

// TestShortHeightCompact renders the compact view with its linear-mode offer
// below 10 rows, and keeps the width-driven layout at 10 rows (AC-2.4).
func TestShortHeightCompact(t *testing.T) {
	t.Parallel()
	for _, height := range []int{5, 9} {
		view := stripANSISequences(readyModel(t, fullCaps, 160, height).View())
		for _, want := range []string{"now ", "use ", "--accessible: linear"} {
			if !strings.Contains(view, want) {
				t.Errorf("160x%d view missing %q\n%s", height, want, view)
			}
		}
		if rows := len(strings.Split(view, "\n")); rows > height {
			t.Errorf("160x%d view has %d rows", height, rows)
		}
	}
	view := readyModel(t, fullCaps, 160, 10).View()
	if !strings.Contains(view, "EVIDENCE") {
		t.Errorf("160x10 view lost its width-driven layout\n%s", view)
	}
}

// TestResizePreservesIdentity drives WindowSizeMsg sequences and asserts the
// selected task, focused pane and pending dialog survive every resize while
// the focus index clamps instead of resetting (AC-2.6).
func TestResizePreservesIdentity(t *testing.T) {
	t.Parallel()
	m := readyModel(t, fullCaps, 160, 30)
	m.snap.Run.State = "executing"
	m.selectedTask, m.selectedLane = "task_9", "attempt_9"
	for _, want := range []string{"agent", "evidence", "diff", "tasks"} {
		m.handleKey(specialMsg(tea.KeyTab))
		if m.focusPane != want {
			t.Fatalf("four stable tab IDs: got%q want%q", m.focusPane, want)
		}
	}
	for _, origin := range []string{"tasks", "agent", "evidence"} {
		m.focusPane = origin
		m.focusIndex = 5
		m.openDetails()
		if m.focusPane != "diff" {
			t.Fatalf("Enter did not open Diff from %s", origin)
		}
		m.dialog = dialogExitOptions
		for _, size := range []tea.WindowSizeMsg{{Width: 160, Height: 40}, {Width: 120, Height: 30}, {Width: 88, Height: 24}, {Width: 72, Height: 24}, {Width: 160, Height: 9}, {Width: 160, Height: 10}} {
			m.Update(size)
			if m.selectedTask != "task_9" || m.selectedLane != "attempt_9" || m.focusPane != "diff" || m.dialog != dialogExitOptions || m.focusIndex != 0 {
				t.Fatalf("resize%v changed task/lane/focus/dialog/index: %q/%q/%q/%q/%d", size, m.selectedTask, m.selectedLane, m.focusPane, m.dialog, m.focusIndex)
			}
		}
		m.dialog = ""
		m.handleKey(specialMsg(tea.KeyEsc))
		if m.focusPane != origin {
			t.Fatalf("Esc restored%q instead of Diff origin%q", m.focusPane, origin)
		}
	}
}
