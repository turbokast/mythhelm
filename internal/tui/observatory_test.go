package tui

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/tui/caps"
	"github.com/turbokast/mythhelm/internal/tui/theme"
)

func observatoryFixture(t *testing.T) *Model {
	t.Helper()
	m := newModel(testConfig(fullCaps, t))
	m.snap = richSnapshot()
	m.ready = true
	m.snap.Goal = "Recorded goal"
	m.snap.Model = "native-model"
	m.snap.Run.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m.snap.Run.UpdatedAt = m.snap.Run.CreatedAt.Add(80 * time.Second)
	m.snap.At = m.snap.Run.CreatedAt.Add(121 * time.Second)
	return m
}
func observatoryPlain(rows []line) string {
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(r.text)
		b.WriteByte('\n')
	}
	return b.String()
}
func observatoryStates(t *testing.T) map[string]bool {
	t.Helper()
	states := map[string]bool{"created": true, "admission": true, "executing": true, "verifying": true, "ready_for_review": false, "applying": true, "completed": false, "stopping": true, "cancelled": false, "interrupted": false, "recovering": true, "blocked": false, "failed": false}
	words := map[string]string{}
	for s := range states {
		words[s] = s
	}
	raw, err := os.ReadFile(filepath.Join("..", "supervisor", "state.go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := checkRunStateInventory(raw, words); err != nil {
		t.Fatal(err)
	}
	return states
}
func TestHeaderIdentityAndElapsed(t *testing.T) {
	t.Parallel()
	m := observatoryFixture(t)
	m.snap.Run.State = "executing"
	m.snap.Attempt.State = "running"
	for _, tc := range []struct {
		seconds int
		want    string
	}{{0, "0:00"}, {59, "0:59"}, {60, "1:00"}, {3599, "59:59"}, {3600, "1:00:00"}, {3661, "1:01:01"}, {-9, "0:00"}} {
		m.snap.At = m.snap.Run.CreatedAt.Add(time.Duration(tc.seconds) * time.Second)
		if got := m.observatoryElapsed(); got != tc.want {
			t.Errorf("elapsed(%d)=%q want %q", tc.seconds, got, tc.want)
		}
	}
	m.snap.At = m.snap.Run.CreatedAt.Add(121 * time.Second)
	for _, width := range []int{160, 140, 139, 100, 99, 59, 1, 0} {
		got := m.observatoryHeader(width)
		if lipgloss.Width(got.text) > width {
			t.Fatalf("header width %d: %q", width, got.text)
		}
		if width < 59 {
			continue
		}
		if strings.Contains(got.text, "MYTHHELM") != (width >= 100) {
			t.Errorf("wordmark threshold %d: %q", width, got.text)
		}
		for _, identity := range []string{"builtin/fake", "native-model", "trusted-host"} {
			if strings.Contains(got.text, identity) != (width >= 140) {
				t.Errorf("identity threshold %d: %q", width, got.text)
			}
		}
		if !strings.Contains(got.text, "Recorded goal") || !strings.Contains(got.text, "2:01") || strings.Contains(got.text, m.snap.Run.RunID) {
			t.Errorf("title/state/time or run-ID separation: %q", got.text)
		}
		if width >= 140 && !strings.HasSuffix(got.text, "trusted-host") {
			t.Errorf("identity not at right edge: %q", got.text)
		}
	}
	m.snap.Model = ""
	row := m.observatoryHeader(160)
	underlined := false
	for _, s := range row.spans {
		if s.text == "unknown" && s.underline {
			underlined = true
		}
		if s.colour == m.cfg.Tokens.Focus && lipgloss.Width(s.text) != 1 {
			t.Errorf("long focus span: %+v", s)
		}
	}
	if !underlined {
		t.Fatalf("missing underlined unknown model: %+v", row.spans)
	}
	m.snap.Goal = strings.Repeat("界e\u0301", 100) + "\x1b]0;hidden\a"
	m.snap.Model = strings.Repeat("model", 100) + "\x1b[31m"
	m.snap.Run.ExecutionProfile = "trusted\x1b[0m"
	for _, width := range []int{160, 140, 100, 99, 59} {
		row := m.observatoryHeader(width)
		if lipgloss.Width(row.text) > width || strings.ContainsAny(row.text, "\x1b\n\t") || strings.Contains(row.text, "hidden") {
			t.Errorf("unsafe/overwide header %d: %q", width, row.text)
		}
		if !strings.Contains(row.text, "2:01") {
			t.Errorf("title displaced elapsed: %q", row.text)
		}
		if !strings.Contains(row.text, "e\u0301") {
			t.Errorf("combining title lost: %q", row.text)
		}
	}
	m.snap = richSnapshot()
	m.snap.Goal = "Task title"
	m.snap.Run.State = "failed"
	rows := m.tasksRows(100)
	if len(rows) < 3 || !strings.Contains(rows[2].text, m.snap.Run.RunID) || !strings.Contains(rows[2].text, "Task title") || !strings.Contains(rows[2].text, "failed") {
		t.Fatalf("first task row lost identity/state: %+v", rows)
	}
	if len(rows[2].spans) == 0 {
		t.Fatal("failure row has no spans")
	}
	for _, s := range rows[2].spans {
		if s.colour != m.cfg.Tokens.Err {
			t.Fatalf("real task failure emphasis moved: %+v", s)
		}
	}
}

func TestNowNextActionGates(t *testing.T) {
	t.Parallel()
	m := observatoryFixture(t)
	base := m.cfg
	wired := Actions{Stop: func(context.Context, string) (string, error) { return "", nil }, Recover: func(context.Context, string, supervisor.Hooks) (supervisor.RecoveryOutcome, error) {
		return supervisor.RecoveryOutcome{}, nil
	}, Apply: func(context.Context, string, string, bool, bool) (supervisor.Receipt, error) {
		return supervisor.Receipt{}, nil
	}}
	for state := range observatoryStates(t) {
		for _, mode := range []ClientMode{ModeLive, ModeInspect} {
			for _, connected := range []bool{false, true} {
				for _, worker := range []string{"", "launching", "running", "stop_requested"} {
					for _, pending := range []bool{false, true} {
						for _, capability := range []string{"unknown", "supported"} {
							m.cfg = base
							m.cfg.Mode = mode
							m.cfg.Actions = Actions{}
							m.cfg.RequestStop = nil
							m.cfg.Detach = nil
							if connected {
								m.cfg.Actions = wired
								m.cfg.RequestStop = func() {}
								m.cfg.Detach = func() {}
							}
							m.ready = true
							m.stopRequested = pending
							m.stopSent = false
							m.snap = richSnapshot()
							m.snap.Run.State = state
							m.snap.Supervision = "unknown"
							m.snap.Admission.WorkerDetachment = capability
							if worker == "" {
								m.snap.Attempt = nil
							} else {
								m.snap.Attempt.State = worker
							}
							text := observatoryPlain(m.nowNext(600))
							keys := observatoryPlain(m.observatoryKeys(600, false))
							if !strings.Contains(text, "NOW ") || !strings.Contains(text, "NEXT ") {
								t.Fatalf("missing labelled shared row: %q", text)
							}
							for _, a := range []struct {
								action Action
								label  string
							}{{actionStop, "s stop"}, {actionDetach, "d detach"}, {actionRecover, "r recover"}, {actionApply, "a apply"}} {
								ok, _ := m.actionGate(a.action)
								if !ok && (strings.Contains(text, a.label) || strings.Contains(keys, a.label)) {
									t.Fatalf("%s mode%d connected%v worker%s pending%v offers closed %q: %q %q", state, mode, connected, worker, pending, a.label, text, keys)
								}
							}
							if (state == "blocked" || state == "failed") && strings.Contains(m.observatoryNext(), "recover") {
								t.Fatalf("terminal suggests recovery: %q", m.observatoryNext())
							}
							if state == "stopping" {
								for _, body := range []string{m.observatoryNow(), m.observatoryNext()} {
									if !strings.Contains(body, "stop requested") || strings.Contains(body, "stopped") {
										t.Fatalf("stopping claimed confirmation: %q", body)
									}
								}
							}
						}
					}
				}
			}
		}
	}
	m.cfg = base
	m.snap = richSnapshot()
	m.snap.Run.State = "executing"
	m.snap.Attempt.State = "running"
	now := m.observatoryNow()
	for _, want := range []string{"lane A", "running", "7 assistant turns (native-reported)", "retries: 1", "target unknown"} {
		if !strings.Contains(now, want) {
			t.Errorf("activity lacks %q: %q", want, now)
		}
	}
	if strings.Contains(now, "main.go") || strings.Contains(now, "/tmp") || strings.Contains(now, "%") {
		t.Fatalf("invented target/progress: %q", now)
	}
	m.snap.Run.State = "completed"
	m.snap.Verification.Checks = []journal.CheckRow{{Status: "passed"}, {Status: "failed"}, {Status: "unknown"}, {Status: "passed"}}
	if !strings.Contains(m.observatoryNow(), "passed 2 of 4 checks") {
		t.Fatalf("overall proof substituted for individual checks: %q", m.observatoryNow())
	}
	m.snap.Verification = nil
	if !strings.Contains(m.observatoryNow(), "no checks recorded") {
		t.Fatal("nil checks invented")
	}
	for _, w := range []int{79, 72, 59} {
		rows := m.nowNext(w)
		if len(rows) != 2 || !strings.HasPrefix(rows[0].text, "now ") || !strings.HasPrefix(rows[1].text, "next ") {
			t.Fatalf("compact shape: %+v", rows)
		}
		for _, r := range rows {
			if lipgloss.Width(r.text) > w {
				t.Fatalf("overwide NOW/NEXT: %+v", rows)
			}
		}
	}
}

func TestObservatoryKeySlots(t *testing.T) {
	t.Parallel()
	m := observatoryFixture(t)
	m.cfg.Mode = ModeLive
	m.cfg.RequestStop = func() {}
	m.cfg.Detach = func() {}
	m.snap.Run.State = "executing"
	m.snap.Attempt.State = "running"
	m.snap.Admission.WorkerDetachment = "supported"
	for _, name := range []string{"dark", "light"} {
		tokens, err := theme.BuiltIn(name)
		if err != nil {
			t.Fatal(err)
		}
		m.cfg.Tokens = tokens
		for _, width := range []int{160, 140, 100, 99, 90, 89, 60, 59} {
			rows := m.observatoryKeys(width, false)
			last := rows[len(rows)-1]
			if !strings.Contains(last.text, "q exit") || !strings.HasSuffix(last.text, "q exit"+ellGlyph(m.cfg.Caps)) {
				t.Fatalf("q not retained/rightmost %d: %q", width, last.text)
			}
			if lipgloss.Width(last.text) > width {
				t.Fatalf("key row overflow: %q", last.text)
			}
			if width < 100 && (strings.Contains(last.text, ":") || strings.Contains(last.text, "/") || strings.Contains(last.text, "nothing needs you")) {
				t.Fatalf("narrow slots remain: %q", last.text)
			}
			if width < 90 && strings.Contains(last.text, "detach") {
				t.Fatalf("detach threshold: %q", last.text)
			}
			previous := -1
			for _, k := range []string{"Enter", "s", "d", "?", ":", "/"} {
				at := -1
				for i, s := range last.spans {
					if s.text == k {
						at = i
						if !s.bold || s.colour != tokens.Text {
							t.Fatalf("key style: %+v", s)
						}
					}
				}
				if at >= 0 && at < previous {
					t.Fatalf("key order: %+v", last.spans)
				}
				if at >= 0 {
					previous = at
				}
			}
			for _, s := range last.spans {
				if strings.HasPrefix(s.text, " ") && strings.TrimSpace(s.text) != "" && s.colour != tokens.TextMuted {
					t.Fatalf("key label not muted: %+v", s)
				}
			}
			rendered := renderLine(m.cfg.Caps, last, width)
			if strings.Contains(rendered, "\x1b[48;") {
				t.Fatal("chip background introduced")
			}
		}
	}
	m.cfg.Tokens = darkTokens(t)
	for state := range observatoryStates(t) {
		m.snap.Run.State = state
		want := state == "blocked" || state == "failed" || state == "interrupted"
		rows := m.observatoryKeys(140, false)
		found := false
		for _, r := range rows {
			for _, s := range r.spans {
				if s.colour == m.cfg.Tokens.Attention {
					found = true
					if !strings.Contains(s.text, "needs you") {
						t.Errorf("attention outside need: %+v", s)
					}
				}
			}
		}
		if found != want {
			t.Fatalf("attention %s=%v want%v", state, found, want)
		}
	}
	m.snap.Run.State = "blocked"
	for _, width := range []int{79, 72, 60, 59, 32} {
		rows := m.observatoryKeys(width, true)
		last := rows[len(rows)-1]
		if !strings.Contains(last.text, "q exit") || !strings.HasSuffix(last.text, "--accessible: linear") {
			t.Fatalf("mandatory compact group lost %d: %+v", width, rows)
		}
		if !strings.Contains(observatoryPlain(rows), "needs you") {
			t.Fatal("attention lost")
		}
		for _, r := range rows {
			if lipgloss.Width(r.text) > width {
				t.Fatalf("compact key overflow %d: %+v", width, rows)
			}
		}
		if width == 32 && (len(rows) != 2 || !strings.HasPrefix(rows[0].text, "NEXT ")) {
			t.Fatalf("attention not relocated: %+v", rows)
		}
	}
	// At60 the mandatory group plus Enter fits, but help does not. Reverse
	// dropping must preserve Enter rather than discarding the first slot.
	rows := m.observatoryKeys(60, true)
	last := rows[len(rows)-1]
	if !strings.Contains(last.text, "Enter inspect") || strings.Contains(last.text, "help") {
		t.Fatalf("reverse pressure dropping failed: %+v", rows)
	}
	m.cfg.Caps = caps.Caps{Colour: caps.ColourNever, Icons: caps.IconsUnicode}
	if got := renderLine(m.cfg.Caps, last, 60); strings.Contains(got, "\x1b") {
		t.Fatalf("ColourNever escapes: %q", got)
	}
}

func TestClockAndAttentionTruth(t *testing.T) {
	t.Parallel()
	m := observatoryFixture(t)
	for state, active := range observatoryStates(t) {
		m.snap.Run.State = state
		want := "1:20"
		if active {
			want = "2:01"
		}
		if got := m.observatoryElapsed(); got != want {
			t.Errorf("%s clock=%q want%q", state, got, want)
		}
	}
	m.snap.Run.State = "executing"
	m.snap.Attempt.State = "running"
	m.snap.At = m.snap.Run.CreatedAt.Add(1200 * time.Millisecond)
	first := m.observatoryHeader(160).text
	m.snap.At = m.snap.Run.CreatedAt.Add(1900 * time.Millisecond)
	if m.observatoryHeader(160).text != first {
		t.Fatal("subsecond display changed")
	}
	m.snap.At = m.snap.Run.CreatedAt.Add(2 * time.Second)
	if m.observatoryHeader(160).text == first {
		t.Fatal("second boundary did not advance")
	}
	m.snap.At = m.snap.Run.CreatedAt.Add(121 * time.Second)
	for _, observation := range []string{"", "unknown", "stale", "supervised"} {
		m.snap.Supervision = observation
		if m.inspectUnsupervised() || strings.Contains(m.observatoryNow(), "unsupervised") || m.observatoryElapsed() != "2:01" {
			t.Fatalf("unknown evidence inferred no owner: %q", observation)
		}
	}
	for _, state := range []string{"executing", "stopping"} {
		m.snap.Run.State = state
		m.snap.Supervision = "unsupervised"
		if m.observatoryElapsed() != "1:20" || !strings.Contains(m.observatoryNext(), "mythhelm recover "+m.snap.Run.RunID) || strings.Contains(m.observatoryNow(), "assistant turns") {
			t.Fatalf("proven inspect observation lost: %q %q", m.observatoryNow(), m.observatoryNext())
		}
		if !m.observatoryAttention() {
			t.Fatal("unsupervised has no attention")
		}
	}
	m.cfg.Mode = ModeLive
	if m.inspectUnsupervised() || m.observatoryElapsed() != "2:01" || strings.Contains(m.observatoryNow(), "unsupervised") {
		t.Fatal("live view interpreted inspect observation")
	}
	m.snap.Run.State = "failed"
	m.motion.deliveryFrames = 1
	m.notices = []journal.Event{noticeEvent("recorded notice", m.snap.At)}
	rows := append(m.tasksRows(100), m.noticeRows(2)...)
	rows = append(rows, m.criticalStatusLine())
	for _, r := range rows {
		if r.colour == m.cfg.Tokens.Attention {
			t.Errorf("attention outside slot: %+v", r)
		}
	}
	file, err := parser.ParseFile(token.NewFileSet(), "observatory.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if s, ok := call.Fun.(*ast.SelectorExpr); ok {
				if id, ok := s.X.(*ast.Ident); ok && (id.Name == "time" || id.Name == "tea") {
					switch s.Sel.Name {
					case "Now", "Tick", "NewTicker", "NewTimer", "AfterFunc", "Sleep":
						t.Errorf("renderer added a clock/timer call: %s.%s", id.Name, s.Sel.Name)
					}
				}
			}
		}
		return true
	})
}
