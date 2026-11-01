package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestObservatoryLadderGoldens(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{160, 40}, {148, 40}, {120, 30}, {110, 24}, {90, 24}, {88, 24}, {79, 24}, {72, 24}, {59, 24}, {160, 9}, {100, 24}} {
		for _, ascii := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dx%d/ascii=%t", size[0], size[1], ascii), func(t *testing.T) {
				t.Parallel()
				c := fullCaps
				if ascii {
					c = asciiCaps
				}
				m := readyModel(t, c, size[0], size[1])
				m.snap.Run.State = "executing"
				if size[0] < 80 {
					m.snap.Run.State = "blocked"
					m.snap.Run.Reason = "recorded refusal"
				}
				view := stripANSISequences(m.View())
				rows := strings.Split(view, "\n")
				if len(rows) != size[1] {
					t.Fatalf("capture has %d rows, want %d", len(rows), size[1])
				}
				if !strings.Contains(rows[len(rows)-1], "q exit") {
					t.Fatalf("keys not last: %q", rows[len(rows)-1])
				}
				if size[0] >= 140 && size[1] >= 10 {
					for _, name := range []string{"TASKS", "AGENT", "EVIDENCE"} {
						if !strings.Contains(view, name) {
							t.Errorf("wide frame lacks %s", name)
						}
					}
				}
				if size[0] < 80 {
					for _, name := range []string{"now ", "next ", "use ", "needs you", "--accessible: linear"} {
						if !strings.Contains(view, name) {
							t.Errorf("compact frame lacks %s", name)
						}
					}
				}
				for _, row := range rows {
					if lipgloss.Width(row) > size[0] {
						t.Fatalf("wide row: %q", row)
					}
				}
			})
		}
	}
}
func TestObservatoryHeightBoundary(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{160, 10}, {160, 9}, {72, 5}} {
		m := readyModel(t, fullCaps, size[0], size[1])
		m.snap.Run.State = "blocked"
		view := stripANSISequences(m.View())
		rows := strings.Split(view, "\n")
		if len(rows) != size[1] || !strings.Contains(rows[len(rows)-1], "q exit") {
			t.Fatalf("mandatory chrome lost at %v: %q", size, view)
		}
		if size[1] == 10 && !strings.Contains(view, "EVIDENCE") {
			t.Fatal("ten rows lost width-driven panes")
		}
		if size[1] < 10 {
			for _, text := range []string{"now ", "next ", "use ", "--accessible: linear"} {
				if !strings.Contains(view, text) {
					t.Fatalf("compact missing %s at %v", text, size)
				}
			}
		}
	}
}
func TestHiddenKeysStillWork(t *testing.T) {
	t.Parallel()
	m := readyModel(t, fullCaps, 88, 24)
	m.handleKey(runeMsg(":"))
	if !m.palette.open {
		t.Fatal("hidden palette key stopped working")
	}
	m.handleKey(specialMsg(tea.KeyEsc))
	m.handleKey(runeMsg("/"))
	if !m.filterOn {
		t.Fatal("hidden filter key stopped working")
	}
	m.handleKey(specialMsg(tea.KeyEsc))
	m.handleKey(runeMsg("?"))
	help := stripANSISequences(m.View())
	for _, text := range []string{":", "/", "v", "disabled:"} {
		if !strings.Contains(help, text) {
			t.Fatalf("help hides %q: %s", text, help)
		}
	}
}
