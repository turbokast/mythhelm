package caps_test

import (
	"errors"
	"testing"

	"github.com/turbokast/mythhelm/internal/tui/caps"
)

// noColorEnv is the standard colour-suppression variable name.
const noColorEnv = "NO_COLOR" //nolint:misspell // NO_COLOR is the standard variable name.

func envOf(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func mustParse(t *testing.T, colour, motion, icons string) caps.Prefs {
	t.Helper()
	p, err := caps.Parse(colour, motion, icons)
	if err != nil {
		t.Fatalf("Parse(%q, %q, %q) error = %v, want nil", colour, motion, icons, err)
	}
	return p
}

func TestParseRejectsUnknown(t *testing.T) {
	t.Parallel()

	t.Run("unknown modes rejected", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name           string
			colour, motion string
			icons          string
		}{
			{name: "colour rainbow", colour: "rainbow", motion: "auto", icons: "auto"},
			{name: "motion turbo", colour: "auto", motion: "turbo", icons: "auto"},
			{name: "icons emoji", colour: "auto", motion: "auto", icons: "emoji"},
			{name: "empty colour", colour: "", motion: "auto", icons: "auto"},
			{name: "empty motion", colour: "auto", motion: "", icons: "auto"},
			{name: "empty icons", colour: "auto", motion: "auto", icons: ""},
			{name: "wrong case colour", colour: "Always", motion: "auto", icons: "auto"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				_, err := caps.Parse(tt.colour, tt.motion, tt.icons)
				if !errors.Is(err, caps.ErrInvalidMode) {
					t.Fatalf("Parse(%q, %q, %q) error = %v, want ErrInvalidMode", tt.colour, tt.motion, tt.icons, err)
				}
			})
		}
	})

	t.Run("every enumeration value parses", func(t *testing.T) {
		t.Parallel()
		colours := []string{"auto", "always", "never"}
		motions := []string{"auto", "full", "reduced", "off"}
		iconsets := []string{"auto", "unicode", "ascii"}
		for _, c := range colours {
			for _, m := range motions {
				for _, i := range iconsets {
					got, err := caps.Parse(c, m, i)
					if err != nil {
						t.Errorf("Parse(%q, %q, %q) error = %v, want nil", c, m, i, err)
						continue
					}
					if got.Colour != c || got.Motion != m || got.Icons != i {
						t.Errorf("Parse(%q, %q, %q) = %+v, want echo of inputs", c, m, i, got)
					}
				}
			}
		}
	})
}

func TestNoColorSuppressesColourOnly(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		prefs [3]string
		env   map[string]string
		want  caps.Caps
	}{
		{
			name:  "auto flags with suppression set keep motion and icons",
			prefs: [3]string{"auto", "auto", "auto"},
			env:   map[string]string{noColorEnv: "1"},
			want:  caps.Caps{Colour: caps.ColourNever, Motion: caps.MotionFull, Icons: caps.IconsUnicode},
		},
		{
			name:  "suppression beats explicit always",
			prefs: [3]string{"always", "full", "unicode"},
			env:   map[string]string{noColorEnv: "1"},
			want:  caps.Caps{Colour: caps.ColourNever, Motion: caps.MotionFull, Icons: caps.IconsUnicode},
		},
		{
			name:  "empty suppression value does not suppress",
			prefs: [3]string{"auto", "auto", "auto"},
			env:   map[string]string{noColorEnv: ""},
			want:  caps.Caps{Colour: caps.ColourFull, Motion: caps.MotionFull, Icons: caps.IconsUnicode},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := caps.Resolve(mustParse(t, tt.prefs[0], tt.prefs[1], tt.prefs[2]), envOf(tt.env))
			if got != tt.want {
				t.Fatalf("Resolve(%v, %v) = %+v, want %+v", tt.prefs, tt.env, got, tt.want)
			}
		})
	}
}

func TestExplicitNeverWins(t *testing.T) {
	t.Parallel()
	envs := []struct {
		name string
		env  map[string]string
	}{
		{name: "empty", env: map[string]string{}},
		{name: "full colour terminfo", env: map[string]string{"TERM": "xterm-256color"}},
		{name: "suppression set", env: map[string]string{noColorEnv: "1"}},
		{name: "dumb term", env: map[string]string{"TERM": "dumb"}},
	}
	for _, ee := range envs {
		t.Run("colour never/"+ee.name, func(t *testing.T) {
			t.Parallel()
			got := caps.Resolve(mustParse(t, "never", "auto", "auto"), envOf(ee.env))
			if got.Colour != caps.ColourNever {
				t.Fatalf("Resolve(never, env %v).Colour = %v, want ColourNever", ee.env, got.Colour)
			}
		})
		t.Run("motion off/"+ee.name, func(t *testing.T) {
			t.Parallel()
			got := caps.Resolve(mustParse(t, "auto", "off", "auto"), envOf(ee.env))
			if got.Motion != caps.MotionOff {
				t.Fatalf("Resolve(off, env %v).Motion = %v, want MotionOff", ee.env, got.Motion)
			}
		})
		t.Run("icons ascii/"+ee.name, func(t *testing.T) {
			t.Parallel()
			got := caps.Resolve(mustParse(t, "auto", "auto", "ascii"), envOf(ee.env))
			if got.Icons != caps.IconsASCII {
				t.Fatalf("Resolve(ascii, env %v).Icons = %v, want IconsASCII", ee.env, got.Icons)
			}
		})
	}
}

func TestDumbTermMinimal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		prefs [3]string
	}{
		{name: "auto flags", prefs: [3]string{"auto", "auto", "auto"}},
		{name: "explicit enabling flags still minimal", prefs: [3]string{"always", "full", "unicode"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := caps.Resolve(mustParse(t, tt.prefs[0], tt.prefs[1], tt.prefs[2]), envOf(map[string]string{"TERM": "dumb"}))
			want := caps.Caps{Colour: caps.ColourNever, Motion: caps.MotionOff, Icons: caps.IconsASCII}
			if got != want {
				t.Fatalf("Resolve(%v, TERM=dumb) = %+v, want %+v", tt.prefs, got, want)
			}
		})
	}
}

func TestAutoDefaults(t *testing.T) {
	t.Parallel()
	got := caps.Resolve(mustParse(t, "auto", "auto", "auto"), envOf(map[string]string{}))
	want := caps.Caps{Colour: caps.ColourFull, Motion: caps.MotionFull, Icons: caps.IconsUnicode}
	if got != want {
		t.Fatalf("Resolve(auto, empty env) = %+v, want %+v", got, want)
	}
}

func TestColorFgBgNeverConsulted(t *testing.T) {
	t.Parallel()
	baseline := caps.Resolve(mustParse(t, "auto", "auto", "auto"), envOf(map[string]string{}))
	for _, v := range []string{"15;0", "0;15"} {
		got := caps.Resolve(mustParse(t, "auto", "auto", "auto"), envOf(map[string]string{"COLORFGBG": v}))
		if got != baseline {
			t.Fatalf("Resolve(auto, COLORFGBG=%q) = %+v, want %+v (same as empty env)", v, got, baseline)
		}
	}
}
