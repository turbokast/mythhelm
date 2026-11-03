package tui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/tui/caps"
	"github.com/turbokast/mythhelm/internal/tui/theme"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

// sgrCell is one rendered terminal cell with the SGR state that painted it.
type sgrCell struct {
	r                      rune
	text                   string
	fg, bg                 string
	bold, underline, faint bool
}

// parseSGRCells parses the renderer's own final output (trusted test input)
// into cells per row. Only the SGR forms the renderer emits are understood;
// any other escape fails the test so a new form cannot slip past unparsed.
func parseSGRCells(t *testing.T, view string) [][]sgrCell {
	t.Helper()
	var rows [][]sgrCell
	for row := range strings.SplitSeq(view, "\n") {
		var cur sgrCell
		var cells []sgrCell
		for i := 0; i < len(row); {
			if row[i] == 0x1b {
				end := strings.IndexByte(row[i:], 'm')
				if end < 0 || i+1 >= len(row) || row[i+1] != '[' {
					t.Fatalf("unparsed escape in %q", row)
				}
				params := strings.Split(row[i+2:i+end], ";")
				for p := 0; p < len(params); p++ {
					switch params[p] {
					case "0", "":
						cur = sgrCell{}
					case "1":
						cur.bold = true
					case "2":
						cur.faint = true
					case "4":
						cur.underline = true
					case "38", "48":
						if p+4 >= len(params) || params[p+1] != "2" {
							t.Fatalf("unparsed colour SGR in %q", row)
						}
						var ch [3]int
						for k, v := range params[p+2 : p+5] {
							n, err := strconv.Atoi(v)
							if err != nil {
								t.Fatalf("bad SGR channel %q", v)
							}
							ch[k] = n
						}
						hex := fmt.Sprintf("#%02x%02x%02x", ch[0], ch[1], ch[2])
						if params[p] == "38" {
							cur.fg = hex
						} else {
							cur.bg = hex
						}
						p += 4
					default:
						t.Fatalf("unexpected SGR parameter %q in %q", params[p], row)
					}
				}
				i += end + 1
				continue
			}
			cluster, width := ansi.FirstGraphemeCluster(row[i:], ansi.GraphemeWidth)
			r, _ := utf8.DecodeRuneInString(cluster)
			for x := range width {
				c := cur
				if x == 0 {
					c.r, c.text = r, cluster
				}
				cells = append(cells, c)
			}
			i += len(cluster)
		}
		rows = append(rows, cells)
	}
	return rows
}

func sameHex(a, b string) bool { return a != "" && strings.EqualFold(a, b) }

func plainRow(cells []sgrCell) string {
	var b strings.Builder
	for _, c := range cells {
		if c.text != "" {
			b.WriteString(c.text)
		} else if c.r != 0 {
			b.WriteRune(c.r)
		}
	}
	return b.String()
}

// rect is a card rectangle in cells, inclusive top/left, exclusive bottom/right.
type rect struct{ top, left, bottom, right int }

func (r rect) contains(y, x int) bool {
	return y >= r.top && y < r.bottom && x >= r.left && x < r.right
}

// raisedRect is the bounding box of every painted background cell; ok is
// false when nothing carries a background.
func raisedRect(rows [][]sgrCell) (rect, bool) {
	r := rect{top: -1}
	for y, row := range rows {
		for x, c := range row {
			if c.bg == "" {
				continue
			}
			if r.top < 0 {
				r = rect{top: y, left: x, bottom: y + 1, right: x + 1}
			}
			r.left, r.right = min(r.left, x), max(r.right, x+1)
			r.bottom = y + 1
		}
	}
	return r, r.top >= 0
}

// cardViolations lists every semantic-span breach of a raised-card capture.
// It is the predicate the acceptance tests and their kept negative controls
// share, so a control that stays green proves the predicate bites.
func cardViolations(rows [][]sgrCell, tok theme.Tokens, bodyTop, bodyBottom int) []string {
	var out []string
	card, ok := raisedRect(rows)
	if !ok {
		return []string{"no raised card"}
	}
	if card.top < bodyTop || card.bottom > bodyBottom {
		out = append(out, fmt.Sprintf("card rows %d-%d leave body %d-%d", card.top, card.bottom, bodyTop, bodyBottom))
	}
	focus := 0
	for y, row := range rows {
		for x, c := range row {
			in := card.contains(y, x)
			switch {
			case c.faint:
				out = append(out, fmt.Sprintf("faint cell at %d,%d", y, x))
			case in && !sameHex(c.bg, tok.SurfaceRaised):
				out = append(out, fmt.Sprintf("card cell %d,%d background %q, want SurfaceRaised", y, x, c.bg))
			case !in && c.bg != "":
				out = append(out, fmt.Sprintf("background %q outside card at %d,%d", c.bg, y, x))
			case !in && c.r != ' ' && !sameHex(c.fg, tok.TextMuted):
				out = append(out, fmt.Sprintf("base cell %d,%d %q fg %q, want TextMuted", y, x, string(c.r), c.fg))
			case in && sameHex(c.fg, tok.OK):
				out = append(out, fmt.Sprintf("OK colour on card at %d,%d", y, x))
			case in && sameHex(c.fg, tok.Attention):
				out = append(out, fmt.Sprintf("Attention colour on card at %d,%d", y, x))
			}
			if in && sameHex(c.fg, tok.Focus) {
				focus++
			}
			border := y == card.top || y == card.bottom-1 || x == card.left || x == card.right-1
			if in && border && !strings.ContainsRune("─│-|", c.r) {
				out = append(out, fmt.Sprintf("border cell %d,%d is %q", y, x, string(c.r)))
			}
			if in && border && (unicode.IsLetter(c.r) || unicode.IsDigit(c.r)) {
				out = append(out, fmt.Sprintf("alphanumeric border cell %d,%d", y, x))
			}
		}
	}
	if focus > 1 {
		out = append(out, fmt.Sprintf("%d Focus cells on card, want at most one", focus))
	}
	return out
}

func viewOf(t *testing.T, m *Model) [][]sgrCell {
	t.Helper()
	return parseSGRCells(t, m.View())
}

// overlayModel is a ready_for_review model with wired fake actions, sized via
// Update like the program.
func overlayModel(t *testing.T, c caps.Caps, themeName string, width, height int) *Model {
	t.Helper()
	m := actionModel(t, c, richSnapshot(), (&fakeActions{}).actions())
	if themeName == "light" {
		m.toggleTheme()
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return updated.(*Model)
}

// openOverlay opens one overlay through the real key path.
func openOverlay(t *testing.T, m *Model, name string) *Model {
	t.Helper()
	keys := map[string]string{"help": "?", "palette": ":", "exit": "q", "apply": "a"}
	m, _ = pressKey(t, m, runeMsg(keys[name]))
	open := map[string]bool{"help": m.helpOpen, "palette": m.palette.open, "exit": m.dialog == dialogExitOptions, "apply": m.dialog == dialogApply}
	if !open[name] {
		t.Fatalf("%s overlay did not open through its key", name)
	}
	return m
}

// TestObservatoryOverlayCards pins AC-5.1/5.2: at 140x40 every card is raised
// over the dimmed mission with the header, NOW/NEXT row and key bar intact;
// below the card threshold the overlay is full screen under the header.
func TestObservatoryOverlayCards(t *testing.T) {
	t.Parallel()
	for _, themeName := range []string{"dark", "light"} {
		for _, name := range []string{"help", "palette", "exit", "apply"} {
			t.Run(themeName+"/"+name, func(t *testing.T) {
				t.Parallel()
				m := overlayModel(t, fullCaps, themeName, 140, 40)
				base := viewOf(t, m)
				m = openOverlay(t, m, name)
				rows := viewOf(t, m)
				if len(rows) != 40 {
					t.Fatalf("overlay view has %d rows, want 40", len(rows))
				}
				for _, y := range []int{0, 1, 2, 3, 38, 39} {
					if got, want := plainRow(rows[y]), plainRow(base[y]); got != want {
						t.Errorf("chrome row %d changed under %s card:\n got %q\nwant %q", y, name, got, want)
					}
				}
				if row := plainRow(rows[2]); !strings.Contains(row, "NOW") || !strings.Contains(row, "NEXT") {
					t.Errorf("NOW/NEXT row lost: %q", row)
				}
				for _, v := range cardViolations(rows, m.cfg.Tokens, overlayChromeTop, 40-overlayChromeBottom) {
					t.Error(v)
				}
				card, _ := raisedRect(rows)
				title := plainRow(rows[card.top+1][card.left:card.right])
				if c := rows[card.top+1][card.left+2]; !c.bold || !c.underline || !sameHex(c.fg, m.cfg.Tokens.Text) {
					t.Errorf("card title %q first cell = %+v, want bold underlined Text", title, c)
				}
			})
		}
	}
	for _, size := range [][2]int{{99, 40}, {140, 19}} {
		t.Run(fmt.Sprintf("fallback/%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()
			m := overlayModel(t, fullCaps, "dark", size[0], size[1])
			header := plainRow(viewOf(t, m)[0])
			m = openOverlay(t, m, "help")
			rows := viewOf(t, m)
			if _, raised := raisedRect(rows); raised {
				t.Errorf("fallback paints a raised card at %dx%d", size[0], size[1])
			}
			if got := plainRow(rows[0]); got != header {
				t.Errorf("fallback header = %q, want the one-line title/state %q", got, header)
			}
			if got := strings.TrimSpace(plainRow(rows[1])); got != "HELP" {
				t.Errorf("fallback row 1 = %q, want the overlay title", got)
			}
		})
	}
}

// TestCardViolationsBite keeps the broken inputs: a background outside the
// card, OK on the card and a second Focus cell must each be reported.
func TestCardViolationsBite(t *testing.T) {
	t.Parallel()
	tok := darkTokens(t)
	c := fullCaps
	card := func(spans ...span) string { return renderSpans(c, spans, 6, tok.SurfaceRaised) }
	edge := card(span{text: "──────", colour: tok.Border})
	muted := renderSpans(c, []span{{text: "ab", colour: tok.TextMuted}}, 2, "")
	side := func(mid span) string {
		return muted + card(span{text: "│", colour: tok.Border}, mid, span{text: "│", colour: tok.Border})
	}
	plainMid := side(span{text: "    ", colour: tok.Text})
	cases := []struct{ name, view, want string }{
		{"background outside card", muted + edge + "\n" + renderSpans(c, []span{{text: "ab", colour: tok.TextMuted}}, 2, tok.Surface) + card(span{text: "│    │", colour: tok.Border}) + "\n" + muted + edge, "background"},
		{"OK on card", muted + edge + "\n" + side(span{text: "ok  ", colour: tok.OK}) + "\n" + muted + edge, "OK colour"},
		{"two focus cells", muted + edge + "\n" + side(span{text: "▶▶  ", colour: tok.Focus}) + "\n" + muted + edge, "Focus cells"},
		{"text in border", muted + card(span{text: "──x───", colour: tok.Border}) + "\n" + plainMid + "\n" + muted + edge, "border cell"},
	}
	clean := parseSGRCells(t, muted+edge+"\n"+plainMid+"\n"+muted+edge)
	if v := cardViolations(clean, tok, 0, 3); len(v) != 0 {
		t.Fatalf("clean synthetic card reports %q", v)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := strings.Join(cardViolations(parseSGRCells(t, tc.view), tok, 0, 3), "\n")
			if !strings.Contains(got, tc.want) {
				t.Errorf("violations %q lack %q", got, tc.want)
			}
		})
	}
}

// TestOverlayKeepsUpdating pins AC-5.4/4.9: a journaled progress event with
// help open recolours a changed dimmed NOW row without closing the overlay,
// and a resize through the fallback keeps the pending effect and theme.
func TestOverlayKeepsUpdating(t *testing.T) {
	t.Parallel()
	t.Run("live event under help", func(t *testing.T) {
		t.Parallel()
		b := newJournalBuilder(t)
		t.Cleanup(func() { _ = b.j.Close() })
		const runID = "run_overlay"
		b.addRun(runID, "executing", b.writeTask(runID, []byte("# Task\n")))
		attemptID := b.addAttempt(runID, "running")
		cfg := testConfig(fullCaps, t)
		cfg.RunID, cfg.StateDir = runID, b.dir
		m := newModel(cfg)
		snap, err := viewmodel.Load(t.Context(), b.dir, runID)
		if err != nil {
			t.Fatal(err)
		}
		m.setSnapshot(t.Context(), snap)
		m.width, m.height = 160, 40
		m, _ = pressKey(t, m, runeMsg("?"))
		before := viewOf(t, m)
		ev := journal.Event{SchemaVersion: journal.EnvelopeVersion, EventID: "evt_overlay_progress", ProducerID: "overlay", ProducerSequence: 1, Generation: 1, ObservedAt: time.Now().UTC(), Type: "attempt.progress", RunID: runID, AttemptID: attemptID, Payload: json.RawMessage(`{"assistant_turns":15,"tool_uses":{"Read":8,"Edit":2,"Bash":3}}`)}
		if err := b.j.Append(t.Context(), ev, nil); err != nil {
			t.Fatal(err)
		}
		stored, err := viewmodel.Load(t.Context(), b.dir, runID)
		if err != nil || stored.LatestProgress == nil || stored.LatestProgress.AssistantTurns != 15 {
			t.Fatalf("stored progress = %+v, err %v", stored.LatestProgress, err)
		}
		updated, _ := m.Update(pollTickMsg{})
		m = deliverReload(t, updated.(*Model))
		if !m.helpOpen {
			t.Fatal("live event closed the help overlay")
		}
		after := viewOf(t, m)
		if !backgroundChanged(before, after) {
			t.Errorf("dimmed NOW row did not follow the event:\n%q", plainRow(after[2]))
		}
		if !strings.Contains(plainRow(after[2]), "15") {
			t.Errorf("dimmed NOW row lacks the new turn count: %q", plainRow(after[2]))
		}
		for _, c := range after[2] {
			if c.r != ' ' && !sameHex(c.fg, m.cfg.Tokens.TextMuted) {
				t.Fatalf("NOW row under help not dimmed: %q fg %q", string(c.r), c.fg)
			}
		}
		if before, after := cardText(before), cardText(after); before != after {
			t.Errorf("help card changed on a background event:\n%s\n---\n%s", before, after)
		}
	})
	t.Run("resize through fallback keeps the pending effect", func(t *testing.T) {
		t.Parallel()
		m := overlayModel(t, fullCaps, "light", 140, 40)
		m = openOverlay(t, m, "apply")
		m, _ = pressKey(t, m, runeMsg("x"))
		m, _ = pressKey(t, m, specialMsg(tea.KeyTab))
		m, _ = pressKey(t, m, runeMsg(" "))
		type state struct {
			dialog, branch, focusPane, theme string
			field, focus                     int
			flags                            bool
			tokens                           theme.Tokens
		}
		snapshot := func() state {
			return state{m.dialog, m.applyBranch, m.focusPane, m.themeName, m.dialogField, m.dialogFocus, m.acceptFlags, m.cfg.Tokens}
		}
		want := snapshot()
		if !want.flags || want.field != applyFieldFlags {
			t.Fatalf("setup did not reach the flags toggle: %+v", want)
		}
		for _, size := range [][2]int{{80, 15}, {140, 40}} {
			updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m = updated.(*Model)
			rows := viewOf(t, m)
			_, raised := raisedRect(rows)
			if raised != (size[0] == 140) {
				t.Errorf("%dx%d raised card = %v", size[0], size[1], raised)
			}
			if got := snapshot(); got != want {
				t.Errorf("%dx%d changed dialog state:\n got %+v\nwant %+v", size[0], size[1], got, want)
			}
		}
	})
}

func backgroundChanged(before, after [][]sgrCell) bool {
	return plainRow(before[2]) != plainRow(after[2])
}

// cardText is the plain text of the raised card.
func cardText(rows [][]sgrCell) string {
	card, ok := raisedRect(rows)
	if !ok {
		return ""
	}
	var b strings.Builder
	for y := card.top; y < card.bottom; y++ {
		b.WriteString(plainRow(rows[y][card.left:card.right]) + "\n")
	}
	return b.String()
}

// TestSemanticSGRRuns pins AC-1.5–1.9 on final emitted cells for both themes:
// default views, palette/apply/diff and overlay captures. Goldens are
// synthetic fixture renders, never native evidence.
func TestSemanticSGRRuns(t *testing.T) {
	t.Parallel()
	type capture struct {
		name    string
		w, h    int
		overlay string
		diff    bool
	}
	captures := []capture{
		{name: "default 160x40", w: 160, h: 40},
		{name: "default 88x24", w: 88, h: 24},
		{name: "diff 160x40", w: 160, h: 40, diff: true},
		{name: "palette 140x40", w: 140, h: 40, overlay: "palette"},
		{name: "apply 140x40", w: 140, h: 40, overlay: "apply"},
		{name: "help 140x40", w: 140, h: 40, overlay: "help"},
	}
	for _, themeName := range []string{"dark", "light"} {
		for _, cp := range captures {
			t.Run(themeName+"/"+cp.name, func(t *testing.T) {
				t.Parallel()
				m := overlayModel(t, fullCaps, themeName, cp.w, cp.h)
				tok := m.cfg.Tokens
				if cp.diff {
					m, _ = pressKey(t, m, specialMsg(tea.KeyEnter))
				}
				if cp.overlay != "" {
					m = openOverlay(t, m, cp.overlay)
				}
				rows := viewOf(t, m)
				if cp.overlay != "" {
					for _, v := range cardViolations(rows, tok, overlayChromeTop, cp.h-overlayChromeBottom) {
						t.Error(v)
					}
					card, _ := raisedRect(rows)
					focus := 0
					for y := card.top; y < card.bottom; y++ {
						for _, c := range rows[y][card.left:card.right] {
							if sameHex(c.fg, tok.Focus) {
								focus++
							}
						}
					}
					if cp.overlay != "help" && focus != 1 {
						t.Errorf("%s card has %d Focus cells, want exactly one", cp.overlay, focus)
					}
				}
				for y, row := range rows {
					checkFocusRuns(t, y, row, tok)
					checkAttention(t, y, row, tok)
					if cp.overlay == "" || y < overlayChromeTop {
						checkUnknownUnderlined(t, y, row, cp.overlay != "")
					}
				}
			})
		}
	}
	t.Run("apply card recolours verified OK", func(t *testing.T) {
		t.Parallel()
		m := openOverlay(t, overlayModel(t, fullCaps, "dark", 140, 40), "apply")
		rows := viewOf(t, m)
		text := cardText(rows)
		if !strings.Contains(text, "verif") {
			t.Fatalf("apply card lacks the verified row:\n%s", text)
		}
	})
	for _, cp := range captures {
		t.Run("suppressed/"+cp.name, func(t *testing.T) {
			t.Parallel()
			m := overlayModel(t, asciiCaps, "dark", cp.w, cp.h)
			if cp.overlay != "" {
				m = openOverlay(t, m, cp.overlay)
			}
			for y, row := range viewOf(t, m) {
				for x, c := range row {
					if c.fg != "" || c.bg != "" {
						t.Fatalf("colour under suppression at %d,%d: fg %q bg %q", y, x, c.fg, c.bg)
					}
				}
			}
		})
	}
}

// checkFocusRuns: every Focus-coloured run is one cell (the marker only).
func checkFocusRuns(t *testing.T, y int, row []sgrCell, tok theme.Tokens) {
	t.Helper()
	run := 0
	for x, c := range append(row, sgrCell{}) {
		if sameHex(c.fg, tok.Focus) {
			run++
			continue
		}
		if run > 1 {
			t.Errorf("row %d: %d-cell Focus run ending at %d, want one cell", y, run, x)
		}
		run = 0
	}
}

// checkAttention: Attention paints only the needs-you slot.
func checkAttention(t *testing.T, y int, row []sgrCell, tok theme.Tokens) {
	t.Helper()
	var run strings.Builder
	for _, c := range append(row, sgrCell{}) {
		if sameHex(c.fg, tok.Attention) {
			run.WriteRune(c.r)
			continue
		}
		if run.Len() > 0 && !strings.Contains(run.String(), "needs you") {
			t.Errorf("row %d: Attention paints %q outside the needs-you slot", y, run.String())
		}
		run.Reset()
	}
}

// checkUnknownUnderlined: the literal word unknown is underlined in full.
func checkUnknownUnderlined(t *testing.T, y int, row []sgrCell, card bool) {
	t.Helper()
	text := plainRow(row)
	for off := 0; ; {
		i := unknownWord(text[off:])
		if i < 0 {
			return
		}
		start := ansi.StringWidth(text[:off+i])
		for x := start; x < start+len("unknown"); x++ {
			if !row[x].underline {
				t.Errorf("row %d: unknown at cell %d not underlined (card=%v): %q", y, start, card, text)
				return
			}
		}
		off += i + len("unknown")
	}
}

func TestReviewCutCellsPreservesSanitizedGraphemes(t *testing.T) {
	for _, tc := range []struct {
		name, text, want string
		width            int
	}{
		{"combining-accent", "e\u0301x", "e\u0301", 1},
		{"joined-emoji", "\U0001f469\u200d\U0001f4bbx", "\U0001f469\u200d\U0001f4bb", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			safe := cell(tc.text)
			if safe != tc.text {
				t.Fatalf("TermSafe path unexpectedly changed benign grapheme: %q", safe)
			}
			got := cutCells(safe, 0, tc.width)
			if got != tc.want {
				t.Fatalf("actual compositor cutCells on sanitised input=%q, want intact grapheme %q", got, tc.want)
			}
		})
	}
}
func TestReviewSGRParserCountsTerminalCells(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		want       int
	}{
		{"wide", "界", 2}, {"combining", "e\u0301", 1}, {"joined", "\U0001f469\u200d\U0001f4bb", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := parseSGRCells(t, tc.text)
			if got := len(rows[0]); got != tc.want {
				t.Fatalf("SGR oracle counts %d rune entries, want %d terminal cells for %q", got, tc.want, tc.text)
			}
		})
	}
}

func TestIndependentOverlayGraphemeBoundaryCells(t *testing.T) {
	for _, tc := range []struct {
		s        string
		from, to int
		want     string
	}{
		{"界a", 0, 1, " "}, {"界a", 1, 3, " a"}, {"Ae\u0301B", 1, 2, "e\u0301"},
		{"👩\u200d💻x", 0, 1, " "}, {"👩\u200d💻x", 1, 3, " x"}, {"Ae\u0301B", 0, 3, "Ae\u0301B"},
	} {
		if got := cutCells(tc.s, tc.from, tc.to); got != tc.want {
			t.Errorf("cutCells(%q,%d,%d)=%q want %q", tc.s, tc.from, tc.to, got, tc.want)
		}
	}
	tok := darkTokens(t)
	text := "界 e\u0301 👩\u200d💻 unknown"
	view := renderSpans(fullCaps, unknownSpans(text, tok.Text), ansi.StringWidth(text), tok.SurfaceRaised)
	rows := parseSGRCells(t, view)
	if got := plainRow(rows[0]); got != text {
		t.Fatalf("plain text=%q want %q", got, text)
	}
	if got := len(rows[0]); got != ansi.StringWidth(text) {
		t.Fatalf("cells=%d want %d", got, ansi.StringWidth(text))
	}
	for _, i := range []int{0, 1, 5, 6} {
		if !sameHex(rows[0][i].fg, tok.Text) || !sameHex(rows[0][i].bg, tok.SurfaceRaised) {
			t.Errorf("cell %d lost SGR: %+v", i, rows[0][i])
		}
	}
	if rows[0][1].r != 0 || rows[0][1].text != "" {
		t.Errorf("wide continuation duplicates text: %+v", rows[0][1])
	}
	checkUnknownUnderlined(t, 0, rows[0], true)
}
