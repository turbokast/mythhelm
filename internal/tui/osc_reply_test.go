package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

func oscPrefix() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Alt: true, Runes: []rune{']'}} }

func TestLateOSCReplyNotKeys(t *testing.T) {
	t.Parallel()
	m := flowModel(t)
	acts := &fakeActions{}
	m.cfg.Actions = acts.actions()
	m, _ = pressKey(t, m, oscPrefix())
	for _, r := range "11;rgb:dddd/0000/0000" {
		m, _ = pressKey(t, m, runeMsg(string(r)))
	}
	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Alt: true, Runes: []rune{'\\'}})
	if m.palette.open || m.filterOn || m.dialog != "" || m.working != "" || acts.applies+acts.recovers+acts.stops != 0 {
		t.Fatalf("OSC reply acted as keys: palette=%v filter=%v dialog=%q working=%q actions=%+v", m.palette.open, m.filterOn, m.dialog, m.working, acts)
	}
	control := flowModel(t)
	control.filterOn = true
	for _, r := range "11;rgb:dddd/0000/0000" {
		control, _ = pressKey(t, control, runeMsg(string(r)))
	}
	if control.filterText != "11;rgb:dddd/0000/0000" {
		t.Fatalf("prefix-free keys swallowed: %q", control.filterText)
	}
}

func TestOSCReplyGroupedAndSplit(t *testing.T) {
	t.Parallel()
	body := "11;rgb:dddd/0000/0000"
	for _, tc := range []struct {
		name string
		keys []tea.KeyMsg
	}{
		{"grouped ST", []tea.KeyMsg{runeMsg(body + "\x1b\\")}},
		{"grouped BEL", []tea.KeyMsg{runeMsg(body + "\a")}},
		{"special BEL", []tea.KeyMsg{runeMsg(body), specialMsg(tea.KeyCtrlG)}},
		{"split ST", []tea.KeyMsg{runeMsg("11;"), runeMsg("rgb:"), runeMsg("dddd/0000/0000"), specialMsg(tea.KeyEsc), runeMsg("\\")}},
		{"short uppercase RGB", []tea.KeyMsg{runeMsg("11;rgb:A/bC/0123"), specialMsg(tea.KeyCtrlG)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, overlay := range []string{"mission", "filter", "palette", "help", "dialog"} {
				m := flowModel(t)
				switch overlay {
				case "filter":
					m.filterOn = true
					m.filterText = "original"
				case "palette":
					m.openPalette()
					m.palette.input = "original"
				case "help":
					m.helpOpen = true
				case "dialog":
					m.openDialog(dialogExitOptions)
				}
				beforeFilter, beforePalette, beforeDialog := m.filterText, m.palette.input, m.dialog
				m, _ = pressKey(t, m, oscPrefix())
				for _, key := range tc.keys {
					var cmd tea.Cmd
					m, cmd = pressKey(t, m, key)
					if cmd != nil {
						t.Fatalf("%s reply scheduled command", overlay)
					}
				}
				if m.oscReply.armed || m.filterText != beforeFilter || m.palette.input != beforePalette || m.dialog != beforeDialog || m.helpOpen != (overlay == "help") || m.palette.open != (overlay == "palette") || m.filterOn != (overlay == "filter") {
					t.Fatalf("%s reply changed input state: filter=%v/%q palette=%v/%q help=%v dialog=%q armed=%v", overlay, m.filterOn, m.filterText, m.palette.open, m.palette.input, m.helpOpen, m.dialog, m.oscReply.armed)
				}
			}
		})
	}
	m := flowModel(t)
	m, _ = pressKey(t, m, oscPrefix())
	m, _ = pressKey(t, m, runeMsg(body+"\a?"))
	if !m.helpOpen || m.oscReply.armed {
		t.Fatal("ordinary key following grouped terminator was swallowed")
	}
}

func TestOSCReplyMalformedAndOverlongReplay(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"x", "11;rgb:zz", "11;rgb:aaaaa", "11;rgb:/0/0", "11;rgb:0//0", "11;rgb:0/0/0/", "11;rgb:" + strings.Repeat("d", 1024)} {
		t.Run(bad[:min(len(bad), 24)], func(t *testing.T) {
			t.Parallel()
			m := flowModel(t)
			m.filterOn = true
			m, _ = pressKey(t, m, oscPrefix())
			m, _ = pressKey(t, m, runeMsg(bad))
			want := []rune(bad)
			if len(want) > maxFilterRunes {
				want = want[:maxFilterRunes]
			}
			if m.filterText != string(want) || m.oscReply.armed || len(m.oscReply.text) > maxOSCReplyRunes || len(m.oscReply.keys) > maxOSCReplyRunes {
				t.Fatalf("malformed input not released/bounded: text=%q state=%+v", m.filterText, m.oscReply)
			}
		})
	}
	// Replay uses the ordinary confirmation gate, not an action shortcut.
	m := flowModel(t)
	acts := &fakeActions{}
	m.cfg.Actions = acts.actions()
	m, _ = pressKey(t, m, oscPrefix())
	m, _ = pressKey(t, m, runeMsg("a"))
	if m.dialog != dialogApply || m.dialogFocus != buttonCancel || acts.applies != 0 {
		t.Fatalf("replay bypassed/lost confirmation: dialog=%q focus=%d calls=%d", m.dialog, m.dialogFocus, acts.applies)
	}
	m, _ = pressKey(t, m, specialMsg(tea.KeyEnter))
	if m.dialog != dialogApply || m.dialogField != applyFieldFlags || acts.applies != 0 {
		t.Fatal("first Enter must advance the ordinary branch field without applying")
	}
	m, _ = pressKey(t, m, specialMsg(tea.KeyEsc))
	if m.dialog != "" || acts.applies != 0 {
		t.Fatal("Esc must cancel the replay-opened dialog without applying")
	}
	// Plain r remains an ordinary recover key when that action is eligible.
	m = flowModel(t)
	m.snap.Run.State = "interrupted"
	m.cfg.Actions = acts.actions()
	m, _ = pressKey(t, m, runeMsg("r"))
	if m.dialog != dialogRecover {
		t.Fatalf("plain r suppressed: dialog=%q", m.dialog)
	}
}

func TestOSCReplyExpiryUsesExistingPoll(t *testing.T) {
	t.Parallel()
	m := flowModel(t)
	m.filterOn = true
	m.managedPoll = true
	m, _ = pressKey(t, m, oscPrefix())
	key := runeMsg("11;rgb:")
	m, _ = pressKey(t, m, key)
	key.Runes[0] = 'x' // Buffered input owns its bounded copy.
	if cmd := m.expireOSCReply(m.oscReply.expires.Add(-time.Nanosecond)); cmd != nil || !m.oscReply.armed || m.filterText != "" {
		t.Fatal("candidate expired before deadline")
	}
	m.oscReply.expires = time.Time{}
	snap := m.snap
	m.loadSnapshot = func(context.Context, string, string) (viewmodel.Snapshot, error) { return snap, nil }
	t.Cleanup(func() { m.cancelReload(); m.joinReload() })
	_, _ = m.Update(pollTickMsg{})
	if m.oscReply.armed || m.filterText != "11;rgb:" {
		t.Fatalf("existing poll lost ordinary suffix: armed=%v filter=%q", m.oscReply.armed, m.filterText)
	}
	m, _ = pressKey(t, m, runeMsg("d/r"))
	if m.filterText != "11;rgb:d/r" {
		t.Fatalf("ordinary d/r after expiry swallowed: %q", m.filterText)
	}
}

func TestOSCReplyModelIsolationAndBound(t *testing.T) {
	t.Parallel()
	a, b := flowModel(t), flowModel(t)
	a.filterOn = true
	a, _ = pressKey(t, a, oscPrefix())
	a, _ = pressKey(t, a, runeMsg("11;"))
	b, _ = pressKey(t, b, runeMsg("/"))
	if !b.filterOn || b.oscReply.armed {
		t.Fatal("one model armed another")
	}
	for _, key := range a.oscReply.keys {
		if len(key.Runes) > maxOSCReplyRunes {
			t.Fatal("retained unbounded key")
		}
	}
	if a.oscReply.runes > maxOSCReplyRunes || len(a.oscReply.text) > maxOSCReplyRunes {
		t.Fatal("reply exceeded bound")
	}
	a.oscReply.expires = time.Time{}
	_ = a.expireOSCReply(time.Time{})
	if a.filterText != "11;" {
		t.Fatalf("partial input lost: %q", a.filterText)
	}
}

// A terminal read may group a complete bounded reply with later ordinary input;
// that suffix cannot make the already-terminated reply look overlong.
func TestOSCReplyBoundAppliesToCandidateNotTrailingKeys(t *testing.T) {
	t.Parallel()
	m := flowModel(t)
	m.filterOn = true
	m, _ = pressKey(t, m, oscPrefix())
	m, _ = pressKey(t, m, runeMsg("11;rgb:dddd/0000/0000\a"+strings.Repeat("x", 256)))
	if m.filterText != strings.Repeat("x", maxFilterRunes) || m.oscReply.armed {
		t.Fatalf("reply leaked into grouped trailing keys: filter=%q armed=%v", m.filterText, m.oscReply.armed)
	}
}
