package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// unfinishedObservatoryAttempt is deliberately separate from the narrower
// workerRunning action gate: launch preparation is also unfinished evidence.
func (m *Model) unfinishedObservatoryAttempt() bool {
	if m.snap.Attempt == nil {
		return false
	}
	switch m.snap.Attempt.State {
	case "launch_intent_recorded", "launching", "running", "stop_requested":
		return true
	default:
		return false
	}
}

// inspectUnsupervised consumes only the snapshot's explicit observation. The
// production observer leaves stale/missing beats unknown; rendering never probes.
func (m *Model) inspectUnsupervised() bool {
	return m.cfg.Mode == ModeInspect && (m.snap.Run.State == "executing" || m.snap.Run.State == "stopping") && m.unfinishedObservatoryAttempt() && m.snap.Supervision == "unsupervised"
}
func (m *Model) observatoryElapsed() string {
	start, end := m.snap.Run.CreatedAt, m.snap.Run.UpdatedAt
	if !m.inspectUnsupervised() {
		switch m.snap.Run.State {
		case "created", "admission", "executing", "verifying", "applying", "stopping", "recovering":
			end = m.snap.At
		}
	}
	if start.IsZero() || end.IsZero() {
		return "unknown"
	}
	seconds := int64(max(time.Duration(0), end.Sub(start)) / time.Second)
	if seconds < 3600 {
		return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
}
func observatoryValue(text string) string {
	text = strings.TrimSpace(cell(text))
	if text == "" {
		return "unknown"
	}
	return text
}
func (m *Model) observatoryFit(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	prefix := ""
	if budget := width - 1; budget > 0 {
		prefix = lipgloss.NewStyle().Inline(true).MaxWidth(budget).Render(text)
	}
	return prefix + ellGlyph(m.cfg.Caps)
}
func (m *Model) observatoryHeader(width int) line {
	if width <= 0 {
		return line{}
	}
	t := m.cfg.Tokens
	prefix := []span{{text: missionGlyph(m.cfg.Caps), colour: t.Focus}, {text: " ", colour: t.Text}}
	if width >= 100 {
		prefix = append(prefix, span{text: "MYTHHELM ", colour: t.Text, bold: true})
	}
	state := m.stateSpans()
	if m.inspectUnsupervised() {
		state = []span{{text: unknownGlyph(m.cfg.Caps), colour: t.TextMuted}, {text: " unsupervised", colour: t.Text}}
	}
	clock := m.observatoryElapsed()
	state = append(state, span{text: " ", colour: t.Text}, span{text: clock, colour: t.Text, underline: clock == "unknown"})
	suffix := state
	var identity []span
	if width >= 140 {
		budget := max(1, (min(width/2, width-lipgloss.Width(lineFromSpans(prefix...).text)-lipgloss.Width(lineFromSpans(suffix...).text)-5)-6)/3)
		for i, value := range []string{laneAdapter(m.snap), m.snap.Model, m.snap.Run.ExecutionProfile} {
			if i > 0 {
				identity = append(identity, span{text: " " + sepDotGlyph(m.cfg.Caps) + " ", colour: t.TextMuted})
			}
			value = observatoryValue(value)
			identity = append(identity, span{text: m.observatoryFit(value, budget), colour: t.Text, underline: value == "unknown"})
		}
	}
	fixed := lipgloss.Width(lineFromSpans(prefix...).text) + lipgloss.Width(lineFromSpans(suffix...).text) + 1
	if len(identity) > 0 {
		fixed += lipgloss.Width(lineFromSpans(identity...).text) + 2
	}
	if fixed > width {
		return lineFromSpans(span{text: m.observatoryFit(lineFromSpans(suffix...).text, width), colour: t.Text})
	}
	title := observatoryValue(m.snap.Goal)
	prefix = append(prefix, span{text: m.observatoryFit(title, width-fixed), colour: t.Text, bold: true, underline: title == "unknown"}, span{text: " ", colour: t.Text})
	spans := prefix
	spans = append(spans, suffix...)
	if len(identity) > 0 {
		gap := max(2, width-lipgloss.Width(lineFromSpans(spans...).text)-lipgloss.Width(lineFromSpans(identity...).text))
		spans = append(spans, span{text: strings.Repeat(" ", gap), colour: t.Text})
		spans = append(spans, identity...)
	}
	return lineFromSpans(spans...)
}
func (m *Model) observatoryLane() string {
	if m.snap.Attempt == nil {
		return "lane unknown"
	}
	return "lane " + laneLetter(m.snap.Attempt.AttemptNumber) + " " + observatoryValue(laneAdapter(m.snap))
}
func (m *Model) observatoryNow() string {
	if m.inspectUnsupervised() {
		text := "unsupervised; " + m.observatoryLane() + "; progress not live; target unknown"
		if m.snap.Run.State == "stopping" {
			text = "stop requested; " + text
		}
		return text
	}
	switch m.snap.Run.State {
	case "created", "admission":
		return "admitting"
	case "stopping":
		return m.observatorySupervision("stop requested; " + m.observatoryLane())
	case "interrupted":
		return "interrupted; " + reasonOrUnknown(m.snap.Run.Reason)
	case "ready_for_review", "completed", "blocked", "failed", "cancelled":
		state := strings.TrimSpace(m.stateSpans()[1].text)
		if m.snap.Verification == nil || len(m.snap.Verification.Checks) == 0 {
			return state + "; no checks recorded"
		}
		passed := 0
		for _, check := range m.snap.Verification.Checks {
			if check.Status == "passed" {
				passed++
			}
		}
		return fmt.Sprintf("%s; passed %d of %d checks", state, passed, len(m.snap.Verification.Checks))
	case "executing":
		state := "unknown"
		if m.snap.Attempt != nil {
			state = cellOrUnknown(m.snap.Attempt.State)
		}
		activity := "counters unknown"
		if m.snap.LatestProgress != nil {
			activity = m.activityText()
		}
		return m.observatorySupervision(m.observatoryLane() + "; " + state + "; " + activity + "; target unknown")
	default:
		return strings.TrimSpace(m.stateSpans()[1].text)
	}
}

func (m *Model) observatorySupervision(text string) string {
	if m.cfg.Mode == ModeInspect && m.unfinishedObservatoryAttempt() && m.snap.Supervision != "supervised" {
		return text + "; supervision unknown"
	}
	return text
}

// observatoryNext uses the existing gates; recorded state never grants an action.
func (m *Model) observatoryNext() string {
	if m.inspectUnsupervised() {
		text := "mythhelm recover " + cellOrUnknown(m.snap.Run.RunID)
		if m.snap.Run.State == "stopping" {
			text = "stop requested, waiting for the worker; " + text
		}
		return text
	}
	if m.stopPending() || m.snap.Run.State == "stopping" {
		return "stop requested, waiting for the worker"
	}
	if m.working != "" {
		return "operation in progress; waiting for result"
	}
	if !m.ready {
		return "waiting for snapshot"
	}
	if m.snap.Run.State == "blocked" || m.snap.Run.State == "failed" {
		return "read the reason; start a new run"
	}
	if m.snap.Run.State == "ready_for_review" {
		text := "review the candidate"
		if ok, _ := m.actionGate(actionApply); ok {
			text += ", then a apply"
		}
		return text
	}
	if ok, _ := m.actionGate(actionRecover); ok {
		return "r recover the run"
	}
	var actions []string
	if ok, _ := m.actionGate(actionStop); ok {
		actions = append(actions, "s stop")
	}
	if ok, _ := m.actionGate(actionDetach); ok {
		actions = append(actions, "d detach")
	}
	if len(actions) > 0 {
		return strings.Join(actions, "; ")
	}
	switch m.snap.Run.State {
	case "created", "admission":
		return "admitting; no action available"
	case "executing":
		return "agent working; no action available"
	case "verifying":
		return "checks running; no action available"
	case "applying":
		return "applying; no action available"
	case "recovering":
		return "recovery running; no action available"
	case "interrupted":
		return "read the recorded reason; recovery unavailable"
	case "completed", "cancelled":
		return "no action; run is terminal"
	default:
		return "unknown"
	}
}
func (m *Model) observatoryLabel(label, text string, width int) line {
	if width <= 0 {
		return line{}
	}
	prefix := label + " "
	if width < lipgloss.Width(prefix) {
		return lineFromSpans(span{text: m.observatoryFit(label, width), colour: m.cfg.Tokens.Text, bold: true})
	}
	spans := []span{{text: prefix, colour: m.cfg.Tokens.Text, bold: true}}
	parts := strings.Split(m.observatoryFit(text, width-lipgloss.Width(prefix)), "unknown")
	for i, part := range parts {
		if i > 0 {
			spans = append(spans, span{text: "unknown", colour: m.cfg.Tokens.Text, underline: true})
		}
		spans = append(spans, span{text: part, colour: m.cfg.Tokens.Text})
	}
	return lineFromSpans(spans...)
}
func (m *Model) nowNext(width int) []line {
	if width < 80 || m.height < 10 {
		return []line{m.observatoryLabel("now", m.observatoryNow(), width), m.observatoryLabel("next", m.observatoryNext(), width)}
	}
	leftWidth := (width - 2) / 2
	left := m.observatoryLabel("NOW", m.observatoryNow(), leftWidth)
	right := m.observatoryLabel("NEXT", m.observatoryNext(), width-leftWidth-2)
	left.spans = append(left.spans, span{text: strings.Repeat(" ", leftWidth-lipgloss.Width(left.text)+2), colour: m.cfg.Tokens.Text})
	spans := left.spans
	return []line{lineFromSpans(append(spans, right.spans...)...)}
}

type observatoryKey struct{ key, label string }

func (m *Model) observatoryAttention() bool {
	return m.inspectUnsupervised() || m.snap.Run.State == "blocked" || m.snap.Run.State == "failed" || m.snap.Run.State == "interrupted"
}

// observatoryKeys returns relocation before the final key row. Task15 composes
// that row with NEXT; no key routing or responsive layout is changed here.
func (m *Model) observatoryKeys(width int, compact bool) []line {
	if width <= 0 {
		return []line{{}}
	}
	t := m.cfg.Tokens
	var slots []observatoryKey
	add := func(key, label string, a Action) {
		if ok, _ := m.actionGate(a); ok && m.working == "" {
			slots = append(slots, observatoryKey{key, label})
		}
	}
	if m.snap.Run.State == "ready_for_review" {
		add("a", "apply", actionApply)
		add("e", "export", actionExport)
	}
	slots = append(slots, observatoryKey{"Enter", "inspect"})
	if m.snap.Run.State != "ready_for_review" {
		add("s", "stop", actionStop)
		if width >= 90 {
			add("d", "detach", actionDetach)
		}
		add("r", "recover", actionRecover)
	}
	slots = append(slots, observatoryKey{"?", "help"})
	if width >= 100 && m.snap.Run.State != "ready_for_review" {
		slots = append(slots, observatoryKey{":", "commands"}, observatoryKey{"/", "filter"})
	}
	attention := m.observatoryAttention()
	compose := func(keys []observatoryKey, showAttention bool) []span {
		var spans []span
		gap := func() {
			if len(spans) > 0 {
				spans = append(spans, span{text: "  ", colour: t.TextMuted})
			}
		}
		for _, k := range keys {
			gap()
			spans = append(spans, span{text: k.key, colour: t.Text, bold: true})
			if width >= 60 {
				spans = append(spans, span{text: " " + k.label, colour: t.TextMuted})
			}
		}
		if showAttention {
			gap()
			spans = append(spans, span{text: missionGlyph(m.cfg.Caps) + " 1 needs you", colour: t.Attention})
		} else if !attention && width >= 100 {
			gap()
			spans = append(spans, span{text: "nothing needs you", colour: t.TextMuted})
		}
		gap()
		spans = append(spans, span{text: "q", colour: t.Text, bold: true}, span{text: " exit" + ellGlyph(m.cfg.Caps), colour: t.TextMuted})
		if compact {
			gap()
			spans = append(spans, span{text: "--accessible: linear", colour: t.TextMuted})
		}
		return spans
	}
	for len(slots) > 0 && lipgloss.Width(lineFromSpans(compose(slots, attention)...).text) > width {
		slots = slots[:len(slots)-1]
	}
	var rows []line
	if attention && lipgloss.Width(lineFromSpans(compose(slots, true)...).text) > width {
		rows = append(rows, lineFromSpans(span{text: "NEXT ", colour: t.Text, bold: true}, span{text: m.observatoryFit(missionGlyph(m.cfg.Caps)+" 1 needs you", max(0, width-5)), colour: t.Attention}))
		attention = false
	}
	spans := compose(slots, attention)
	plain := lineFromSpans(spans...).text
	if lipgloss.Width(plain) > width {
		return append(rows, line{text: m.observatoryFit(plain, width), colour: t.Text})
	}
	// Reserve the rightmost q/linear group rather than moving key slots.

	for i := range spans {
		if spans[i].text == "q" {
			padding := width - lipgloss.Width(plain)
			if padding > 0 {
				spans = append(spans[:i], append([]span{{text: strings.Repeat(" ", padding), colour: t.TextMuted}}, spans[i:]...)...)
			}
			break
		}
	}
	return append(rows, lineFromSpans(spans...))
}
