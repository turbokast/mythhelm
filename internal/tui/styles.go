package tui

import (
	"fmt"
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
const geometricShapes = "─│▶●·…—◆○◌✓"

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
	spans  []span
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

// span carries plain terminal text and its declarative foreground/emphasis.
type span struct {
	text, colour    string
	bold, underline bool
}

func missionGlyph(c caps.Caps) string { return glyph(c, "◆", "#") }
func pendingGlyph(c caps.Caps) string { return glyph(c, "○", "o") }
func unknownGlyph(c caps.Caps) string { return glyph(c, "◌", ".") }
func checkGlyph(c caps.Caps) string   { return glyph(c, "✓", "v") }

// lineFromSpans retains a plain mirror for filtering and legacy row inspection.
// Callers sanitise stored values before constructing spans. No styled diff row
// passes through this path: legacy rows may already contain trusted SGR.
func lineFromSpans(spans ...span) line {
	var text strings.Builder
	for _, s := range spans {
		text.WriteString(s.text)
	}
	return line{text: text.String(), spans: spans}
}

// renderSpans clips plain graphemes before adding trusted SGR. ColourNever
// preserves the incumbent unstyled mode (the design also permits emphasis).
// A valid background is restored at the next span after each reset, and the
// final reset leaves the terminal's own background in place outside the row.
func renderSpans(c caps.Caps, spans []span, width int, background string) string {
	if width <= 0 {
		return ""
	}
	clean := make([]span, len(spans))
	var plain strings.Builder
	for i, s := range spans {
		clean[i] = s
		// Defence in depth for the plain span boundary, never final styled SGR.
		clean[i].text = cell(s.text)
		plain.WriteString(clean[i].text)
	}
	remaining := plain.Len()
	marker := ""
	if lipgloss.Width(plain.String()) > width {
		marker = ellGlyph(c)
		prefix := ""
		if budget := width - lipgloss.Width(marker); budget > 0 {
			// Lipgloss truncates grapheme clusters, including combining marks
			// across span boundaries, without adding a new dependency.
			prefix = lipgloss.NewStyle().Inline(true).MaxWidth(budget).Render(plain.String())
		}
		remaining = len(prefix)
	}
	var out strings.Builder
	bg := ""
	if c.Colour != caps.ColourNever {
		bg = colourSGR(background, true)
	}
	write := func(s span) {
		if s.text == "" {
			return
		}
		out.WriteString(bg)
		styled := false
		if c.Colour != caps.ColourNever {
			var emphasis []string
			if s.bold {
				emphasis = append(emphasis, "1")
			}
			if s.underline {
				emphasis = append(emphasis, "4")
			}
			if len(emphasis) > 0 {
				out.WriteString("\x1b[" + strings.Join(emphasis, ";") + "m")
				styled = true
			}
			if fg := colourSGR(s.colour, false); fg != "" {
				out.WriteString(fg)
				styled = true
			}
		}
		out.WriteString(s.text)
		if styled || bg != "" {
			out.WriteString("\x1b[0m")
		}
	}
	for _, s := range clean {
		if remaining == 0 {
			break
		}
		n := min(remaining, len(s.text))
		s.text = s.text[:n]
		write(s)
		remaining -= n
	}
	write(span{text: marker})
	return out.String()
}

func colourSGR(hex string, background bool) string {
	r, g, b, ok := parseHexColour(hex)
	if !ok {
		return ""
	}
	channel := 38
	if background {
		channel = 48
	}
	return fmt.Sprintf("\x1b[%d;2;%d;%d;%dm", channel, r, g, b)
}

// renderLine preserves legacy pre-styled rows and gives spans precedence.
func renderLine(c caps.Caps, ln line, width int) string {
	if len(ln.spans) > 0 {
		return renderSpans(c, ln.spans, width, "")
	}
	return styleText(c, fitLine(c, ln.text, width), ln.colour)
}
