package tui

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/turbokast/mythhelm/internal/tui/caps"
	"github.com/turbokast/mythhelm/internal/tui/theme"
)

func TestThemeChoiceCountingSeam(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, choice, want string
		enabled            bool
		colour             caps.ColourLevel
		dark, nilAnswer    bool
		calls              int
	}{
		{name: "auto light", choice: "auto", want: "light", enabled: true, colour: caps.ColourFull, calls: 1},
		{name: "auto dark", choice: "auto", want: "dark", enabled: true, colour: caps.ColourFull, dark: true, calls: 1},
		{name: "basic colour light", choice: "auto", want: "light", enabled: true, colour: caps.ColourBasic, calls: 1},
		{name: "disabled auto", choice: "auto", want: "dark", colour: caps.ColourFull},
		{name: "suppressed auto", choice: "auto", want: "dark", enabled: true, colour: caps.ColourNever},
		{name: "nil answer", choice: "auto", want: "dark", enabled: true, colour: caps.ColourFull, nilAnswer: true},
		{name: "explicit light", choice: "light", want: "light", enabled: true, colour: caps.ColourFull, dark: true},
		{name: "explicit dark", choice: "dark", want: "dark", enabled: true, colour: caps.ColourFull},
		{name: "disabled explicit light", choice: "light", want: "light", colour: caps.ColourNever},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			count := 0
			answer := func() bool { count++; return tc.dark }
			if tc.nilAnswer {
				answer = nil
			}
			name, tokens, err := ResolveTheme(tc.choice, tc.enabled, caps.Caps{Colour: tc.colour}, answer)
			want, loadErr := theme.BuiltIn(tc.want)
			if err != nil || loadErr != nil || name != tc.want || tokens != want || count != tc.calls {
				t.Fatalf("ResolveTheme=%q/%+v err=%v calls=%d; want %q/%+v calls=%d", name, tokens, err, count, tc.want, want, tc.calls)
			}
			parsed, err := theme.ParseChoice(tc.choice)
			if err != nil || parsed != tc.choice {
				t.Fatalf("ParseChoice(%q)=%q,%v", tc.choice, parsed, err)
			}
		})
	}
	for _, value := range []string{"", "Light", " auto", "paper", "dark\n"} {
		t.Run("invalid "+value, func(t *testing.T) {
			t.Parallel()
			for _, enabled := range []bool{false, true} {
				count := 0
				_, _, err := ResolveTheme(value, enabled, fullCaps, func() bool { count++; return true })
				if !errors.Is(err, theme.ErrUnknownTheme) || !strings.Contains(err.Error(), "--theme") || !strings.Contains(err.Error(), strconv.Quote(value)) || count != 0 {
					t.Fatalf("invalid %q enabled=%v err=%v calls=%d", value, enabled, err, count)
				}
				_, err = theme.ParseChoice(value)
				if !errors.Is(err, theme.ErrUnknownTheme) {
					t.Fatalf("ParseChoice invalid %q err=%v", value, err)
				}
			}
		})
	}
}

func TestThemeTogglePersists(t *testing.T) {
	t.Parallel()
	for _, chosen := range []string{"light", "dark"} {
		t.Run(chosen, func(t *testing.T) {
			t.Parallel()
			cfg := testConfig(fullCaps, t)
			var err error
			cfg.ThemeName, cfg.Tokens, err = ResolveTheme(chosen, true, fullCaps, nil)
			if err != nil {
				t.Fatal(err)
			}
			m := newModel(cfg)
			if m.themeName != chosen || m.cfg.Tokens != cfg.Tokens {
				t.Fatalf("initial theme=%q tokens=%+v", m.themeName, m.cfg.Tokens)
			}
			m, _ = pressKey(t, m, runeMsg(":"))
			m = typeRunes(t, m, "toggle")
			m, _ = pressKey(t, m, specialMsg(tea.KeyEnter))
			want := "light"
			if chosen == "light" {
				want = "dark"
			}
			tokens, err := theme.BuiltIn(want)
			if err != nil {
				t.Fatal(err)
			}
			for i := range 2 {
				m, _ = pressKey(t, m, runeMsg("?"))
				_, _ = m.Update(tea.WindowSizeMsg{Width: 100 + i*60, Height: 20 + i*10})
				m, _ = pressKey(t, m, specialMsg(tea.KeyEsc))
				if m.themeName != want || m.cfg.Tokens != tokens || m.helpOpen {
					t.Fatalf("theme lost through overlay/resize: %q %+v help=%v", m.themeName, m.cfg.Tokens, m.helpOpen)
				}
			}
		})
	}
}
