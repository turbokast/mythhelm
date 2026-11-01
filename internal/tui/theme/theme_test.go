package theme

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnknownTheme(t *testing.T) {
	t.Parallel()
	_, err := BuiltIn("aurora")
	if !errors.Is(err, ErrUnknownTheme) {
		t.Fatalf("BuiltIn(\"aurora\") error = %v, want ErrUnknownTheme", err)
	}
}

func TestBuiltInsValidate(t *testing.T) {
	t.Parallel()
	goldens := observatoryPalettes()
	for name, want := range goldens {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := BuiltIn(name)
			if err != nil {
				t.Fatalf("BuiltIn(%q) error = %v, want nil", name, err)
			}
			if got != want {
				t.Fatalf("BuiltIn(%q) = %+v, want %+v", name, got, want)
			}
			if err := got.Validate(); err != nil {
				t.Fatalf("BuiltIn(%q).Validate() error = %v, want nil", name, err)
			}
		})
	}
	lowContrast := []struct {
		name    string
		change  func(*Tokens)
		wantKey string
		wantBG  string
	}{
		{name: "low contrast text", change: func(t *Tokens) { t.Text = "#23242f" }, wantKey: "text", wantBG: "surface"},
		{name: "low contrast muted", change: func(t *Tokens) { t.TextMuted = "#2b2e40" }, wantKey: "text_muted", wantBG: "surface"},
		{name: "low contrast text on raised surface", change: func(t *Tokens) { t.Text = "#7e7e7e" }, wantKey: "text", wantBG: "surface_raised"},
		{name: "low contrast muted text on raised surface", change: func(t *Tokens) { t.TextMuted = "#7e7e7e" }, wantKey: "text_muted", wantBG: "surface_raised"},
	}
	for _, tt := range lowContrast {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bad := goldens["dark"]
			tt.change(&bad)
			err := bad.Validate()
			if !errors.Is(err, ErrInvalidTheme) {
				t.Fatalf("Validate() error = %v, want ErrInvalidTheme", err)
			}
			if !strings.Contains(err.Error(), tt.wantKey+" on "+tt.wantBG+" contrast") {
				t.Fatalf("Validate() error = %v, want contrast for %q on %q", err, tt.wantKey, tt.wantBG)
			}
		})
	}
}

func TestValidateRejectsBadTokens(t *testing.T) {
	t.Parallel()
	good, err := BuiltIn("dark")
	if err != nil {
		t.Fatalf("BuiltIn(\"dark\") error = %v, want nil", err)
	}
	mutate := func(change func(*Tokens)) Tokens {
		t.Helper()
		bad := good
		change(&bad)
		return bad
	}
	tests := []struct {
		name    string
		tokens  Tokens
		wantKey string
	}{
		{name: "non-hex colour", tokens: mutate(func(t *Tokens) { t.Focus = "blue" }), wantKey: "focus"},
		{name: "short hex colour", tokens: mutate(func(t *Tokens) { t.OK = "#9e6" }), wantKey: "ok"},
		{name: "missing colour", tokens: mutate(func(t *Tokens) { t.Border = "" }), wantKey: "border"},
		{name: "bad border", tokens: mutate(func(t *Tokens) { t.Border = "double" }), wantKey: "border"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.tokens.Validate()
			if !errors.Is(err, ErrInvalidTheme) {
				t.Fatalf("Validate() error = %v, want ErrInvalidTheme", err)
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantKey) {
				t.Fatalf("Validate() error = %v, want it to name %q", err, tt.wantKey)
			}
		})
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exact := writeThemeFile(t, dir, "exact.toml", validThemeBody)
	got, err := Load(exact)
	if err != nil {
		t.Fatalf("Load(exact schema) error = %v, want nil", err)
	}
	want, err := BuiltIn("dark")
	if err != nil {
		t.Fatalf("BuiltIn(\"dark\") error = %v, want nil", err)
	}
	if got != want {
		t.Fatalf("Load(exact schema) = %+v, want %+v", got, want)
	}

	//nolint:misspell // intentional unknown-key fixture: the acceptance pins this typo.
	typo := writeThemeFile(t, dir, "typo.toml", validThemeBody+"backgroud = \"#000000\"\n")
	_, err = Load(typo)
	if !errors.Is(err, ErrInvalidTheme) {
		t.Fatalf("Load(typo key) error = %v, want ErrInvalidTheme", err)
	}
	if !strings.Contains(err.Error(), "backgroud") { //nolint:misspell // matches the fixture above.
		t.Fatalf("Load(typo key) error = %v, want it to name the key", err)
	}
}

func TestLoadRejectsBadColours(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		body    string
		wantKey string
	}{
		{name: "named colour", body: strings.Replace(validThemeBody, `focus = "#4ddae9"`, `focus = "blue"`, 1), wantKey: "focus"},
		{name: "short hex", body: strings.Replace(validThemeBody, `ok = "#7fd8a9"`, `ok = "#9e6"`, 1), wantKey: "ok"},
		{name: "missing key", body: strings.Replace(validThemeBody, "border = \"#4b5668\"\n", "", 1), wantKey: "border"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := writeThemeFile(t, t.TempDir(), "bad.toml", tt.body)
			_, err := Load(path)
			if !errors.Is(err, ErrInvalidTheme) {
				t.Fatalf("Load() error = %v, want ErrInvalidTheme", err)
			}
			if !strings.Contains(err.Error(), tt.wantKey) {
				t.Fatalf("Load() error = %v, want it to name %q", err, tt.wantKey)
			}
		})
	}
}

// validThemeBody is the exact-schema TOML matching the dark golden.
const validThemeBody = `surface = "#090f1a"
surface_raised = "#0f1a2b"
text = "#ebeff5"
text_muted = "#989fab"
focus = "#4ddae9"
attention = "#f0c27b"
ok = "#7fd8a9"
err = "#f4918a"
border = "#4b5668"
`

func writeThemeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestContrastRatio(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		fg    string
		bg    string
		want  float64
		delta float64
	}{
		{name: "black on white is 21", fg: "#000000", bg: "#ffffff", want: 21, delta: 0.01},
		{name: "identical colours are 1", fg: "#1a1b26", bg: "#1a1b26", want: 1, delta: 0.01},
		{name: "symmetric", fg: "#ffffff", bg: "#000000", want: 21, delta: 0.01},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := contrastRatio(tt.fg, tt.bg)
			if math.Abs(got-tt.want) > tt.delta {
				t.Fatalf("contrastRatio(%q, %q) = %v, want %v", tt.fg, tt.bg, got, tt.want)
			}
		})
	}
}

func observatoryPalettes() map[string]Tokens {
	return map[string]Tokens{
		"dark":  {Surface: "#090f1a", SurfaceRaised: "#0f1a2b", Text: "#ebeff5", TextMuted: "#989fab", Focus: "#4ddae9", Attention: "#f0c27b", OK: "#7fd8a9", Err: "#f4918a", Border: "#4b5668"},
		"light": {Surface: "#f6f4ee", SurfaceRaised: "#ebe8df", Text: "#101621", TextMuted: "#59616f", Focus: "#00829d", Attention: "#8a5a00", OK: "#23794a", Err: "#b4232a", Border: "#a3a7ae"},
	}
}

func tokenFields(v *Tokens) map[string]*string {
	return map[string]*string{"surface": &v.Surface, "surface_raised": &v.SurfaceRaised, "text": &v.Text, "text_muted": &v.TextMuted, "focus": &v.Focus, "attention": &v.Attention, "ok": &v.OK, "err": &v.Err, "border": &v.Border}
}

func TestObservatoryPaperTokens(t *testing.T) {
	t.Parallel()
	for name, want := range observatoryPalettes() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := BuiltIn(name)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("BuiltIn(%q) = %+v, want %+v", name, got, want)
			}
		})
	}
}

func TestThemeContrastMatrix(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"text", "text_muted", "attention", "err", "ok", "focus"} {
		for _, bg := range []string{"surface", "surface_raised"} {
			if key == "ok" && bg == "surface_raised" {
				continue
			}
			t.Run(key+" on "+bg, func(t *testing.T) {
				t.Parallel()
				v := Tokens{Surface: "#090f1a", SurfaceRaised: "#090f1a", Text: "#ffffff", TextMuted: "#ffffff", Focus: "#ffffff", Attention: "#ffffff", OK: "#ffffff", Err: "#ffffff", Border: "#4b5668"}
				fields := tokenFields(&v)
				*fields[bg] = "#303030"
				minimum, fg := 4.5, "#7e7e7e"
				if key == "focus" {
					minimum, fg = 3, "#666666"
				}
				*fields[key] = fg
				other := "surface"
				if bg == other {
					other = "surface_raised"
				}
				if contrastRatio(fg, *fields[bg]) >= minimum || contrastRatio(fg, *fields[other]) < minimum {
					t.Fatal("fixture does not isolate the named pair")
				}
				assertContrastError(t, v.Validate(), key, bg, contrastRatio(fg, *fields[bg]), minimum)
			})
		}
	}
	for _, tt := range []struct {
		key, colour    string
		ratio, minimum float64
	}{
		{"attention", "#918c91", 3, 4.5}, {"focus", "#aab0b0", 2, 3},
	} {
		t.Run("Paper "+tt.key, func(t *testing.T) {
			t.Parallel()
			v := observatoryPalettes()["light"]
			*tokenFields(&v)[tt.key] = tt.colour
			ratio := contrastRatio(tt.colour, v.Surface)
			if math.Abs(ratio-tt.ratio) > .001 {
				t.Fatalf("fixture ratio = %v, want approximately %v", ratio, tt.ratio)
			}
			assertContrastError(t, v.Validate(), tt.key, "surface", ratio, tt.minimum)
		})
	}
}

func assertContrastError(t *testing.T, err error, key, bg string, ratio, minimum float64) {
	t.Helper()
	want := fmt.Sprintf("%s on %s contrast %.2f below %.1f", key, bg, ratio, minimum)
	if !errors.Is(err, ErrInvalidTheme) || !strings.Contains(err.Error(), want) {
		t.Fatalf("Validate() = %v, want ErrInvalidTheme and %q", err, want)
	}
}

func TestThemeBorderAndKeys(t *testing.T) {
	t.Parallel()
	if got, err := Load(writeThemeFile(t, t.TempDir(), "valid.toml", validThemeBody)); err != nil || got != observatoryPalettes()["dark"] {
		t.Fatalf("valid nine-key control = %+v, %v", got, err)
	}
	var v Tokens
	for key := range tokenFields(&v) {
		t.Run("missing "+key, func(t *testing.T) {
			t.Parallel()
			var lines []string
			for line := range strings.SplitSeq(validThemeBody, "\n") {
				if !strings.HasPrefix(line, key+" = ") {
					lines = append(lines, line)
				}
			}
			assertThemeLoadError(t, strings.Join(lines, "\n"), key)
		})
	}
	for _, border := range []string{"rounded", "ascii", "double", "blue", "#abc", "#zzzzzz", ""} {
		t.Run("border "+border, func(t *testing.T) {
			t.Parallel()
			body := strings.Replace(validThemeBody, `border = "#4b5668"`, fmt.Sprintf("border = %q", border), 1)
			assertThemeLoadError(t, body, "border")
		})
	}
	for _, tt := range []struct{ name, suffix, key string }{
		{"removed warning", "warning = \"#ffffff\"\n", "warning"},
		{"unknown key", "unexpected = \"#ffffff\"\n", "unexpected"},
		{"nested code path", "[plugin]\ncommand = \"run\"\n", "plugin"},
		{"malformed TOML", "broken = [\n", ""},
		{"duplicate TOML key", "border = \"#ffffff\"\n", "border"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertThemeLoadError(t, validThemeBody+tt.suffix, tt.key)
		})
	}
}

func assertThemeLoadError(t *testing.T, body, key string) {
	t.Helper()
	_, err := Load(writeThemeFile(t, t.TempDir(), "bad.toml", body))
	if !errors.Is(err, ErrInvalidTheme) || !strings.Contains(err.Error(), key) {
		t.Fatalf("Load() = %v, want ErrInvalidTheme naming %q", err, key)
	}
}
