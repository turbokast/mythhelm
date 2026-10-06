// Package caps parses and resolves terminal capability preferences.
//
// Raw flag values are validated with Parse and then resolved against the
// environment with Resolve. Suppression always wins: an explicit minimal
// preference, the standard suppression variable (colour only) and TERM=dumb
// each force the minimal level for their scope, regardless of enabling
// flags elsewhere.
package caps

import (
	"errors"
	"fmt"
)

// ErrInvalidMode is returned by Parse for a mode outside the §15.7 enumerations.
var ErrInvalidMode = errors.New("invalid capability mode")

// Prefs holds raw flag values as validated by Parse.
type Prefs struct {
	Colour string
	Motion string
	Icons  string
}

// ColourLevel is the resolved colour capability. The zero value suppresses.
type ColourLevel int

// Colour levels from suppressed to full.
const (
	ColourNever ColourLevel = iota
	ColourBasic
	ColourFull
)

// MotionLevel is the resolved motion capability. The zero value suppresses.
type MotionLevel int

// Motion levels from suppressed to full.
const (
	MotionOff MotionLevel = iota
	MotionReduced
	MotionFull
)

// IconSet is the resolved icon capability. The zero value suppresses.
type IconSet int

// Icon sets from ASCII-only to unicode.
const (
	IconsASCII IconSet = iota
	IconsUnicode
)

// Caps is the resolved terminal capability set.
type Caps struct {
	Colour ColourLevel
	Motion MotionLevel
	Icons  IconSet
}

// Resolve maps validated Prefs and the environment to effective levels.
// Suppression wins from any source, in this order: explicit minimal prefs
// (never/off/ascii) always hold; the standard suppression variable set and
// non-empty forces ColourNever without touching motion or icons (AC-4.3);
// TERM=dumb forces
// all-minimal even over explicit enabling flags, since a dumb terminal
// cannot render escapes or non-ASCII glyphs. auto with no suppression
// yields full colour, full motion and unicode icons. COLORFGBG is never
// consulted (AC-4.4), and no OS reduced-motion preference is queried: the
// explicit motion flag is the portable control (D5). A Prefs value outside
// the Parse enumerations behaves as auto for its dimension.
func Resolve(p Prefs, getenv func(string) string) Caps {
	c := Caps{Colour: ColourFull, Motion: MotionFull, Icons: IconsUnicode}
	if p.Colour == "never" {
		c.Colour = ColourNever
	}
	switch p.Motion {
	case "off":
		c.Motion = MotionOff
	case "reduced":
		c.Motion = MotionReduced
	}
	if p.Icons == "ascii" {
		c.Icons = IconsASCII
	}
	if getenv("NO_COLOR") != "" { //nolint:misspell // NO_COLOR is the standard variable name.
		c.Colour = ColourNever
	}
	if getenv("TERM") == "dumb" {
		c = Caps{Colour: ColourNever, Motion: MotionOff, Icons: IconsASCII}
	}
	return c
}

// Parse validates raw flag values against the §15.7 enumerations:
// colour auto|always|never, motion auto|full|reduced|off,
// icons auto|unicode|ascii. Anything else returns an error wrapping
// ErrInvalidMode that names the offending dimension and value.
func Parse(colour, motion, icons string) (Prefs, error) {
	switch colour {
	case "auto", "always", "never":
	default:
		return Prefs{}, fmt.Errorf("invalid --colour mode %q: %w", colour, ErrInvalidMode)
	}
	switch motion {
	case "auto", "full", "reduced", "off":
	default:
		return Prefs{}, fmt.Errorf("invalid --motion mode %q: %w", motion, ErrInvalidMode)
	}
	switch icons {
	case "auto", "unicode", "ascii":
	default:
		return Prefs{}, fmt.Errorf("invalid --icons mode %q: %w", icons, ErrInvalidMode)
	}
	return Prefs{Colour: colour, Motion: motion, Icons: icons}, nil
}
