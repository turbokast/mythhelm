package security

import (
	"strings"
	"unicode/utf8"
)

const esc = 0x1b

// TermSafe removes everything from s that a terminal would interpret rather
// than print: C0 controls except '\n' and '\t', DEL, C1 controls (as UTF-8
// runes or raw 8-bit bytes) and whole escape sequences, including CSI and the
// string-bearing OSC, DCS, SOS, PM and APC (so an OSC 8 hyperlink keeps only
// its visible text). A control string ends at BEL, ST or a newline, so an
// unterminated one cannot hide the following lines. Invalid UTF-8 bytes
// outside the C1 range become U+FFFD (§12.7, AC-8.2).
func TermSafe(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 {
			r = rune(s[i]) // a raw 8-bit byte: C1 if 0x80–0x9f
			if r < 0x80 || r > 0x9f {
				b.WriteRune(utf8.RuneError)
				i++
				continue
			}
		}
		switch {
		case r == esc:
			i = skipEscape(s, i+n)
		case r >= 0x80 && r <= 0x9f:
			i = skipC1(s, r, i+n)
		case r < 0x20 && r != '\n' && r != '\t', r == 0x7f:
			i += n
		default:
			b.WriteString(s[i : i+n])
			i += n
		}
	}
	return b.String()
}

// skipEscape skips the escape sequence whose ESC ends just before i.
func skipEscape(s string, i int) int {
	if i >= len(s) {
		return i
	}
	switch c := s[i]; {
	case c == '[' || c == ']' || c == 'P' || c == 'X' || c == '^' || c == '_':
		return skipC1(s, rune(c)+0x40, i+1) // ESC Fe is the 7-bit form of C1 0x80+(c-0x40)
	case c >= 0x20 && c <= 0x2f: // intermediates, then one final byte
		for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
			i++
		}
		if i < len(s) && s[i] >= 0x30 && s[i] <= 0x7e {
			i++
		}
		return i
	case c >= 0x30 && c <= 0x7e:
		return i + 1
	default: // a bare ESC: drop it and keep what follows
		return i
	}
}

// skipC1 skips the body of the C1 control c that ends just before i.
func skipC1(s string, c rune, i int) int {
	switch c {
	case 0x9b: // CSI: parameters and intermediates, then one final byte
		for i < len(s) && s[i] >= 0x20 && s[i] <= 0x3f {
			i++
		}
		if i < len(s) && s[i] >= 0x40 && s[i] <= 0x7e {
			i++
		}
		return i
	case 0x90, 0x98, 0x9d, 0x9e, 0x9f: // DCS, SOS, OSC, PM, APC: a control string
		for i < len(s) {
			r, n := utf8.DecodeRuneInString(s[i:])
			switch {
			case r == '\a', r == 0x9c, r == utf8.RuneError && n == 1 && s[i] == 0x9c:
				return i + n
			case r == esc && strings.HasPrefix(s[i+1:], `\`):
				return i + 2
			case r == '\n':
				return i
			}
			i += n
		}
		return i
	default:
		return i
	}
}
