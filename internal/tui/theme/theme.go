// Package theme loads and validates the declarative TUI theme tokens.
package theme

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"

	"github.com/BurntSushi/toml"
	"github.com/turbokast/mythhelm/mods/themes"
)

var (
	// ErrUnknownTheme marks a built-in theme name that does not exist.
	ErrUnknownTheme = errors.New("unknown theme")
	// ErrInvalidTheme marks a theme file or token set that fails validation.
	ErrInvalidTheme = errors.New("invalid theme")
)

// minContrast is the WCAG AA text contrast ratio (§15.2: text checked for
// contrast in supported colour modes).
const minContrast = 4.5

// Tokens is the declarative theme format: hex colours plus a border style.
// It carries no code paths (I11) and no font requirement (I13).
type Tokens struct {
	Surface       string `toml:"surface"`
	SurfaceRaised string `toml:"surface_raised"`
	Text          string `toml:"text"`
	TextMuted     string `toml:"text_muted"`
	Focus         string `toml:"focus"`
	Attention     string `toml:"attention"`
	OK            string `toml:"ok"`
	Warning       string `toml:"warning"`
	Err           string `toml:"err"`
	Border        string `toml:"border"`
}

// BuiltIn parses the named built-in theme from the embedded token files,
// through the same parse path as Load so both share one validation.
func BuiltIn(name string) (Tokens, error) {
	switch name {
	case "dark", "light":
		raw, err := themes.Files.ReadFile(name + ".toml")
		if err != nil {
			return Tokens{}, fmt.Errorf("%w: built-in %q unreadable: %w", ErrInvalidTheme, name, err)
		}
		return parse(raw)
	default:
		return Tokens{}, fmt.Errorf("%w: %q", ErrUnknownTheme, name)
	}
}

// maxThemeBytes bounds theme files: tokens are ten short lines, so anything
// larger is not a theme.
const maxThemeBytes = 64 << 10

// Load parses a theme TOML file, rejecting unknown keys and bad values.
func Load(path string) (Tokens, error) {
	f, err := os.Open(path) //nolint:gosec // Load opens the caller-chosen theme file by design.
	if err != nil {
		return Tokens{}, fmt.Errorf("%w: open: %w", ErrInvalidTheme, err)
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, maxThemeBytes+1))
	if err != nil {
		return Tokens{}, fmt.Errorf("%w: read: %w", ErrInvalidTheme, err)
	}
	if len(raw) > maxThemeBytes {
		return Tokens{}, fmt.Errorf("%w: larger than %d bytes", ErrInvalidTheme, maxThemeBytes)
	}
	return parse(raw)
}

// parse decodes TOML bytes, rejects unknown keys and validates the tokens.
func parse(raw []byte) (Tokens, error) {
	var t Tokens
	meta, err := toml.Decode(string(raw), &t)
	if err != nil {
		return Tokens{}, fmt.Errorf("%w: %w", ErrInvalidTheme, err)
	}
	if keys := meta.Undecoded(); len(keys) > 0 {
		return Tokens{}, fmt.Errorf("%w: unknown key %s", ErrInvalidTheme, keys[0].String())
	}
	if err := t.Validate(); err != nil {
		return Tokens{}, err
	}
	return t, nil
}

// Validate checks hex shape, border style and text/surface contrast.
// Every failure names the offending key.
func (t Tokens) Validate() error {
	colours := []struct {
		key   string
		value string
	}{
		{"surface", t.Surface},
		{"surface_raised", t.SurfaceRaised},
		{"text", t.Text},
		{"text_muted", t.TextMuted},
		{"focus", t.Focus},
		{"attention", t.Attention},
		{"ok", t.OK},
		{"warning", t.Warning},
		{"err", t.Err},
	}
	for _, c := range colours {
		if !isHexColour(c.value) {
			return fmt.Errorf("%w: %s must be #rrggbb, got %q", ErrInvalidTheme, c.key, c.value)
		}
	}
	if t.Border != "rounded" && t.Border != "ascii" {
		return fmt.Errorf("%w: border must be rounded or ascii, got %q", ErrInvalidTheme, t.Border)
	}
	pairs := []struct {
		fgKey, bgKey string
		fg, bg       string
	}{
		{"text", "surface", t.Text, t.Surface},
		{"text", "surface_raised", t.Text, t.SurfaceRaised},
		{"text_muted", "surface", t.TextMuted, t.Surface},
		{"text_muted", "surface_raised", t.TextMuted, t.SurfaceRaised},
	}
	for _, p := range pairs {
		if r := contrastRatio(p.fg, p.bg); r < minContrast {
			return fmt.Errorf("%w: %s on %s contrast %.2f below %.1f",
				ErrInvalidTheme, p.fgKey, p.bgKey, r, minContrast)
		}
	}
	return nil
}

// isHexColour reports whether s is a #rrggbb colour.
func isHexColour(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		digit := c >= '0' && c <= '9'
		lower := c >= 'a' && c <= 'f'
		upper := c >= 'A' && c <= 'F'
		if !digit && !lower && !upper {
			return false
		}
	}
	return true
}

// contrastRatio is the WCAG contrast ratio of two validated hex colours.
func contrastRatio(fg, bg string) float64 {
	l1, l2 := relativeLuminance(fg), relativeLuminance(bg)
	if l2 > l1 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

// relativeLuminance is the WCAG relative luminance of a validated #rrggbb
// colour. Inputs come from Validate, so the parse cannot fail.
func relativeLuminance(hex string) float64 {
	r, _ := strconv.ParseUint(hex[1:3], 16, 8)
	g, _ := strconv.ParseUint(hex[3:5], 16, 8)
	b, _ := strconv.ParseUint(hex[5:7], 16, 8)
	return 0.2126*channel(r) + 0.7152*channel(g) + 0.0722*channel(b)
}

// channel linearises one sRGB channel value.
func channel(v uint64) float64 {
	c := float64(v) / 255
	if c <= 0.03928 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}
