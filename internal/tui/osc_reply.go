package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const maxOSCReplyRunes = 64
const oscReplyLifetime = 200 * time.Millisecond

// oscReplyState belongs to one model. Both copied key messages and parsed runes
// are bounded; the original alt-] is inert even when a candidate is rejected.
type oscReplyState struct {
	armed   bool
	expires time.Time
	text    []rune
	keys    []tea.KeyMsg
	runes   int
}

func oscPrefixKey(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyRunes && msg.Alt && len(msg.Runes) == 1 && msg.Runes[0] == ']'
}

// oscKeyRunes maps only reply-shaped key encodings. Other keys force replay.
func oscKeyRunes(msg tea.KeyMsg) ([]rune, bool) {
	if msg.Type == tea.KeyRunes {
		if !msg.Alt {
			return msg.Runes, len(msg.Runes) > 0
		}
		if len(msg.Runes) == 1 && msg.Runes[0] == '\\' {
			return []rune{'\x1b', '\\'}, true
		}
	}
	if !msg.Alt {
		switch msg.Type {
		case tea.KeyEsc:
			return []rune{'\x1b'}, true
		case tea.KeyCtrlG:
			return []rune{'\a'}, true
		}
	}
	return nil, false
}

// oscShape returns malformed=-1, incomplete=0 or complete=1. RGB has exactly
// three nonempty groups of at most four hex digits, then BEL or ESC-backslash.
func oscShape(text []rune) int {
	const prefix = "11;rgb:"
	for i, r := range text {
		if i < len(prefix) && r != rune(prefix[i]) {
			return -1
		}
	}
	if len(text) <= len(prefix) {
		return 0
	}
	group, digits := 0, 0
	for i := len(prefix); i < len(text); i++ {
		r := text[i]
		switch {
		case r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F':
			digits++
			if digits > 4 {
				return -1
			}
		case r == '/' && digits > 0 && group < 2:
			group++
			digits = 0
		case r == '\a' && group == 2 && digits > 0:
			if i == len(text)-1 {
				return 1
			}
			return -1
		case r == '\x1b' && group == 2 && digits > 0:
			if i == len(text)-1 {
				return 0
			}
			if i+2 == len(text) && text[i+1] == '\\' {
				return 1
			}
			return -1
		default:
			return -1
		}
	}
	return 0
}

func (m *Model) replayOSCKeys(keys []tea.KeyMsg) tea.Cmd {
	commands := make([]tea.Cmd, 0, len(keys))
	for _, key := range keys {
		if m.exiting {
			break
		}
		_, cmd := m.handleOrdinaryKey(key)
		if cmd != nil {
			commands = append(commands, cmd)
		}
	}
	return tea.Batch(commands...)
}

// expireOSCReply reuses the existing poll chain, never scheduling its own timer.
func (m *Model) expireOSCReply(now time.Time) tea.Cmd {
	if !m.oscReply.armed || now.Before(m.oscReply.expires) {
		return nil
	}
	keys := m.oscReply.keys
	m.oscReply = oscReplyState{}
	return m.replayOSCKeys(keys)
}

func (m *Model) handleOSCKey(msg tea.KeyMsg) tea.Cmd {
	state := &m.oscReply
	if !state.armed {
		if oscPrefixKey(msg) {
			*state = oscReplyState{armed: true, expires: time.Now().Add(oscReplyLifetime), runes: 2}
			return nil
		}
		_, cmd := m.handleOrdinaryKey(msg)
		return cmd
	}
	runes, ok := oscKeyRunes(msg)
	if !ok {
		keys := append(state.keys, msg)
		*state = oscReplyState{}
		return m.replayOSCKeys(keys)
	}
	for i, r := range runes {
		if state.runes+i+1 > maxOSCReplyRunes {
			keys := append(state.keys, msg)
			*state = oscReplyState{}
			return m.replayOSCKeys(keys)
		}
		state.text = append(state.text, r)
		switch oscShape(state.text) {
		case -1:
			keys := append(state.keys, msg)
			*state = oscReplyState{}
			return m.replayOSCKeys(keys)
		case 1:
			*state = oscReplyState{}
			if i+1 < len(runes) {
				suffix := msg
				suffix.Runes = runes[i+1:]
				return m.replayOSCKeys([]tea.KeyMsg{suffix})
			}
			return nil
		}
	}
	state.runes += len(runes)
	copyKey := msg
	copyKey.Runes = append([]rune(nil), msg.Runes...)
	state.keys = append(state.keys, copyKey)
	return nil
}
