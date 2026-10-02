package tui

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/turbokast/mythhelm/internal/tui/caps"
	"github.com/turbokast/mythhelm/internal/tui/theme"
)

// fullCaps exercises structural highlighting and unicode markers.
var fullCaps = caps.Caps{Colour: caps.ColourFull, Motion: caps.MotionFull, Icons: caps.IconsUnicode}

// asciiCaps exercises the ASCII fallback: no non-ASCII bytes, no escapes.
var asciiCaps = caps.Caps{Colour: caps.ColourNever, Motion: caps.MotionOff, Icons: caps.IconsASCII}

func darkTokens(t *testing.T) theme.Tokens {
	t.Helper()
	tokens, err := theme.BuiltIn("dark")
	if err != nil {
		t.Fatalf("BuiltIn(dark): %v", err)
	}
	return tokens
}

// twoFileDiff is a two-file unified diff: cart.go carries one hunk (2 context,
// 1 removed, 1 added), README.md carries two hunks (2 context, 1 added, then
// 1 removed, 1 added). Flattened: 2 boundary + 6 meta + 3 hunk + 4 context +
// 3 added + 2 removed = 20 rows.
const twoFileDiff = `diff --git a/cart.go b/cart.go
index 1111111..2222222 100644
--- a/cart.go
+++ b/cart.go
@@ -1,3 +1,3 @@
 package cart
-const Old = 1
+const New = 2
 var Keep = true
diff --git a/README.md b/README.md
index 3333333..4444444 100644
--- a/README.md
+++ b/README.md
@@ -10,2 +10,3 @@
 line one
+line two
 line three
@@ -20,1 +21,1 @@
-old tail
+new tail
`

func TestParseHunks(t *testing.T) {
	d, err := ParseDiff([]byte(twoFileDiff))
	if err != nil {
		t.Fatalf("ParseDiff: %v", err)
	}
	if d.Truncated {
		t.Fatalf("Truncated = true, want false")
	}
	if len(d.Files) != 2 {
		t.Fatalf("len(Files) = %d, want 2", len(d.Files))
	}
	first := d.Files[0]
	if first.OldPath != "cart.go" || first.NewPath != "cart.go" {
		t.Errorf("Files[0] paths = %q/%q, want cart.go/cart.go", first.OldPath, first.NewPath)
	}
	if len(first.Hunks) != 1 {
		t.Fatalf("len(Files[0].Hunks) = %d, want 1", len(first.Hunks))
	}
	h := first.Hunks[0]
	if h.OldStart != 1 || h.OldCount != 3 || h.NewStart != 1 || h.NewCount != 3 {
		t.Errorf("hunk counts = -%d,%d +%d,%d, want -1,3 +1,3",
			h.OldStart, h.OldCount, h.NewStart, h.NewCount)
	}
	if len(h.Lines) != 4 {
		t.Errorf("len(Hunks[0].Lines) = %d, want 4 body rows", len(h.Lines))
	}
	second := d.Files[1]
	if second.NewPath != "README.md" {
		t.Errorf("Files[1].NewPath = %q, want README.md", second.NewPath)
	}
	if len(second.Hunks) != 2 {
		t.Fatalf("len(Files[1].Hunks) = %d, want 2", len(second.Hunks))
	}
	if len(d.Lines) != 20 {
		t.Fatalf("len(Lines) = %d, want 20 flattened rows", len(d.Lines))
	}
	kinds := map[DiffLineKind]int{}
	for _, ln := range d.Lines {
		kinds[ln.Kind]++
	}
	want := map[DiffLineKind]int{
		DiffFileBoundary: 2,
		DiffFileMeta:     6,
		DiffHunkHeader:   3,
		DiffContext:      4,
		DiffAdd:          3,
		DiffDel:          2,
	}
	for kind, n := range want {
		if kinds[kind] != n {
			t.Errorf("kind %d rows = %d, want %d", kind, kinds[kind], n)
		}
	}
}

func TestParseMalformed(t *testing.T) {
	cases := map[string]string{
		"garbage":      "not a diff at all\n",
		"bad header":   "@@ bogus header\n",
		"bad body row": "@@ -1,1 +1,1 @@\n? bogus\n",
		"short hunk":   "@@ -1,2 +1,2 @@\n only one row\n",
		"over counts":  "@@ -1,1 +1,1 @@\n+one\n+two\n",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDiff([]byte(input)); !errors.Is(err, errMalformedDiff) {
				t.Fatalf("ParseDiff error = %v, want errMalformedDiff", err)
			}
		})
	}
}

func TestParseEmpty(t *testing.T) {
	for _, input := range [][]byte{nil, {}} {
		d, err := ParseDiff(input)
		if err != nil {
			t.Fatalf("ParseDiff(%q): %v", input, err)
		}
		if len(d.Files) != 0 || len(d.Lines) != 0 || d.Truncated {
			t.Fatalf("ParseDiff(%q) = %+v, want empty untruncated diff", input, d)
		}
	}
}

// wideDiff carries a wide-glyph row, a tab row and a combining-mark row.
const wideDiff = `diff --git a/wide.txt b/wide.txt
--- a/wide.txt
+++ b/wide.txt
@@ -1,2 +1,3 @@
 12345678日本語x
-old line here
+` + "\t" + `indented
+éclair
`

func TestRenderWidthSafe(t *testing.T) {
	tokens := darkTokens(t)
	d, err := ParseDiff([]byte(wideDiff))
	if err != nil {
		t.Fatalf("ParseDiff: %v", err)
	}
	for _, width := range []int{1, 2, 10, 12, 20, 80} {
		for _, c := range []caps.Caps{fullCaps, asciiCaps} {
			for i, row := range d.Render(width, c, tokens) {
				if w := lipgloss.Width(row); w > width {
					t.Fatalf("width %d caps %+v row %d measures %d cells: %q",
						width, c, i, w, row)
				}
			}
		}
	}
	// A wide glyph at the boundary is dropped whole with the marker, never
	// split: " 12345678" is 9 cells, so 日 (2 cells) plus the marker cannot
	// fit in 10 and the row ends after 8.
	// Rows: 0 diff --git, 1 ---, 2 +++, 3 hunk header, 4 context, 5 removed,
	// 6 tabbed addition, 7 combining-mark addition.
	unicode := d.Render(10, fullCaps, tokens)
	if unicode[4] != " 12345678…" {
		t.Errorf("unicode row 4 = %q, want %q", unicode[4], " 12345678…")
	}
	if strings.Contains(unicode[4], "日") {
		t.Errorf("unicode row 4 splits a wide glyph: %q", unicode[4])
	}
	ascii := d.Render(10, asciiCaps, tokens)
	if ascii[4] != " 12345678+" {
		t.Errorf("ascii row 4 = %q, want %q", ascii[4], " 12345678+")
	}
	// Where the glyph fits it survives intact: at width 12 the row keeps 日.
	wide := d.Render(12, fullCaps, tokens)
	if wide[4] != " 12345678日…" {
		t.Errorf("width-12 row 4 = %q, want %q", wide[4], " 12345678日…")
	}
	// Tabs expand to the next multiple of 8: the tab at column 1 becomes 7 spaces.
	plain := d.Render(80, asciiCaps, tokens)
	if plain[6] != "+       indented" {
		t.Errorf("tab row = %q, want %q", plain[6], "+       indented")
	}
}

func TestRenderAsciiFallback(t *testing.T) {
	tokens := darkTokens(t)
	d, err := ParseDiff([]byte(twoFileDiff))
	if err != nil {
		t.Fatalf("ParseDiff: %v", err)
	}
	for _, row := range d.Render(80, asciiCaps, tokens) {
		if strings.Contains(row, "\x1b") {
			t.Fatalf("ascii row holds an escape introduction: %q", row)
		}
		for i := 0; i < len(row); i++ {
			if row[i] >= 0x80 {
				t.Fatalf("ascii row holds non-ASCII byte %#02x: %q", row[i], row)
			}
		}
	}
	// The narrow ASCII render truncates with "+" and stays pure ASCII.
	narrow := d.Render(20, asciiCaps, tokens)
	found := false
	for _, row := range narrow {
		if strings.HasSuffix(row, "+") {
			found = true
		}
		for i := 0; i < len(row); i++ {
			if row[i] >= 0x80 {
				t.Fatalf("narrow ascii row holds non-ASCII byte %#02x: %q", row[i], row)
			}
		}
	}
	if !found {
		t.Fatalf("no narrow ascii row carries the + truncation marker: %q", narrow)
	}
	// The unicode/full-caps variant carries structural highlighting.
	highlighted := strings.Join(d.Render(80, fullCaps, tokens), "\n")
	if !strings.Contains(highlighted, "\x1b[") {
		t.Fatalf("full-caps render holds no structural highlighting: %q", highlighted)
	}
}

// lineTruncationDiff builds a valid diff of 3 header rows plus pairs of hunk
// header + one context row, so every line counts toward the cap.
func lineTruncationDiff(pairs int) (input string, total int) {
	var b strings.Builder
	b.WriteString("diff --git a/big.txt b/big.txt\n--- a/big.txt\n+++ b/big.txt\n")
	for i := 1; i <= pairs; i++ {
		b.WriteString("@@ -" + strconv.Itoa(i) + ",1 +" + strconv.Itoa(i) + ",1 @@\n")
		b.WriteString(" line " + strconv.Itoa(i) + "\n")
	}
	return b.String(), 3 + 2*pairs
}

func TestTruncationLabelled(t *testing.T) {
	tokens := darkTokens(t)
	input, total := lineTruncationDiff(30000)
	if total != 60003 {
		t.Fatalf("fixture totals %d lines, want 60003", total)
	}
	d, err := ParseDiff([]byte(input))
	if err != nil {
		t.Fatalf("ParseDiff: %v", err)
	}
	if !d.Truncated || d.TruncateCap != "lines" {
		t.Fatalf("Truncated/Cap = %v/%q, want true/lines", d.Truncated, d.TruncateCap)
	}
	// The retained 50,000 rows end on a bare trailing hunk header, which a
	// truncated input keeps rather than rejecting as a short hunk.
	if len(d.Lines) != maxDiffLines {
		t.Fatalf("len(Lines) = %d, want %d retained rows", len(d.Lines), maxDiffLines)
	}
	if d.TotalLines() != total {
		t.Fatalf("TotalLines() = %d, want the %d pre-cap input lines", d.TotalLines(), total)
	}
	rows := d.Render(120, asciiCaps, tokens)
	footer := rows[len(rows)-1]
	if !strings.Contains(footer, "50000 of 60003") {
		t.Errorf("footer %q lacks the retained/total line counts", footer)
	}
	if !strings.Contains(footer, "50,000-line limit") {
		t.Errorf("footer %q does not name the line cap", footer)
	}
	for i, row := range rows {
		if w := lipgloss.Width(row); w > 120 {
			t.Fatalf("row %d measures %d cells, over the 120 width", i, w)
		}
	}
}

func TestByteCapBoundsSingleLine(t *testing.T) {
	tokens := darkTokens(t)
	input := strings.Repeat("x", maxDiffBytes+100) // one line, no newline
	d, err := ParseDiff([]byte(input))
	if err != nil {
		t.Fatalf("ParseDiff: %v", err)
	}
	if !d.Truncated || d.TruncateCap != "bytes" {
		t.Fatalf("Truncated/Cap = %v/%q, want true/bytes", d.Truncated, d.TruncateCap)
	}
	if len(d.Lines) != 0 {
		t.Fatalf("len(Lines) = %d, want zero parsed rows", len(d.Lines))
	}
	rows := d.Render(120, asciiCaps, tokens)
	if len(rows) != 1 {
		t.Fatalf("len(Render) = %d, want the lone footer row", len(rows))
	}
	if !strings.Contains(rows[0], "0 of 1") {
		t.Errorf("footer %q lacks the 0 retained over 1 total counts", rows[0])
	}
	if !strings.Contains(rows[0], "4 MiB byte limit") {
		t.Errorf("footer %q does not name the byte cap", rows[0])
	}
	if strings.Contains(rows[0], "50000") {
		t.Errorf("footer %q claims retained rows it does not hold", rows[0])
	}
}

func TestByteCapFiresFirst(t *testing.T) {
	tokens := darkTokens(t)
	// Past both caps (60,003 lines near 6 MiB): the byte cap must fire first,
	// so the footer never claims 50,000 shown lines.
	input, _ := lineTruncationDiff(30000)
	padded := strings.ReplaceAll(input, "\n line ", "\n line "+strings.Repeat(".", 200))
	d, err := ParseDiff([]byte(padded))
	if err != nil {
		t.Fatalf("ParseDiff: %v", err)
	}
	if !d.Truncated || d.TruncateCap != "bytes" {
		t.Fatalf("Truncated/Cap = %v/%q, want true/bytes", d.Truncated, d.TruncateCap)
	}
	if len(d.Lines) >= maxDiffLines {
		t.Fatalf("len(Lines) = %d, want fewer than %d", len(d.Lines), maxDiffLines)
	}
	rows := d.Render(120, asciiCaps, tokens)
	if footer := rows[len(rows)-1]; !strings.Contains(footer, "4 MiB byte limit") {
		t.Errorf("footer %q does not name the byte cap", footer)
	}
}

// markdownDiff is a diff over a .md file: it must render diff chrome, never
// Markdown-prose rendering.
const markdownDiff = `diff --git a/guide.md b/guide.md
index aaaaaaa..bbbbbbb 100644
--- a/guide.md
+++ b/guide.md
@@ -1,2 +1,3 @@
 # Title
+## Added heading
 existing prose
`

func TestMarkdownDiffKeepsDiffChrome(t *testing.T) {
	tokens := darkTokens(t)
	d, err := ParseDiff([]byte(markdownDiff))
	if err != nil {
		t.Fatalf("ParseDiff: %v", err)
	}
	if len(d.Files) != 1 || d.Files[0].NewPath != "guide.md" {
		t.Fatalf("Files = %+v, want one file at guide.md", d.Files)
	}
	// Colour off so chrome rows carry no escape introductions to strip.
	rows := d.Render(80, caps.Caps{Colour: caps.ColourNever, Motion: caps.MotionFull, Icons: caps.IconsUnicode}, tokens)
	var hunk, added, context bool
	for _, row := range rows {
		switch {
		case strings.Contains(row, "@@ -1,2 +1,3 @@"):
			hunk = true
		case strings.HasPrefix(row, "+"):
			added = true
		case strings.HasPrefix(row, " "):
			context = true
		}
	}
	if !hunk || !added || !context {
		t.Fatalf("render lost diff chrome (hunk=%v added=%v context=%v): %q",
			hunk, added, context, rows)
	}
}

func TestRenderSanitisesControls(t *testing.T) {
	tokens := darkTokens(t)
	input := "diff --git a/evil.txt b/evil.txt\n--- a/evil.txt\n+++ b/evil.txt\n" +
		"@@ -1,1 +1,2 @@\n-a\x00b\x07c\n+\x1b[31mred\x1b[0m\n+plain\n"
	d, err := ParseDiff([]byte(input))
	if err != nil {
		t.Fatalf("ParseDiff: %v", err)
	}
	joined := strings.Join(d.Render(80, asciiCaps, tokens), "\n")
	if strings.Contains(joined, "\x1b") {
		t.Fatalf("render leaks an escape introduction: %q", joined)
	}
	if !strings.Contains(joined, "red") {
		t.Errorf("render lost the escape-wrapped text: %q", joined)
	}
	if !strings.Contains(joined, "abc") {
		t.Errorf("render did not strip control characters: %q", joined)
	}
}

func TestParseKeepsNoNewlineMarkers(t *testing.T) {
	input := "diff --git a/x b/x\n--- a/x\n+++ b/x\n" +
		"@@ -1,1 +1,1 @@\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file\n"
	d, err := ParseDiff([]byte(input))
	if err != nil {
		t.Fatalf("ParseDiff: %v", err)
	}
	// Rows: boundary, ---, +++, hunk header, removed, old-side marker,
	// added, new-side marker. The trailing marker arrives after the hunk
	// counts complete; it must be kept, not skipped as preamble.
	if len(d.Lines) != 8 {
		t.Fatalf("len(Lines) = %d, want 8 rows with both markers", len(d.Lines))
	}
	for _, i := range []int{5, 7} {
		if d.Lines[i].Kind != DiffContext || d.Lines[i].Text != "\\ No newline at end of file" {
			t.Errorf("row %d = %+v, want the context no-newline marker", i, d.Lines[i])
		}
	}
}

func TestParseDecodesQuotedPaths(t *testing.T) {
	// Git quotes non-ASCII pathnames (core.quotePath) with C-style escapes.
	input := "diff --git \"a/f\\303\\264o\" \"b/f\\303\\264o\"\n" +
		"--- \"a/f\\303\\264o\"\n+++ \"b/f\\303\\264o\"\n" +
		"@@ -1,1 +1,1 @@\n-old\n+new\n"
	d, err := ParseDiff([]byte(input))
	if err != nil {
		t.Fatalf("ParseDiff: %v", err)
	}
	if len(d.Files) != 1 {
		t.Fatalf("len(Files) = %d, want 1", len(d.Files))
	}
	if d.Files[0].OldPath != "f\xc3\xb4o" || d.Files[0].NewPath != "f\xc3\xb4o" {
		t.Fatalf("paths = %+v, want decoded f\\303\\264o on both sides", d.Files[0])
	}
}

func TestRenderEmptyLabelsAbsence(t *testing.T) {
	tokens := darkTokens(t)
	d, err := ParseDiff(nil)
	if err != nil {
		t.Fatalf("ParseDiff: %v", err)
	}
	rows := d.Render(80, asciiCaps, tokens)
	if len(rows) != 1 || rows[0] != emptyDiffLabel {
		t.Fatalf("Render = %q, want the lone %q label", rows, emptyDiffLabel)
	}
	if zero := (Diff{}).Render(80, asciiCaps, tokens); len(zero) != 1 || zero[0] != emptyDiffLabel {
		t.Fatalf("zero Diff Render = %q, want the lone %q label", zero, emptyDiffLabel)
	}
}
