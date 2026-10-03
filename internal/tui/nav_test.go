package tui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/turbokast/mythhelm/internal/tui/theme"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

// runeMsg builds a printable-key message; specialMsg builds one for a named
// non-printable key (enter, esc, tab, arrows, ...).
func runeMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func specialMsg(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

// pressKey drives one key message through Update and returns the model and
// command; a non-Model return fails the test.
func pressKey(t *testing.T, m *Model, msg tea.KeyMsg) (*Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(msg)
	mm, ok := updated.(*Model)
	if !ok {
		t.Fatalf("Update returned %T, want *Model", updated)
	}
	return mm, cmd
}

// typeRunes feeds printable input one rune at a time, as a terminal would.
func typeRunes(t *testing.T, m *Model, s string) *Model {
	t.Helper()
	for _, r := range s {
		m, _ = pressKey(t, m, runeMsg(string(r)))
	}
	return m
}

// flowModel is a wide ready model for key-flow tests.
func flowModel(t *testing.T) *Model {
	t.Helper()
	return readyModel(t, fullCaps, 160, 30)
}

// TestKeyFlows drives every §15.5 surface through key messages using arrows
// alone (no j/k): Tab/arrows focus, / filter, : palette, ? help, Enter
// details and the Esc back-out order (AC-3.1). Any unreachable surface fails.
func TestKeyFlows(t *testing.T) {
	t.Parallel()

	t.Run("tab and arrows move focus", func(t *testing.T) {
		t.Parallel()
		m := flowModel(t)
		if m.focusPane != paneTasks {
			t.Fatalf("initial focus = %q, want tasks", m.focusPane)
		}
		m, _ = pressKey(t, m, specialMsg(tea.KeyTab))
		if m.focusPane != paneAgents {
			t.Fatalf("after Tab focus = %q, want agents", m.focusPane)
		}
		m, _ = pressKey(t, m, specialMsg(tea.KeyTab))
		if m.focusPane != paneDetail {
			t.Fatalf("after Tab Tab focus = %q, want detail", m.focusPane)
		}
		m, _ = pressKey(t, m, specialMsg(tea.KeyTab))
		if m.focusPane != paneTasks {
			t.Fatalf("after Tab Tab Tab focus = %q, want tasks", m.focusPane)
		}
		m, _ = pressKey(t, m, specialMsg(tea.KeyShiftTab))
		if m.focusPane != paneDetail {
			t.Fatalf("after Shift-Tab focus = %q, want detail", m.focusPane)
		}
		m, _ = pressKey(t, m, specialMsg(tea.KeyLeft))
		if m.focusPane != paneAgents {
			t.Fatalf("after Left focus = %q, want agents", m.focusPane)
		}
		m, _ = pressKey(t, m, specialMsg(tea.KeyRight))
		if m.focusPane != paneDetail {
			t.Fatalf("after Right focus = %q, want detail", m.focusPane)
		}
		// Up/Down stay on the single Stage 1 row without leaving the pane.
		m, _ = pressKey(t, m, specialMsg(tea.KeyUp))
		m, _ = pressKey(t, m, specialMsg(tea.KeyDown))
		if m.focusPane != paneDetail || m.focusIndex != 0 {
			t.Fatalf("after Up/Down focus = %q/%d, want detail/0", m.focusPane, m.focusIndex)
		}
	})

	t.Run("slash filters the focused collection", func(t *testing.T) {
		t.Parallel()
		m := flowModel(t)
		m, _ = pressKey(t, m, runeMsg("/"))
		if !m.filterOn {
			t.Fatal("after / the filter input is not active")
		}
		m = typeRunes(t, m, "pagin")
		if m.filterText != "pagin" {
			t.Fatalf("filter text = %q, want pagin", m.filterText)
		}
		if view := m.View(); !strings.Contains(view, "filter /pagin") {
			t.Errorf("view hides the active filter\n%s", view)
		}
		// A matching filter keeps the task row; a non-matching one labels
		// the absence instead of rendering an empty pane.
		if view := m.View(); !strings.Contains(view, "Add pagination to the audit log") {
			t.Errorf("matching filter hid the task row\n%s", view)
		}
		m = typeRunes(t, m, "zzz-no-such-task")
		if view := m.View(); !strings.Contains(view, `no matches for filter "paginzzz-no-such-task"`) {
			t.Errorf("non-matching filter lacks its label\n%s", view)
		}
		// Enter keeps the filter applied but leaves input mode; Esc clears it.
		m, _ = pressKey(t, m, specialMsg(tea.KeyEnter))
		if m.filterOn || m.filterText == "" {
			t.Fatalf("after Enter filter on=%v text=%q, want applied text", m.filterOn, m.filterText)
		}
		m, _ = pressKey(t, m, specialMsg(tea.KeyEsc))
		if m.filterOn || m.filterText != "" {
			t.Fatalf("after Esc filter on=%v text=%q, want cleared", m.filterOn, m.filterText)
		}
	})

	t.Run("colon opens the searchable palette", func(t *testing.T) {
		t.Parallel()
		m := flowModel(t)
		m, _ = pressKey(t, m, runeMsg(":"))
		if !m.palette.open {
			t.Fatal("after : the palette is not open")
		}
		view := m.View()
		if !strings.Contains(view, "COMMANDS") {
			t.Errorf("palette view lacks its title\n%s", view)
		}
		for _, def := range actionRegistry {
			if !strings.Contains(view, def.name) {
				t.Errorf("palette view lacks action %q\n%s", def.name, view)
			}
		}
		m = typeRunes(t, m, "stop")
		view = m.View()
		if !strings.Contains(view, "Stop run") {
			t.Errorf("filtered palette lost Stop run\n%s", view)
		}
		if strings.Contains(view, "Recover run") {
			t.Errorf("filtered palette still lists Recover run\n%s", view)
		}
	})

	t.Run("question opens help", func(t *testing.T) {
		t.Parallel()
		m := flowModel(t)
		m, _ = pressKey(t, m, runeMsg("?"))
		if !m.helpOpen {
			t.Fatal("after ? help is not open")
		}
		if view := m.View(); !strings.Contains(view, "HELP") {
			t.Errorf("help view lacks its title\n%s", view)
		}
	})

	t.Run("enter opens details", func(t *testing.T) {
		t.Parallel()
		m := readyModel(t, fullCaps, 85, 30)
		if view := m.View(); !strings.Contains(view, "TASKS") {
			t.Fatalf("single-pane view starts outside TASKS\n%s", view)
		}
		m, _ = pressKey(t, m, specialMsg(tea.KeyEnter))
		if m.focusPane != paneDetail {
			t.Fatalf("after Enter focus = %q, want detail", m.focusPane)
		}
		if view := m.View(); !strings.Contains(view, "SELECTED CHANGE") {
			t.Errorf("after Enter the single pane is not the detail pane\n%s", view)
		}
	})

	t.Run("q opens exit options", func(t *testing.T) {
		t.Parallel()
		m := flowModel(t)
		m, _ = pressKey(t, m, runeMsg("q"))
		if m.dialog != dialogExitOptions {
			t.Fatalf("after q dialog = %q, want exit-options", m.dialog)
		}
		view := m.View()
		for _, want := range []string{"Exit options", "detach (leave running)", "request stop", "cancel"} {
			if !strings.Contains(view, want) {
				t.Errorf("exit-options dialog lacks %q\n%s", want, view)
			}
		}
	})

	t.Run("esc backs out in order", func(t *testing.T) {
		t.Parallel()
		m := flowModel(t)
		m.dialog = dialogExitOptions
		m.palette = paletteState{open: true, input: "sto"}
		m.helpOpen = true
		m.filterOn, m.filterText = true, "pag"
		m.focusPane, m.focusPrev = paneDetail, paneAgents
		steps := []struct {
			name string
			want func(*Model) bool
		}{
			{"dialog closes first", func(m *Model) bool {
				return m.dialog == "" && m.palette.open && m.helpOpen && m.filterText == "pag"
			}},
			{"palette closes second", func(m *Model) bool {
				return !m.palette.open && m.helpOpen && m.filterText == "pag"
			}},
			{"help closes third", func(m *Model) bool {
				return !m.helpOpen && m.filterOn && m.filterText == "pag"
			}},
			{"filter clears fourth", func(m *Model) bool {
				return !m.filterOn && m.filterText == "" && m.focusPane == paneDetail
			}},
			{"pane backs out last", func(m *Model) bool {
				return m.focusPane == paneAgents
			}},
		}
		for _, step := range steps {
			var cmd tea.Cmd
			m, cmd = pressKey(t, m, specialMsg(tea.KeyEsc))
			if cmd != nil {
				t.Fatalf("Esc during %s returned a command (quit?)", step.name)
			}
			if !step.want(m) {
				t.Fatalf("after Esc for %s: dialog=%q palette=%v help=%v filter=%v/%q pane=%q",
					step.name, m.dialog, m.palette.open, m.helpOpen, m.filterOn, m.filterText, m.focusPane)
			}
		}
	})
}

// TestEscapeNeverQuits presses Esc from every depth — dialog, palette,
// palette with input, help, filter input, applied filter, each pane and the
// top-level tasks pane — and asserts the program never quits (a nil command).
// A quit-on-Esc variant fails every depth.
func TestEscapeNeverQuits(t *testing.T) {
	t.Parallel()
	setup := func(m *Model) *Model { return m }
	withDialog := func(m *Model) *Model { m.dialog = dialogExitOptions; return m }
	withPalette := func(m *Model) *Model { m.palette = paletteState{open: true}; return m }
	withPaletteInput := func(m *Model) *Model { m.palette = paletteState{open: true, input: "sto"}; return m }
	withHelp := func(m *Model) *Model { m.helpOpen = true; return m }
	withFilterInput := func(m *Model) *Model { m.filterOn, m.filterText = true, "pag"; return m }
	withAppliedFilter := func(m *Model) *Model { m.filterText = "pag"; return m }
	onAgents := func(m *Model) *Model { m.focusPane = paneAgents; return m }
	onDetail := func(m *Model) *Model { m.focusPane, m.focusPrev = paneDetail, paneTasks; return m }
	cases := []struct {
		name  string
		apply func(*Model) *Model
	}{
		{"dialog", withDialog},
		{"palette", withPalette},
		{"palette with input", withPaletteInput},
		{"help", withHelp},
		{"filter input", withFilterInput},
		{"applied filter", withAppliedFilter},
		{"agents pane", onAgents},
		{"detail pane", onDetail},
		{"top-level tasks pane", setup},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := tc.apply(flowModel(t))
			for i := 0; i < 3; i++ {
				var cmd tea.Cmd
				m, cmd = pressKey(t, m, specialMsg(tea.KeyEsc))
				if cmd != nil {
					t.Fatalf("Esc %d from %s returned a command (quit?)", i+1, tc.name)
				}
			}
			if !m.ready {
				t.Errorf("Esc from %s left the model unready", tc.name)
			}
		})
	}
}

// TestPaletteSearchable types into the palette and asserts the action list
// filters by substring: every registered action is reachable by typing part
// of its name, matching is case-insensitive, and an empty query lists every
// registered action.
func TestPaletteSearchable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		query    string
		contains []string
		excludes []string
	}{
		{"stop", "stop", []string{"Stop run"}, []string{"Recover run", "Exit options"}},
		{"recover", "rec", []string{"Recover run"}, []string{"Stop run"}},
		{"apply", "appl", []string{"Apply candidate"}, []string{"Stop run"}},
		{"export", "expo", []string{"Export view"}, []string{"Stop run"}},
		{"switch pane", "swit", []string{"Switch pane"}, []string{"Stop run"}},
		{"theme", "them", []string{"Toggle theme"}, []string{"Stop run"}},
		{"help", "help", []string{"Help"}, []string{"Stop run"}},
		{"exit options", "exit", []string{"Exit options"}, []string{"Stop run"}},
		{"multi-match", "run", []string{"Stop run", "Recover run"}, []string{"Exit options"}},
		{"case-insensitive", "STOP", []string{"Stop run"}, []string{"Recover run"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := flowModel(t)
			m, _ = pressKey(t, m, runeMsg(":"))
			m = typeRunes(t, m, tc.query)
			view := m.View()
			for _, want := range tc.contains {
				if !strings.Contains(view, want) {
					t.Errorf("query %q lost %q\n%s", tc.query, want, view)
				}
			}
			for _, bad := range tc.excludes {
				if strings.Contains(view, bad) {
					t.Errorf("query %q still lists %q\n%s", tc.query, bad, view)
				}
			}
		})
	}
	t.Run("empty query lists every registered action", func(t *testing.T) {
		t.Parallel()
		m := flowModel(t)
		m, _ = pressKey(t, m, runeMsg(":"))
		view := m.View()
		for _, def := range actionRegistry {
			if !strings.Contains(view, def.name) {
				t.Errorf("unfiltered palette lacks %q\n%s", def.name, view)
			}
		}
		if got := len(m.filteredActions()); got != len(actionRegistry) {
			t.Errorf("unfiltered palette holds %d actions, want %d", got, len(actionRegistry))
		}
		if len(actionRegistry) != 8 {
			t.Errorf("registry holds %d actions, want the 8 design §10 actions", len(actionRegistry))
		}
	})
}

// TestHelpFullNames asserts the help overlay names every registered action by
// its full name with its effect, never by bare key alone (AC-5.2): a
// keys-only help fails on the multi-word names.
func TestHelpFullNames(t *testing.T) {
	t.Parallel()
	m := flowModel(t)
	m, _ = pressKey(t, m, runeMsg("?"))
	view := m.View()
	for _, def := range actionRegistry {
		if !strings.Contains(view, def.name) {
			t.Errorf("help lacks the full action name %q\n%s", def.name, view)
		}
		if !strings.Contains(view, def.desc) {
			t.Errorf("help lacks the effect text for %q\n%s", def.name, view)
		}
	}
	for _, want := range []string{"Stop run", "Exit options", "Switch pane", "Toggle theme"} {
		if !strings.Contains(view, want) {
			t.Errorf("help lacks multi-word action name %q (keys-only?)\n%s", want, view)
		}
	}
	// The ASCII twin stays byte-clean: help is readable without Unicode.
	ascii := readyModel(t, asciiCaps, 160, 40)
	ascii, _ = pressKey(t, ascii, runeMsg("?"))
	asciiView := ascii.View()
	for i := 0; i < len(asciiView); i++ {
		if asciiView[i] >= 0x80 {
			t.Errorf("ASCII help holds non-ASCII byte at %d: %q", i, asciiView)
			break
		}
	}
	for _, def := range actionRegistry {
		if !strings.Contains(asciiView, def.name) {
			t.Errorf("ASCII help lacks the full action name %q", def.name)
		}
	}
}

// TestFocusStableAcrossReload reloads a changed snapshot and asserts the
// selection re-resolves by stable ID: the same task and lane stay selected,
// the focused pane and index survive, and the new content renders (AC-5.2).
// A variant dropping to no selection fails.
func TestFocusStableAcrossReload(t *testing.T) {
	t.Parallel()
	m := newModel(testConfig(fullCaps, t))
	m.loadDiff = stubDiff(mustParseDiff(t, smallDiffInput))
	first := richSnapshot()
	m.setSnapshot(t.Context(), first)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	m = updated.(*Model)
	if m.selectedTask == "" || m.selectedLane == "" {
		t.Fatalf("first load selected task=%q lane=%q, want stable IDs", m.selectedTask, m.selectedLane)
	}
	m.focusPane = paneAgents
	updated, _ = m.Update(specialMsg(tea.KeyTab))
	m = updated.(*Model)
	if m.focusPane != paneDetail {
		t.Fatalf("Tab from agents reached %q, want detail", m.focusPane)
	}
	second := richSnapshot()
	second.Goal = "Land the reworked audit export"
	second.Run.State = "executing"
	second.LatestProgress = &viewmodel.Progress{AssistantTurns: 11, RunSeq: 20}
	m.setSnapshot(t.Context(), second)
	if m.selectedTask == "" || m.selectedLane == "" {
		t.Errorf("reload dropped the selection: task=%q lane=%q", m.selectedTask, m.selectedLane)
	}
	if m.focusPane != paneDetail || m.focusIndex != 0 {
		t.Errorf("reload moved focus to %q/%d, want detail/0", m.focusPane, m.focusIndex)
	}
	view := m.View()
	if !strings.Contains(view, "Land the reworked audit export") {
		t.Errorf("reloaded view lacks the new goal\n%s", view)
	}
	if !strings.Contains(view, "11 assistant turns (native-reported)") {
		t.Errorf("reloaded view lacks the new progress\n%s", view)
	}
	// The lane the reload kept is the same attempt the first load selected.
	if m.selectedLane != first.Attempt.AttemptID {
		t.Errorf("selected lane = %q, want the stable attempt %q", m.selectedLane, first.Attempt.AttemptID)
	}
}

// TestJKOptional asserts j/k move within the palette list, match the arrow
// keys step for step in the mission view, and are never required: the arrow
// keys alone reach every palette entry.
func TestJKOptional(t *testing.T) {
	t.Parallel()
	t.Run("jk move within the palette list", func(t *testing.T) {
		t.Parallel()
		m := flowModel(t)
		m, _ = pressKey(t, m, runeMsg(":"))
		m, _ = pressKey(t, m, runeMsg("j"))
		m, _ = pressKey(t, m, runeMsg("j"))
		if m.palette.sel != 2 {
			t.Fatalf("after j j palette selection = %d, want 2", m.palette.sel)
		}
		m, _ = pressKey(t, m, runeMsg("k"))
		if m.palette.sel != 1 {
			t.Fatalf("after j j k palette selection = %d, want 1", m.palette.sel)
		}
	})
	t.Run("jk match arrows in the mission view", func(t *testing.T) {
		t.Parallel()
		viaJK := flowModel(t)
		viaJK, _ = pressKey(t, viaJK, runeMsg("j"))
		viaJK, _ = pressKey(t, viaJK, runeMsg("k"))
		viaArrows := flowModel(t)
		viaArrows, _ = pressKey(t, viaArrows, specialMsg(tea.KeyDown))
		viaArrows, _ = pressKey(t, viaArrows, specialMsg(tea.KeyUp))
		if viaJK.focusPane != viaArrows.focusPane || viaJK.focusIndex != viaArrows.focusIndex {
			t.Fatalf("j/k reached %q/%d, arrows reached %q/%d",
				viaJK.focusPane, viaJK.focusIndex, viaArrows.focusPane, viaArrows.focusIndex)
		}
	})
	t.Run("arrows alone reach every palette entry", func(t *testing.T) {
		t.Parallel()
		m := flowModel(t)
		m, _ = pressKey(t, m, runeMsg(":"))
		for i := 1; i < len(actionRegistry); i++ {
			m, _ = pressKey(t, m, specialMsg(tea.KeyDown))
			if m.palette.sel != i {
				t.Fatalf("after %d downs selection = %d, want %d", i, m.palette.sel, i)
			}
		}
		for i := len(actionRegistry) - 2; i >= 0; i-- {
			m, _ = pressKey(t, m, specialMsg(tea.KeyUp))
			if m.palette.sel != i {
				t.Fatalf("selection = %d, want %d on the way back up", m.palette.sel, i)
			}
		}
	})
}

// TestNoMouseOffered fails when sources under internal/tui (excluding this
// file) wire mouse input: this slice offers keyboard navigation only, so
// AC-3.2 holds vacuously (AC-3.2, D9). The forbidden API tokens are assembled
// below so this test never names them, and the test fails when the scanned
// file set is empty so a wrong scan path cannot false-pass.
func TestNoMouseOffered(t *testing.T) {
	t.Parallel()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(self)
	var scanned []string
	var walk func(dir string)
	walk = func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if e.IsDir() {
				walk(p)
				continue
			}
			if strings.HasSuffix(p, ".go") && filepath.Base(p) != "nav_test.go" {
				scanned = append(scanned, p)
			}
		}
	}
	walk(root)
	if len(scanned) == 0 {
		t.Fatal("no sources scanned; the absence check would false-pass")
	}
	forbidden := []string{"Mou" + "seMsg", "WithMou" + "seCellMotion", "WithMou" + "seAllMotion"}
	for _, f := range scanned {
		body, err := os.ReadFile(f) // #nosec G304 -- test scans its own tree sources
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range forbidden {
			if strings.Contains(string(body), bad) {
				t.Errorf("%s wires mouse input", f)
			}
		}
	}
}

// TestPaletteViewOnlyActions asserts the palette selections this task owns:
// switching panes, toggling the theme, opening help and opening exit
// options. Stop/recover/apply/export close the palette without acting —
// their effects arrive in task 9, and faking them here would invent state.
func TestPaletteViewOnlyActions(t *testing.T) {
	t.Parallel()
	run := func(t *testing.T, name string) *Model {
		t.Helper()
		m := flowModel(t)
		m, _ = pressKey(t, m, runeMsg(":"))
		m = typeRunes(t, m, name)
		if got := len(m.filteredActions()); got != 1 {
			t.Fatalf("query %q matches %d actions, want 1", name, got)
		}
		m, _ = pressKey(t, m, specialMsg(tea.KeyEnter))
		if m.palette.open {
			t.Fatalf("after Enter the palette is still open")
		}
		return m
	}
	t.Run("switch pane cycles focus", func(t *testing.T) {
		t.Parallel()
		if m := run(t, "switch"); m.focusPane != paneAgents {
			t.Errorf("after Switch pane focus = %q, want agents", m.focusPane)
		}
	})
	t.Run("toggle theme swaps the token set", func(t *testing.T) {
		t.Parallel()
		before := flowModel(t).cfg.Tokens
		m := run(t, "toggle")
		if m.themeName != "light" {
			t.Fatalf("theme = %q, want light", m.themeName)
		}
		if m.cfg.Tokens == before {
			t.Errorf("tokens unchanged after Toggle theme")
		}
	})

	t.Run("toggle from light lands on dark", func(t *testing.T) {
		t.Parallel()
		light, err := theme.BuiltIn("light")
		if err != nil {
			t.Fatalf("BuiltIn(light): %v", err)
		}
		cfg := testConfig(fullCaps, t)
		cfg.Tokens, cfg.ThemeName = light, "light"
		m := newModel(cfg)
		m.toggleTheme()
		if m.themeName != "dark" {
			t.Fatalf("theme = %q, want dark", m.themeName)
		}
	})
	t.Run("help opens the help overlay", func(t *testing.T) {
		t.Parallel()
		if m := run(t, "help"); !m.helpOpen {
			t.Errorf("after the Help action help is not open")
		}
	})
	t.Run("exit options opens its dialog", func(t *testing.T) {
		t.Parallel()
		if m := run(t, "exit"); m.dialog != dialogExitOptions {
			t.Errorf("after Exit options dialog = %q, want exit-options", m.dialog)
		}
	})
	t.Run("task 9 actions dispatch through their gates", func(t *testing.T) {
		t.Parallel()
		// The ready_for_review flow model disables stop and recover: both
		// stay inert with the palette open on their visible reasons.
		for _, q := range []string{"stop", "rec"} {
			m := flowModel(t)
			m, _ = pressKey(t, m, runeMsg(":"))
			m = typeRunes(t, m, q)
			m, _ = pressKey(t, m, specialMsg(tea.KeyEnter))
			if !m.palette.open || m.dialog != "" {
				t.Errorf("query %q acted: palette open=%v dialog=%q", q, m.palette.open, m.dialog)
			}
		}
		// Apply is enabled and opens its confirmation.
		if m := run(t, "appl"); m.dialog != dialogApply {
			t.Errorf("after Apply candidate dialog = %q, want apply", m.dialog)
		}
		// Export runs immediately and reports through the result dialog.
		if m := run(t, "expo"); m.dialog != dialogDone {
			t.Errorf("after Export view dialog = %q, want done", m.dialog)
		}
	})
}

// TestFilterNeverHidesDetail asserts the applied filter never hides the
// selected-change pane: verification status stays visible while a filter is
// active (I07), and the agents pane still matches on its lane label.
func TestFilterNeverHidesDetail(t *testing.T) {
	t.Parallel()
	m := flowModel(t)
	m.focusPane = paneDetail
	m, _ = pressKey(t, m, runeMsg("/"))
	m = typeRunes(t, m, "zzz-no-such-row")
	view := m.View()
	for _, want := range []string{"added garnish", "verified ccccccc"} {
		if !strings.Contains(view, want) {
			t.Errorf("active filter hid detail content %q\n%s", want, view)
		}
	}
	lanes := flowModel(t)
	lanes.focusPane = paneAgents
	lanes, _ = pressKey(t, lanes, runeMsg("/"))
	lanes = typeRunes(t, lanes, "builtin/fake")
	if view := lanes.View(); !strings.Contains(view, "builtin/fake / trusted-host") {
		t.Errorf("lane filter hid the matching lane\n%s", view)
	}
	// The two-pane rung filters its agents panel too: detailColumn doubles
	// as the agents pane when detail is not focused.
	two := readyModel(t, fullCaps, 120, 30)
	two, _ = pressKey(t, two, specialMsg(tea.KeyTab))
	if two.focusPane != paneAgents {
		t.Fatalf("two-pane focus = %q, want agents", two.focusPane)
	}
	two, _ = pressKey(t, two, runeMsg("/"))
	two = typeRunes(t, two, "zzz-no-such-row")
	if view := two.View(); strings.Contains(view, "attempt 1") {
		t.Errorf("two-pane filter left the agents lane visible\n%s", view)
	}
}

// TestSelectedLaneDefaults pins the stable lane identity a reload keeps: the
// first load selects the attempt ID, and a run without an attempt selects
// the run ID instead of an empty lane.
func TestSelectedLaneDefaults(t *testing.T) {
	t.Parallel()
	m := newModel(testConfig(fullCaps, t))
	m.setSnapshot(t.Context(), richSnapshot())
	if m.selectedLane != "att_1" {
		t.Errorf("selected lane = %q, want the attempt ID", m.selectedLane)
	}
	bare := newModel(testConfig(fullCaps, t))
	snap := richSnapshot()
	snap.Attempt = nil
	snap.Candidate = nil
	snap.Verification = nil
	bare.setSnapshot(t.Context(), snap)
	if bare.selectedLane != snap.Run.RunID {
		t.Errorf("bare-run lane = %q, want the run ID", bare.selectedLane)
	}
}
