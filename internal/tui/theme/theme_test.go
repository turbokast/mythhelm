package theme

import (
	"errors"
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
	goldens := map[string]Tokens{
		"dark": {
			Surface: "#1a1b26", SurfaceRaised: "#24283b",
			Text: "#c0caf5", TextMuted: "#9aa5ce",
			Focus: "#7aa2f7", Attention: "#bb9af7",
			OK: "#9ece6a", Warning: "#e0af68", Err: "#f7768e",
			Border: "rounded",
		},
		"light": {
			Surface: "#fafafa", SurfaceRaised: "#eef0f4",
			Text: "#1a1b26", TextMuted: "#4c5372",
			Focus: "#2959c3", Attention: "#6d4fc2",
			OK: "#2f7a3d", Warning: "#8a5a00", Err: "#b4232a",
			Border: "rounded",
		},
	}
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
	}{
		{name: "low contrast text", change: func(t *Tokens) { t.Text = "#23242f" }, wantKey: "text"},
		{name: "low contrast muted", change: func(t *Tokens) { t.TextMuted = "#2b2e40" }, wantKey: "text_muted"},
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
			if !strings.Contains(err.Error(), tt.wantKey) {
				t.Fatalf("Validate() error = %v, want it to name %q", err, tt.wantKey)
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
		{name: "missing colour", tokens: mutate(func(t *Tokens) { t.Warning = "" }), wantKey: "warning"},
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
		{name: "named colour", body: strings.Replace(validThemeBody, `focus = "#7aa2f7"`, `focus = "blue"`, 1), wantKey: "focus"},
		{name: "short hex", body: strings.Replace(validThemeBody, `ok = "#9ece6a"`, `ok = "#9e6"`, 1), wantKey: "ok"},
		{name: "missing key", body: strings.Replace(validThemeBody, "warning = \"#e0af68\"\n", "", 1), wantKey: "warning"},
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
const validThemeBody = `surface = "#1a1b26"
surface_raised = "#24283b"
text = "#c0caf5"
text_muted = "#9aa5ce"
focus = "#7aa2f7"
attention = "#bb9af7"
ok = "#9ece6a"
warning = "#e0af68"
err = "#f7768e"
border = "rounded"
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
