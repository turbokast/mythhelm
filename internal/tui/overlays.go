package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/turbokast/mythhelm/internal/tui/caps"
)

// Overlay cards are raised over the dimmed mission only at or above this
// size, and only when the card fits the body between the persistent chrome
// (design §6). Below it the overlay is full screen under the one-line header.
const (
	overlayMinWidth  = 100
	overlayMinHeight = 20
	// overlayChromeTop is header, rule, NOW/NEXT and rule; overlayChromeBottom
	// is the final rule and key bar. Both hold at two and three panes.
	overlayChromeTop    = 4
	overlayChromeBottom = 2
)

// overlayLabelRE splits a renderer-built "label: value" row; labels are
// renderer vocabulary, values were sanitised before the row was built.
var overlayLabelRE = regexp.MustCompile(`^(\s*)([A-Za-z][A-Za-z ]*:)( .*)?$`)

// overlayLines returns the open overlay's rows, or nil when none is open.
func (m *Model) overlayLines() []line {
	switch {
	case m.dialog != "":
		return m.dialogLines()
	case m.palette.open:
		return m.paletteLines()
	case m.helpOpen:
		return m.helpLines()
	}
	return nil
}

// overlayView composes the open overlay. It reads focus, dialog and theme
// state and writes none of it, so resizing never disturbs a pending effect.
func (m *Model) overlayView(rows []line) string {
	if !m.ready || m.loadErr != nil || m.empty {
		return m.render(rows)
	}
	title, content := m.overlayTitle(rows[0]), m.overlayContent(rows[1:])
	contentWidth := lipgloss.Width(title.text)
	for _, ln := range content {
		contentWidth = max(contentWidth, lipgloss.Width(ln.text))
	}
	cardWidth, cardHeight := contentWidth+4, len(content)+4
	bodyHeight := m.height - overlayChromeTop - overlayChromeBottom
	if m.width < overlayMinWidth || m.height < overlayMinHeight || cardWidth > m.width || cardHeight > bodyHeight {
		return m.render(append([]line{m.observatoryHeader(m.width), title}, content...))
	}
	return m.composeCard(title, content, contentWidth, cardHeight, bodyHeight)
}

// overlayTitle is the card's focused title: bold, underlined Text.
func (m *Model) overlayTitle(ln line) line {
	return lineFromSpans(span{text: ln.text, colour: m.cfg.Tokens.Text, bold: true, underline: true})
}

// overlayContent rebuilds card rows as semantic spans: one focus cell, bold
// Text labels, underlined unknown and no OK or Attention colour on a card.
func (m *Model) overlayContent(rows []line) []line {
	out := make([]line, len(rows))
	marker := focusGlyph(m.cfg.Caps) + " "
	for i, ln := range rows {
		switch {
		case len(ln.spans) > 0:
			spans := make([]span, len(ln.spans))
			for j, s := range ln.spans {
				spans[j] = s
				if s.colour != m.cfg.Tokens.Focus || s.text != focusGlyph(m.cfg.Caps) {
					spans[j].colour = m.cardColour(s.colour)
				}
			}
			out[i] = lineFromSpans(spans...)
		case m.dialog == dialogExitOptions && strings.HasPrefix(ln.text, marker):
			out[i] = m.focusRow(strings.TrimPrefix(ln.text, marker))
		default:
			out[i] = lineFromSpans(m.labelSpans(ln.text, m.cardColour(ln.colour))...)
		}
	}
	return out
}

// focusRow is a selected row: the focus marker is its only Focus cell.
func (m *Model) focusRow(text string) line {
	return lineFromSpans(span{text: focusGlyph(m.cfg.Caps), colour: m.cfg.Tokens.Focus}, span{text: " " + text, colour: m.cfg.Tokens.Text})
}

// cardColour keeps the card's foregrounds inside its vocabulary: OK and
// Attention never appear on a raised card, a whole-row Focus colour would
// paint more than one cell, and an uncoloured row is Text.
func (m *Model) cardColour(colour string) string {
	t := m.cfg.Tokens
	switch colour {
	case "", t.OK, t.Attention, t.Focus:
		return t.Text
	}
	return colour
}

// labelSpans emboldens a leading label and underlines each literal unknown.
func (m *Model) labelSpans(text, colour string) []span {
	t := m.cfg.Tokens
	var spans []span
	if g := overlayLabelRE.FindStringSubmatch(text); g != nil {
		label := t.Text
		if colour == t.Err {
			label = t.Err
		}
		spans = append(spans, span{text: g[1], colour: colour}, span{text: g[2], colour: label, bold: true})
		text = g[3]
	}
	return append(spans, unknownSpans(text, colour)...)
}

// unknownWord is the byte offset of the first whole word "unknown", or -1.
func unknownWord(s string) int {
	isWord := func(b byte) bool {
		return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
	}
	for off := 0; ; {
		i := strings.Index(s[off:], "unknown")
		if i < 0 {
			return -1
		}
		i += off
		end := i + len("unknown")
		if (i == 0 || !isWord(s[i-1])) && (end == len(s) || !isWord(s[end])) {
			return i
		}
		off = end
	}
}

// composeCard paints the card, centred in the body rectangle, over the freshly
// rendered mission recoloured TextMuted. Only the card carries a background.
func (m *Model) composeCard(title line, content []line, contentWidth, cardHeight, bodyHeight int) string {
	c, t := m.cfg.Caps, m.cfg.Tokens
	base := m.plainMission()
	x := (m.width - contentWidth - 4) / 2
	y := overlayChromeTop + (bodyHeight-cardHeight)/2
	rule := ruleGlyph(c)
	edge := []span{{text: strings.Repeat(rule, contentWidth+4), colour: t.Border}}
	card := make([][]span, 0, cardHeight)
	card = append(card, edge, m.cardRow(title, contentWidth))
	card = append(card, m.cardRow(lineFromSpans(span{text: strings.Repeat(rule, contentWidth), colour: t.Border}), contentWidth))
	for _, ln := range content {
		card = append(card, m.cardRow(ln, contentWidth))
	}
	card = append(card, edge)
	out := make([]string, len(base))
	for i, row := range base {
		if i < y || i >= y+cardHeight {
			out[i] = m.muted(row)
			continue
		}
		out[i] = m.muted(cutCells(row, 0, x)) +
			renderSpans(c, card[i-y], contentWidth+4, t.SurfaceRaised) +
			m.muted(cutCells(row, x+contentWidth+4, m.width))
	}
	return strings.Join(out, "\n")
}

// cardRow frames one content row with the vertical border and padding.
func (m *Model) cardRow(ln line, contentWidth int) []span {
	t := m.cfg.Tokens
	bar := span{text: vBarGlyph(m.cfg.Caps), colour: t.Border}
	pad := strings.Repeat(" ", contentWidth-lipgloss.Width(ln.text))
	row := []span{bar, {text: " "}}
	row = append(row, ln.spans...)
	return append(row, span{text: pad + " "}, bar)
}

// plainMission renders the mission without colour so the overlay can recolour
// it TextMuted; the renderer's own rows carry no other styling to preserve.
func (m *Model) plainMission() []string {
	saved := m.cfg.Caps
	m.cfg.Caps.Colour = caps.ColourNever
	plain := m.observatoryView()
	m.cfg.Caps = saved
	return strings.Split(plain, "\n")
}

// muted colours a plain base fragment TextMuted, never faint, keeping each
// literal unknown underlined.
func (m *Model) muted(s string) string {
	return renderSpans(m.cfg.Caps, unknownSpans(s, m.cfg.Tokens.TextMuted), lipgloss.Width(s), "")
}

// unknownSpans splits s into colour spans with each whole-word unknown
// underlined.
func unknownSpans(s, colour string) []span {
	var spans []span
	for s != "" {
		i := unknownWord(s)
		if i < 0 {
			return append(spans, span{text: s, colour: colour})
		}
		spans = append(spans, span{text: s[:i], colour: colour}, span{text: "unknown", colour: colour, underline: true})
		s = s[i+len("unknown"):]
	}
	return spans
}

// cutCells is the cells [from, to) of plain text, padded with spaces; a wide
// grapheme straddling either edge becomes spaces rather than a split glyph.
func cutCells(s string, from, to int) string {
	var b strings.Builder
	col := 0
	for s != "" {
		if col >= to {
			break
		}
		cluster, w := ansi.FirstGraphemeCluster(s, ansi.GraphemeWidth)
		s = s[len(cluster):]
		switch {
		case col >= from && col+w <= to:
			b.WriteString(cluster)
		case col+w > from:
			b.WriteString(strings.Repeat(" ", min(col+w, to)-max(col, from)))
		}
		col += w
	}
	return padCells(b.String(), to-from)
}
