package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/turbokast/mythhelm/internal/tui/caps"
)

func TestObservatoryGlyphFallbacks(t *testing.T) {
	t.Parallel()
	if geometricShapes != "─│▶●·…—◆○◌✓" {
		t.Fatalf("closed glyph set = %q", geometricShapes)
	}
	old := []struct {
		fn         func(caps.Caps) string
		uni, ascii string
	}{
		{ruleGlyph, "─", "-"}, {vBarGlyph, "│", "|"}, {focusGlyph, "▶", ">"}, {dotGlyph, "●", "*"},
		{sepDotGlyph, "·", "|"}, {ellGlyph, "…", "+"}, {dashGlyph, "—", "-"}, {laneDotGlyph, "·", "-"},
	}
	fresh := []struct {
		fn         func(caps.Caps) string
		uni, ascii string
	}{
		{missionGlyph, "◆", "#"}, {pendingGlyph, "○", "o"}, {unknownGlyph, "◌", "."}, {checkGlyph, "✓", "v"},
	}
	dumb := caps.Resolve(caps.Prefs{Colour: "always", Motion: "full", Icons: "unicode"}, func(key string) string {
		if key == "TERM" {
			return "dumb"
		}
		return ""
	})
	for _, tt := range append(old, fresh...) {
		for _, c := range []caps.Caps{fullCaps, asciiCaps, {}, dumb} {
			want := tt.ascii
			if c.Icons == caps.IconsUnicode {
				want = tt.uni
			}
			if got := tt.fn(c); got != want || lipgloss.Width(got) != 1 {
				t.Errorf("glyph %+v = %q, want one cell %q", c, got, want)
			}
		}
	}
	seen := make(map[string]bool)
	for _, tt := range old {
		seen[tt.fn(asciiCaps)] = true
	}
	for _, tt := range fresh {
		got := tt.fn(asciiCaps)
		if seen[got] {
			t.Errorf("new fallback %q collides with another fallback", got)
		}
		seen[got] = true
	}
}

func TestSpanCellBounds(t *testing.T) {
	t.Parallel()
	const fg = "\x1b[38;2;17;34;51m"
	const bg = "\x1b[48;2;1;2;3m"
	const reset = "\x1b[0m"
	for _, tt := range []struct {
		name       string
		spans      []span
		width      int
		background string
		c          caps.Caps
		want       string
	}{
		{"zero", []span{{text: "hello"}}, 0, "", fullCaps, ""},
		{"negative", []span{{text: "hello"}}, -1, "", fullCaps, ""},
		{"one cell truncation", []span{{text: "hello"}}, 1, "", fullCaps, "…"},
		{"ASCII marker", []span{{text: "hello"}}, 1, "", asciiCaps, "+"},
		{"wide clipped whole", []span{{text: "A日B"}}, 3, "", fullCaps, "A…"},
		{"wide fits", []span{{text: "A日B"}}, 4, "", fullCaps, "A日B"},
		{"combining fits", []span{{text: "e\u0301x"}}, 2, "", fullCaps, "e\u0301x"},
		{"combining preserved", []span{{text: "e\u0301xy"}}, 2, "", fullCaps, "e\u0301…"},
		{"combining across spans", []span{{text: "e"}, {text: "\u0301xy"}}, 2, "", fullCaps, "e\u0301…"},
		{"joined emoji cluster", []span{{text: "👩\u200d💻x"}}, 2, "", fullCaps, "…"},
		{"basic foreground", []span{{text: "A", colour: "#112233"}}, 1, "", caps.Caps{Colour: caps.ColourBasic}, fg + "A" + reset},
		{"foreground reset", []span{{text: "A", colour: "#112233"}, {text: "B"}}, 2, "", fullCaps, fg + "A" + reset + "B"},
		{"emphasis reset", []span{{text: "A", bold: true, underline: true}, {text: "B"}}, 2, "", fullCaps, "\x1b[1;4mA" + reset + "B"},
		{"background restoration", []span{{text: "A", colour: "#112233"}, {text: "B"}}, 2, "#010203", fullCaps, bg + fg + "A" + reset + bg + "B" + reset},
		{"suppressed colours", []span{{text: "A", colour: "#112233", bold: true, underline: true}, {text: "B"}}, 2, "#010203", asciiCaps, "AB"},
		{"invalid foreground", []span{{text: "safe", colour: "#123"}}, 4, "", fullCaps, "safe"},
		{"control foreground", []span{{text: "safe", colour: "\x1b[31m"}}, 4, "", fullCaps, "safe"},
		{"invalid background", []span{{text: "safe"}}, 4, "\x1b[41m", fullCaps, "safe"},
		{"inert span controls", []span{{text: "a\x1b[31mb\x1b[0m\x1b]0;title\a\t\n"}}, 20, "", fullCaps, `ab\t\n`},
		{"empty", nil, 4, "#010203", fullCaps, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := renderSpans(tt.c, tt.spans, tt.width, tt.background)
			if got != tt.want {
				t.Errorf("renderSpans = %q, want %q", got, tt.want)
			}
			if !utf8.ValidString(got) || lipgloss.Width(got) > max(0, tt.width) {
				t.Errorf("invalid or overwide output: %q", got)
			}
			if tt.c.Colour == caps.ColourNever && (strings.Contains(got, "[38;") || strings.Contains(got, "[48;")) {
				t.Errorf("suppressed colour leaked: %q", got)
			}
		})
	}
}

func TestSpanRenderSeams(t *testing.T) {
	t.Parallel()
	m := newModel(testConfig(fullCaps, t))
	m.width, m.height = 4, 10
	ln := line{text: "mirror", colour: "#abcdef", spans: []span{{text: "A", colour: "#112233"}, {text: "B", bold: true}}}
	want := "\x1b[38;2;17;34;51mA\x1b[0m\x1b[1mB\x1b[0m"
	if got := m.render([]line{ln}); got != want {
		t.Errorf("render = %q, want %q", got, want)
	}
	if got := m.finaliseRows([]line{ln}, 4); len(got) != 1 || got[0] != want {
		t.Errorf("finaliseRows = %q", got)
	}
	if got := joinColumns(fullCaps, []column{{width: 4, rows: []line{ln}}}, "|"); len(got) != 1 || got[0] != want+"  " {
		t.Errorf("joinColumns = %q", got)
	}
	// Legacy rows can already carry trusted bounded diff styling. No sanitizer
	// or span override may erase those escapes or colour the padded tail anew.
	legacy := line{text: "\x1b[38;2;17;34;51m@@\x1b[0m"}
	if got := m.render([]line{legacy}); got != legacy.text {
		t.Errorf("legacy render = %q", got)
	}
	if got := m.finaliseRows([]line{legacy}, 4); got[0] != legacy.text {
		t.Errorf("legacy finalise = %q", got)
	}
	if got := joinColumns(fullCaps, []column{{width: 4, rows: []line{legacy}}}, "|"); got[0] != legacy.text+"  " {
		t.Errorf("legacy columns = %q", got)
	}
}
