package tui

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/turbokast/mythhelm/internal/security"
	"github.com/turbokast/mythhelm/internal/tui/caps"
	"github.com/turbokast/mythhelm/internal/tui/theme"
)

// Diff input caps (D7): unbounded candidate diffs must neither blow the
// retained buffer nor the width-measurement work. Inputs past either cap set
// Truncated and render an explicit labelled footer, never silently dropped
// rows. The byte cap fires first when both would apply.
const (
	maxDiffLines = 50000
	maxDiffBytes = 4 << 20 // 4 MiB: same order as 50k typical lines
)

// TruncateCap values naming which cap fired.
const (
	truncateLines = "lines"
	truncateBytes = "bytes"
)

// emptyDiffLabel renders for an empty diff so absence is labelled, never a
// silent empty success (I09).
const emptyDiffLabel = "(empty diff)"

// tabWidth is the tab stop rendering uses when expanding tabs.
const tabWidth = 8

// errMalformedDiff marks input that is not a parseable unified diff.
var errMalformedDiff = errors.New("malformed unified diff")

// DiffLineKind classifies one flattened diff row for structural highlighting.
type DiffLineKind int

const (
	// DiffFileBoundary is a "diff --git ..." file separator row.
	DiffFileBoundary DiffLineKind = iota
	// DiffFileMeta is a file-level row: index, mode, rename, ---/+++ or Binary.
	DiffFileMeta
	// DiffHunkHeader is an "@@ ... @@ " hunk header row.
	DiffHunkHeader
	// DiffContext is an unchanged " " row (including no-newline markers).
	DiffContext
	// DiffAdd is an added "+" row.
	DiffAdd
	// DiffDel is a removed "-" row.
	DiffDel
)

// DiffLine is one flattened render row of a parsed diff.
type DiffLine struct {
	Kind DiffLineKind
	Text string
}

// DiffHunk is one parsed hunk with its body rows.
type DiffHunk struct {
	Header             string
	OldStart, OldCount int
	NewStart, NewCount int
	Lines              []DiffLine // body rows only; the header renders separately
}

// DiffFile is one file's parsed hunks plus its header rows.
type DiffFile struct {
	OldPath, NewPath string
	Header           []DiffLine // file-level rows: boundary plus meta
	Hunks            []DiffHunk
}

// Diff is a parsed unified diff ready to render width-safe.
type Diff struct {
	Files       []DiffFile
	Lines       []DiffLine // flattened render rows in file order
	Truncated   bool
	TruncateCap string // truncateLines or truncateBytes; meaningful only when Truncated
	total       int    // input line count scanned from the full pre-cap input
}

// hunkHeader matches "@@ -old[,count] +new[,count] @@", with an optional
// trailing section heading. Omitted counts mean 1.
var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// TotalLines reports the full pre-cap input line count, the total the
// truncation footer shows the retained rows over. The selected-change pane
// (task 6) needs it for its provenance-carrying footer; recomputing it there
// would duplicate the trailing-newline rule.
func (d Diff) TotalLines() int {
	return d.total
}

// ParseDiff parses unified diff bytes, as produced by review.go's plain git
// diff invocation (no colour, ext-diff or textconv), into hunks. Inputs past
// maxDiffBytes are cut back to the last newline within budget before parsing
// so no partial row is kept; inputs past maxDiffLines keep the first
// maxDiffLines rows. Either cap sets Truncated with TruncateCap naming it.
// Malformed input returns an error wrapping errMalformedDiff rather than a
// half-diff; empty input parses to an empty Diff with no error.
func ParseDiff(unified []byte) (Diff, error) {
	var d Diff
	d.total = countLines(unified)
	retained := unified
	if len(unified) > maxDiffBytes {
		retained = unified[:0]
		if cut := bytes.LastIndexByte(unified[:maxDiffBytes], '\n'); cut >= 0 {
			retained = unified[:cut+1]
		}
		d.Truncated = true
		d.TruncateCap = truncateBytes
	}
	lines := splitLines(retained)
	if len(lines) > maxDiffLines {
		lines = lines[:maxDiffLines]
		if !d.Truncated {
			d.Truncated = true
			d.TruncateCap = truncateLines
		}
	}
	p := diffParser{truncated: d.Truncated}
	if err := p.parse(lines); err != nil {
		return Diff{}, err
	}
	d.Files = p.files
	if len(d.Files) == 0 && len(unified) > 0 && !d.Truncated {
		return Diff{}, fmt.Errorf("%w: no files or hunks in %d lines", errMalformedDiff, d.total)
	}
	for _, f := range d.Files {
		d.Lines = append(d.Lines, f.Header...)
		for _, h := range f.Hunks {
			d.Lines = append(d.Lines, DiffLine{Kind: DiffHunkHeader, Text: h.Header})
			d.Lines = append(d.Lines, h.Lines...)
		}
	}
	return d, nil
}

// countLines counts input lines in O(1) memory: newlines plus one for a final
// unterminated line.
func countLines(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	n := bytes.Count(b, []byte{'\n'})
	if b[len(b)-1] != '\n' {
		n++
	}
	return n
}

// splitLines splits retained bytes into lines, dropping the empty tail after
// a final newline and tolerating CRLF endings.
func splitLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	raw := strings.Split(string(b), "\n")
	if raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	for i, l := range raw {
		raw[i] = strings.TrimSuffix(l, "\r")
	}
	return raw
}

// diffParser is the single-pass unified-diff state machine.
type diffParser struct {
	truncated    bool
	files        []DiffFile
	current      *DiffFile
	hunk         *DiffHunk
	wantOld      int // old-side body rows the open hunk still needs
	wantNew      int // new-side body rows the open hunk still needs
	hunkOpen     bool
	hunkComplete bool
}

// parse consumes retained input lines into files and hunks.
func (p *diffParser) parse(lines []string) error {
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			if err := p.closeHunk(); err != nil {
				return err
			}
			p.openFile()
			old, new := splitDiffGit(strings.TrimPrefix(line, "diff --git "))
			p.current.OldPath, p.current.NewPath = old, new
			p.current.Header = append(p.current.Header, DiffLine{Kind: DiffFileBoundary, Text: line})
		case strings.HasPrefix(line, "@@"):
			m := hunkHeader.FindStringSubmatch(line)
			if m == nil {
				return fmt.Errorf("%w: bad hunk header %q", errMalformedDiff, line)
			}
			if err := p.closeHunk(); err != nil {
				return err
			}
			p.openHunk(line, m)
		case p.inBody():
			if err := p.bodyLine(line); err != nil {
				return err
			}
		case p.hunkOpen && strings.HasPrefix(line, "\\"):
			// A trailing "\ No newline" marker after the hunk's counted rows
			// are complete: kept as a context row like the mid-hunk marker
			// path in bodyLine, never skipped as preamble.
			p.hunk.Lines = append(p.hunk.Lines, DiffLine{Kind: DiffContext, Text: line})
		default:
			p.metaLine(line)
		}
	}
	if p.hunkOpen && !p.hunkComplete && !p.truncated {
		return fmt.Errorf("%w: hunk %q ends with %d old and %d new rows missing",
			errMalformedDiff, p.hunk.Header, p.wantOld, p.wantNew)
	}
	return nil
}

// inBody reports whether the open hunk still expects body rows.
func (p *diffParser) inBody() bool {
	return p.hunkOpen && !p.hunkComplete
}

// openFile starts a file, closing nothing: callers close the hunk first.
func (p *diffParser) openFile() {
	p.files = append(p.files, DiffFile{})
	p.current = &p.files[len(p.files)-1]
	p.hunk = nil
	p.hunkOpen = false
}

// ensureFile starts an implicit file for headers outside any "diff --git"
// block, so plain -u output and bare hunks still parse.
func (p *diffParser) ensureFile() {
	if p.current == nil {
		p.openFile()
	}
}

// openHunk starts a hunk on the current (or an implicit) file.
func (p *diffParser) openHunk(line string, m []string) {
	p.ensureFile()
	p.current.Hunks = append(p.current.Hunks, DiffHunk{
		Header:   line,
		OldStart: atoi(m[1]),
		OldCount: atoiDefault(m[2]),
		NewStart: atoi(m[3]),
		NewCount: atoiDefault(m[4]),
	})
	p.hunk = &p.current.Hunks[len(p.current.Hunks)-1]
	p.wantOld, p.wantNew = p.hunk.OldCount, p.hunk.NewCount
	p.hunkOpen = true
	p.hunkComplete = p.wantOld == 0 && p.wantNew == 0
}

// closeHunk ends the open hunk, failing a mid-input close that leaves rows
// missing: only a truncated input may end on a partial trailing hunk.
func (p *diffParser) closeHunk() error {
	if p.hunkOpen && !p.hunkComplete {
		return fmt.Errorf("%w: hunk %q ends with %d old and %d new rows missing",
			errMalformedDiff, p.hunk.Header, p.wantOld, p.wantNew)
	}
	p.hunk = nil
	p.hunkOpen = false
	return nil
}

// bodyLine consumes one hunk body row, enforcing the header counts. A truly
// empty row reads as context: some producers emit empty (not space-prefixed)
// rows for blank context lines.
func (p *diffParser) bodyLine(line string) error {
	kind := DiffContext
	switch {
	case line == "" || line[0] == ' ':
		p.wantOld, p.wantNew = p.wantOld-1, p.wantNew-1
	case line[0] == '+':
		kind = DiffAdd
		p.wantNew--
	case line[0] == '-':
		kind = DiffDel
		p.wantOld--
	case line[0] == '\\':
		// A "\ No newline at end of file" marker: render it as-is without
		// consuming a counted row.
		p.hunk.Lines = append(p.hunk.Lines, DiffLine{Kind: DiffContext, Text: line})
		return nil
	default:
		return fmt.Errorf("%w: bad hunk row %q", errMalformedDiff, line)
	}
	if p.wantOld < 0 || p.wantNew < 0 {
		return fmt.Errorf("%w: hunk %q exceeds its header counts at %q",
			errMalformedDiff, p.hunk.Header, line)
	}
	p.hunk.Lines = append(p.hunk.Lines, DiffLine{Kind: kind, Text: line})
	p.hunkComplete = p.wantOld == 0 && p.wantNew == 0
	return nil
}

// metaLine consumes a file-level row: ---/+++ paths, known metadata, or an
// unknown row that belongs to a preamble (as in log -p output) and is
// skipped. Unknown rows never error here: input with no diff content at all
// fails in ParseDiff instead, so garbage still errors.
func (p *diffParser) metaLine(line string) {
	switch {
	case strings.HasPrefix(line, "--- "):
		p.ensureFile()
		p.current.OldPath = stripABPrefix(strings.TrimPrefix(line, "--- "))
		p.current.Header = append(p.current.Header, DiffLine{Kind: DiffFileMeta, Text: line})
	case strings.HasPrefix(line, "+++ "):
		p.ensureFile()
		p.current.NewPath = stripABPrefix(strings.TrimPrefix(line, "+++ "))
		p.current.Header = append(p.current.Header, DiffLine{Kind: DiffFileMeta, Text: line})
	case isDiffMeta(line):
		p.ensureFile()
		p.current.Header = append(p.current.Header, DiffLine{Kind: DiffFileMeta, Text: line})
	default:
		// Preamble or trailer row outside any hunk: skipped.
	}
}

// isDiffMeta recognises the file-level metadata rows git emits around hunks.
func isDiffMeta(line string) bool {
	for _, prefix := range []string{
		"index ", "old mode", "new mode", "new file mode", "deleted file mode",
		"similarity index ", "dissimilarity index ", "rename from ", "rename to ",
		"Binary files ",
	} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// splitDiffGit splits the paths after "diff --git ". Paths carry a/ and b/
// prefixes; the split is on the last " b/ " boundary, so only a path
// containing " b/" itself mis-splits.
func splitDiffGit(rest string) (old, new string) {
	if i := strings.LastIndex(rest, ` "b/`); i >= 0 {
		return stripABPrefix(rest[:i]), stripABPrefix(rest[i+1:])
	}
	if i := strings.LastIndex(rest, " b/"); i >= 0 {
		return stripABPrefix(rest[:i]), stripABPrefix(rest[i+1:])
	}
	return stripABPrefix(rest), ""
}

// stripABPrefix drops one a/ or b/ path prefix, leaving /dev/null and bare
// paths untouched. A tab ends the path: non-git unified diffs append a
// timestamp after one.
func stripABPrefix(path string) string {
	if end := gitQuoteEnd(path); end >= 0 {
		if decoded, err := strconv.Unquote(path[:end+1]); err == nil {
			return stripABPrefixDecoded(decoded)
		}
	}
	if i := strings.Index(path, "\t"); i >= 0 {
		path = path[:i]
	}
	return stripABPrefixDecoded(path)
}

func stripABPrefixDecoded(path string) string {
	if strings.HasPrefix(path, "a/") || strings.HasPrefix(path, "b/") {
		return path[2:]
	}
	return path
}

func gitQuoteEnd(path string) int {
	if !strings.HasPrefix(path, `"`) {
		return -1
	}
	escaped := false
	for i := 1; i < len(path); i++ {
		if path[i] == '"' && !escaped {
			return i
		}
		if path[i] == '\\' {
			escaped = !escaped
		} else {
			escaped = false
		}
	}
	return -1
}

// atoi parses a required hunk count, which the header regex guarantees.
func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// atoiDefault parses an optional hunk count: omitted means 1.
func atoiDefault(s string) int {
	if s == "" {
		return 1
	}
	return atoi(s)
}

// Render formats the parsed diff into width-safe rows: every row measures at
// most width cells via lipgloss.Width (never len), with tabs expanded and
// overlong rows truncated with a visible marker that never splits a wide
// glyph. Structural highlighting (file boundaries, hunk headers, +/- rows)
// renders in theme colours where caps allows. A truncated diff gains a footer
// row naming the retained/total line counts and the cap that fired; an empty
// diff renders a labelled row, never nothing.
func (d Diff) Render(width int, c caps.Caps, t theme.Tokens) []string {
	if width < 1 {
		width = 1
	}
	marker := "…"
	if c.Icons == caps.IconsASCII {
		marker = "+"
	}
	if len(d.Lines) == 0 && !d.Truncated {
		return []string{styleRow(DiffContext, fitCells(security.TermSafe(emptyDiffLabel), width, marker), c, t)}
	}
	rows := make([]string, 0, len(d.Lines)+1)
	for _, ln := range d.Lines {
		// Sanitise before measuring or styling so control characters and
		// escape sequences from diff content are inert (§12.7).
		text := fitCells(expandTabs(security.TermSafe(ln.Text)), width, marker)
		rows = append(rows, styleRow(ln.Kind, text, c, t))
	}
	if d.Truncated {
		footer := "showing " + strconv.Itoa(len(d.Lines)) + " of " + strconv.Itoa(d.total) +
			" lines (" + capLabel(d.TruncateCap) + ")"
		rows = append(rows, styleFooter(fitCells(footer, width, marker), c, t))
	}
	return rows
}

// capLabel names the fired cap for the truncation footer.
func capLabel(which string) string {
	switch which {
	case truncateLines:
		return "50,000-line limit"
	case truncateBytes:
		return "4 MiB byte limit"
	default:
		return "truncated"
	}
}

// expandTabs expands tabs to the next multiple of 8 columns, tracking the
// cell column so wide glyphs before a tab keep the alignment truthful.
func expandTabs(s string) string {
	if !strings.ContainsRune(s, '\t') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := tabWidth - col%tabWidth
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col += lipgloss.Width(string(r))
	}
	return b.String()
}

// fitCells truncates s to at most maxCells cells, appending marker when it
// truncates. Prefixes are cut on rune boundaries, so a wide glyph at the
// edge is dropped whole and never split.
func fitCells(s string, maxCells int, marker string) string {
	if lipgloss.Width(s) <= maxCells {
		return s
	}
	budget := maxCells - lipgloss.Width(marker)
	runes := []rune(s)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if lipgloss.Width(string(runes[:mid])) <= budget {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(runes[:lo]) + marker
}

// styleRow applies the kind's theme colour, or none when colour is off.
func styleRow(kind DiffLineKind, text string, c caps.Caps, t theme.Tokens) string {
	if c.Colour == caps.ColourNever {
		return text
	}
	switch kind {
	case DiffFileBoundary:
		return fg(text, t.Attention)
	case DiffFileMeta:
		return fg(text, t.TextMuted)
	case DiffHunkHeader:
		return fg(text, t.Focus)
	case DiffAdd:
		return fg(text, t.OK)
	case DiffDel:
		return fg(text, t.Err)
	default:
		return text
	}
}

// styleFooter colours the truncation footer, or none when colour is off.
func styleFooter(text string, c caps.Caps, t theme.Tokens) string {
	if c.Colour == caps.ColourNever {
		return text
	}
	return fg(text, t.Warning)
}

// fg wraps text in an explicit truecolour sequence from a #rrggbb token.
// Explicit sequences keep Render output independent of stdout detection
// (unlike profile-driven styling, which varies under test); lipgloss stays
// the width authority. An invalid token renders unstyled rather than broken.
func fg(text, hex string) string {
	r, g, b, ok := parseHexColour(hex)
	if !ok {
		return text
	}
	return "\x1b[38;2;" + strconv.Itoa(r) + ";" + strconv.Itoa(g) + ";" + strconv.Itoa(b) + "m" + text + "\x1b[0m"
}

// parseHexColour parses #rrggbb into channels.
func parseHexColour(hex string) (r, g, b int, ok bool) {
	if len(hex) != 7 || hex[0] != '#' {
		return 0, 0, 0, false
	}
	channels := make([]int, 0, 3)
	for i := 1; i < 7; i += 2 {
		n, err := strconv.ParseUint(hex[i:i+2], 16, 8)
		if err != nil {
			return 0, 0, 0, false
		}
		channels = append(channels, int(n))
	}
	return channels[0], channels[1], channels[2], true
}
