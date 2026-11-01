package theme

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The shared table belongs to the site; its names map to TUI roles here without
// rewriting it or treating the TUI-only rows as site CSS tokens.
var sharedPaletteRoles = map[string]string{
	"Night": "dark.surface", "Frost": "dark.text", "Ink": "light.text",
	"Star (dark)": "dark.focus", "Star (light)": "light.focus", "Mist": "dark.text_muted",
	"Paper": "light.surface", "Amber (dark)": "dark.attention", "Amber (light)": "light.attention",
}

func TestTUIPaletteDocsMatch(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "assets", "brand", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	palettes := make(map[string]Tokens)
	for _, name := range []string{"dark", "light"} {
		palettes[name], err = BuiltIn(name)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := paletteDocsMatch(string(raw), palettes); err != nil {
		t.Fatal(err)
	}
}

func paletteDocsMatch(raw string, palettes map[string]Tokens) error {
	values, err := parsePaletteDocs(raw)
	if err != nil {
		return err
	}
	for _, name := range []string{"dark", "light"} {
		v, ok := palettes[name]
		if !ok {
			return fmt.Errorf("missing embedded theme %s", name)
		}
		for key, value := range tokenFields(&v) {
			pair := name + "." + key
			got, ok := values[pair]
			if !ok {
				return fmt.Errorf("missing mapping %s", pair)
			}
			if got != *value {
				return fmt.Errorf("%s docs value %s differs from embedded %s", pair, got, *value)
			}
		}
	}
	return nil
}

func parsePaletteDocs(raw string) (map[string]string, error) {
	values := make(map[string]string)
	sharedSeen := make(map[string]bool)
	tuiRoles := map[string]bool{"dark.surface_raised": true, "dark.border": true, "dark.ok": true, "dark.err": true, "light.surface_raised": true, "light.border": true, "light.ok": true, "light.err": true, "light.text_muted": true}
	sections, headers := make(map[string]int), make(map[string]int)
	section := ""
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if name, ok := strings.CutPrefix(line, "## "); ok {
			section = name
			if section == "Colours" || section == "TUI palette" {
				sections[section]++
				if sections[section] > 1 {
					return nil, fmt.Errorf("duplicate section %s", section)
				}
			}
			continue
		}
		if section != "Colours" && section != "TUI palette" || !strings.HasPrefix(line, "|") {
			continue
		}
		if !strings.HasSuffix(line, "|") {
			return nil, fmt.Errorf("malformed row in %s", section)
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) != 3 {
			return nil, fmt.Errorf("malformed row in %s", section)
		}
		for i := range cells {
			cells[i] = strings.Trim(strings.TrimSpace(cells[i]), "`")
		}
		if cells[0] == "---" && cells[1] == "---" && cells[2] == "---" {
			continue
		}
		if section == "Colours" && cells[0] == "Token" && cells[1] == "Hex" && cells[2] == "Use" || section == "TUI palette" && cells[0] == "Theme ID" && cells[1] == "Token" && cells[2] == "Hex" {
			headers[section]++
			if headers[section] != 1 {
				return nil, fmt.Errorf("duplicate header in %s", section)
			}
			continue
		}
		if headers[section] != 1 {
			return nil, fmt.Errorf("missing header in %s", section)
		}
		var pair, value string
		if section == "Colours" {
			pair, value = sharedPaletteRoles[cells[0]], cells[1]
			sharedSeen[pair] = true
			if pair == "" {
				return nil, fmt.Errorf("unknown shared token %s", cells[0])
			}
		} else {
			if cells[0] != "dark" && cells[0] != "light" {
				return nil, fmt.Errorf("unknown theme %s", cells[0])
			}
			var v Tokens
			if _, ok := tokenFields(&v)[cells[1]]; !ok {
				return nil, fmt.Errorf("unknown token %s", cells[1])
			}
			pair, value = cells[0]+"."+cells[1], cells[2]
			if !tuiRoles[pair] {
				return nil, fmt.Errorf("unexpected TUI mapping %s", pair)
			}
		}
		if !isHexColour(value) {
			return nil, fmt.Errorf("malformed colour for %s", pair)
		}
		if _, exists := values[pair]; exists {
			return nil, fmt.Errorf("duplicate mapping %s", pair)
		}
		values[pair] = value
	}
	for _, section := range []string{"Colours", "TUI palette"} {
		if sections[section] != 1 || headers[section] != 1 {
			return nil, fmt.Errorf("missing table %s", section)
		}
	}
	// Exact source ownership matters as well as combined coverage: neither table
	// may silently replace rows owned by the other.
	for _, pair := range sharedPaletteRoles {
		if !sharedSeen[pair] {
			return nil, fmt.Errorf("missing mapping %s in shared table", pair)
		}
	}
	return values, nil
}

const paletteDocsFixture = `## Colours
| Token | Hex | Use |
|---|---|---|
| Night | ` + "`#090f1a`" + ` | Background |
| Frost | ` + "`#ebeff5`" + ` | Text |
| Ink | ` + "`#101621`" + ` | Text |
| Star (dark) | ` + "`#4ddae9`" + ` | Focus |
| Star (light) | ` + "`#00829d`" + ` | Focus |
| Mist | ` + "`#989fab`" + ` | Muted |
| Paper | ` + "`#f6f4ee`" + ` | Background |
| Amber (dark) | ` + "`#f0c27b`" + ` | Attention |
| Amber (light) | ` + "`#8a5a00`" + ` | Attention |
## TUI palette
| Theme ID | Token | Hex |
|---|---|---|
| dark | surface_raised | #0f1a2b |
| dark | border | #4b5668 |
| dark | ok | #7fd8a9 |
| dark | err | #f4918a |
| light | surface_raised | #ebe8df |
| light | border | #a3a7ae |
| light | ok | #23794a |
| light | err | #b4232a |
| light | text_muted | #59616f |
`

func TestTUIPaletteDocsControls(t *testing.T) {
	t.Parallel()
	if err := paletteDocsMatch(paletteDocsFixture, observatoryPalettes()); err != nil {
		t.Fatalf("valid control: %v", err)
	}
	row := "| dark | border | #4b5668 |\n"
	shared := "| Night | `#090f1a` | Background |\n"
	for _, tt := range []struct{ name, raw, want string }{
		{"empty", "", "missing table"},
		{"empty TUI", strings.Split(paletteDocsFixture, "## TUI palette")[0] + "## TUI palette\n| Theme ID | Token | Hex |\n|---|---|---|\n", "missing mapping"},
		{"empty shared", "## Colours\n| Token | Hex | Use |\n|---|---|---|\n## TUI palette" + strings.Split(paletteDocsFixture, "## TUI palette")[1], "missing mapping"},
		{"missing TUI row", strings.Replace(paletteDocsFixture, row, "", 1), "missing mapping dark.border"},
		{"missing shared row", strings.Replace(paletteDocsFixture, shared, "", 1), "missing mapping dark.surface"},
		{"duplicate identical", paletteDocsFixture + row, "duplicate mapping dark.border"},
		{"duplicate conflicting", paletteDocsFixture + strings.ReplaceAll(row, "#4b5668", "#ffffff"), "duplicate mapping dark.border"},
		{"duplicate shared", strings.Replace(paletteDocsFixture, shared, shared+shared, 1), "duplicate mapping dark.surface"},
		{"shared collision", paletteDocsFixture + "| dark | surface | #090f1a |\n", "unexpected TUI mapping dark.surface"},
		{"unknown theme", strings.Replace(paletteDocsFixture, row, strings.ReplaceAll(row, "dark", "other"), 1), "unknown theme other"},
		{"unknown token", strings.Replace(paletteDocsFixture, row, strings.ReplaceAll(row, "border", "warning"), 1), "unknown token warning"},
		{"unknown shared", strings.Replace(paletteDocsFixture, "Night", "Other", 1), "unknown shared token Other"},
		{"altered cell", strings.Replace(paletteDocsFixture, "#4b5668", "#4b5669", 1), "dark.border docs value"},
		{"malformed colour", strings.Replace(paletteDocsFixture, "#4b5668", "blue", 1), "malformed colour for dark.border"},
		{"malformed row", strings.Replace(paletteDocsFixture, row, "| dark | border |\n", 1), "malformed row"},
		{"missing header", strings.Replace(paletteDocsFixture, "| Theme ID | Token | Hex |\n", "", 1), "missing header"},
		{"duplicate section", paletteDocsFixture + "## TUI palette\n", "duplicate section"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := paletteDocsMatch(tt.raw, observatoryPalettes())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
