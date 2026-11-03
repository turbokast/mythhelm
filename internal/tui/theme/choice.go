package theme

import "fmt"

// ParseChoice validates the built-in theme selection preference.
func ParseChoice(value string) (string, error) {
	switch value {
	case "auto", "dark", "light":
		return value, nil
	default:
		return "", fmt.Errorf("invalid --theme %q: %w", value, ErrUnknownTheme)
	}
}
