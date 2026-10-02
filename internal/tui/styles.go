package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/turbokast/mythhelm/internal/security"
	"github.com/turbokast/mythhelm/internal/tui/caps"
)

// geometricShapes is the closed set of non-ASCII runes the mission view may
// render (AC-1.5, I13): box-drawing rules, the focus marker, the status dot,
// the middle-dot separator, the ellipsis and the em dash. Every other rune
// the view emits is ASCII, and ASCII caps use the plain alternatives below.
// TestNoFontDependentGlyphs asserts goldens stay inside this table (unicode)
// or byte-clean (ASCII), so adding a rune here is a deliberate
// load-bearing-font decision, never casual decoration.
const geometricShapes = "─│▶●·…—"

var tuiCellEscapes = strings.NewReplacer("\t", `\t`, "\n", `\n`)

// cell formats one stored value for a terminal line: control characters and
// escape sequences are inert (TermSafe, §12.7) and the tab/newline TermSafe
// keeps are shown escaped, mirroring the linear renderer's cell formatter.
func cell(s string) string {
	return tuiCellEscapes.Replace(security.TermSafe(s))
}

// cellOrUnknown formats a stored value that must never render blank (I09).
func cellOrUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return cell(s)
}

// glyph picks the unicode or ASCII rendering of one decoration.
func glyph(c caps.Caps, uni, ascii string) string {
	if c.Icons == caps.IconsUnicode {
		return uni
	}
	return ascii
}

// ruleGlyph is the horizontal pane rule. vBarGlyph is the column separator.
// focusGlyph marks the focused pane and selected row (it survives ColourNever
// by shape, AC-5.2). dotGlyph is the status dot. sepDotGlyph separates footer
// items. ellGlyph marks truncation (matching the diff renderer's marker
// choice). dashGlyph joins prose clauses. laneDotGlyph joins the lane letter
// to its attempt number.
func ruleGlyph(c caps.Caps) string    { return glyph(c, "─", "-") }
func vBarGlyph(c caps.Caps) string    { return glyph(c, "│", "|") }
func focusGlyph(c caps.Caps) string   { return glyph(c, "▶", ">") }
func dotGlyph(c caps.Caps) string     { return glyph(c, "●", "*") }
func sepDotGlyph(c caps.Caps) string  { return glyph(c, "·", "|") }
func ellGlyph(c caps.Caps) string     { return glyph(c, "…", "+") }
func dashGlyph(c caps.Caps) string    { return glyph(c, "—", "-") }
func laneDotGlyph(c caps.Caps) string { return glyph(c, "·", "-") }

// styleText colours s with a theme token hex, or returns s unchanged when
// colour is off. An invalid token renders unstyled (fg's contract), so an
// adversarial theme degrades to plain text, never broken escapes.
func styleText(c caps.Caps, s, hex string) string {
	if hex == "" || c.Colour == caps.ColourNever {
		return s
	}
	return fg(s, hex)
}

// line is one view row with its optional theme colour; empty colour renders
// plain. Rows are always fitted to their width before styling, so measurement
// never splits an escape sequence.
type line struct {
	text   string
	colour string
}

// fitLine fits one row's plain text to width cells.
func fitLine(c caps.Caps, s string, width int) string {
	if width < 1 {
		width = 1
	}
	return fitCells(s, width, ellGlyph(c))
}

// padCells pads plain text with spaces to exactly width cells.
func padCells(s string, width int) string {
	if w := lipgloss.Width(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}
