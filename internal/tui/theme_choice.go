package tui

import (
	"github.com/turbokast/mythhelm/internal/tui/caps"
	"github.com/turbokast/mythhelm/internal/tui/theme"
)

// ResolveTheme resolves a validated preference against a cached background answer.
func ResolveTheme(choice string, enabled bool, c caps.Caps, backgroundDark func() bool) (name string, tokens theme.Tokens, err error) {
	name, err = theme.ParseChoice(choice)
	if err != nil {
		return "", theme.Tokens{}, err
	}
	if name == "auto" {
		name = "dark"
		if enabled && c.Colour != caps.ColourNever && backgroundDark != nil && !backgroundDark() {
			name = "light"
		}
	}
	tokens, err = theme.BuiltIn(name)
	return name, tokens, err
}
