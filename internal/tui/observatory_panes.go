package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// paneFact keeps journal text literal, bounded and free of terminal control
// sequences in both the plain mirror and the styled representation.
func (m *Model) paneFact(text string, width int) line {
	text = m.observatoryFit(cell(text), width)
	parts := strings.Split(text, "unknown")
	spans := make([]span, 0, len(parts)*2)
	for i, part := range parts {
		if i > 0 {
			spans = append(spans, span{text: "unknown", colour: m.cfg.Tokens.Text, underline: true})
		}
		spans = append(spans, span{text: part, colour: m.cfg.Tokens.Text})
	}
	return lineFromSpans(spans...)
}

// These producers contain no pane IDs or layout decisions. Task 15 composes
// them across the responsive ladder; the existing pane wrappers consume them
// during the staged transition.
func (m *Model) observatoryAgent(width int) []line {
	var out []line
	add := func(s string) { out = append(out, m.paneFact(s, width)) }
	sep := " " + sepDotGlyph(m.cfg.Caps) + " "
	add("agent: " + observatoryValue(laneAdapter(m.snap)) + sep + observatoryValue(m.snap.Model) + sep + observatoryValue(m.snap.Run.ExecutionProfile))
	if a := m.snap.Attempt; a != nil {
		state := observatoryValue(a.State)
		if m.dispatchWaiting() {
			state = dispatchWaitingText(m.cfg.Caps)
		}
		add(fmt.Sprintf("attempt %d: %s", a.AttemptNumber, state))
	} else {
		add("no attempts yet")
	}
	if p := m.snap.LatestProgress; p != nil {
		add(fmt.Sprintf("native-reported: %d assistant turns; retries: %d; target: unknown", p.AssistantTurns, p.Retries))
		names := make([]string, 0, len(p.ToolUses))
		for name := range p.ToolUses {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			add(fmt.Sprintf("tool %s: %d (native-reported)", observatoryValue(name), p.ToolUses[name]))
		}
	} else {
		add("native progress: unknown; target: unknown")
	}
	if m.agentRaw {
		add("[v] summary" + sep + "raw journal stream")
	} else {
		add("[v] full stream" + sep + "summary")
	}
	events := m.snap.AgentEvents
	dropped := max(0, len(events)-200)
	events = events[dropped:]
	if omitted := m.snap.AgentEventsOmitted + dropped; omitted > 0 {
		add(fmt.Sprintf("%d earlier agent events omitted", omitted))
	}
	for _, ev := range events {
		stamp := "unknown"
		if !ev.ObservedAt.IsZero() {
			stamp = ev.ObservedAt.Format("15:04:05")
		}
		body := string(ev.Payload)
		var keys []string
		switch ev.Type {
		case "attempt.state_changed":
			keys = []string{"state", "reason"}
		case "attempt.permission_denied":
			keys = []string{"tool_name", "reason"}
		case "attempt.native_error":
			keys = []string{"class", "message", "error", "reason"}
		}
		if !m.agentRaw && len(keys) > 0 {
			var fields map[string]json.RawMessage
			if json.Unmarshal(ev.Payload, &fields) == nil {
				var facts []string
				for _, key := range keys {
					if raw, ok := fields[key]; ok {
						var value string
						if json.Unmarshal(raw, &value) == nil {
							label := key
							if key == "tool_name" {
								label = "permission denied"
							}
							if key == "class" {
								label = "native error"
							}
							facts = append(facts, label+": "+value)
						}
					}
				}
				if len(facts) > 0 {
					body = strings.Join(facts, "; ")
				}
			}
		}
		add(stamp + " " + observatoryValue(ev.Type) + " " + body)
	}
	return out
}

func (m *Model) observatoryEvidence(width int) []line {
	var out []line
	add := func(s string) { out = append(out, m.paneFact(s, width)) }
	stage := func(name string, recorded bool) {
		marker := glyph(m.cfg.Caps, "○", "o")
		if recorded {
			marker = checkGlyph(m.cfg.Caps)
		}
		add(marker + " " + name)
	}
	stage("admitted", m.snap.Admission != nil && m.snap.Admission.RunSeq > 0)
	if m.snap.Run.State == "executing" {
		add(glyph(m.cfg.Caps, "●", "*") + " working")
	} else {
		add(glyph(m.cfg.Caps, "○", "o") + " working")
	}
	stage("frozen", m.snap.Candidate != nil)
	if verified(m.snap) {
		stage("checked", true)
	} else if v := m.snap.Verification; v != nil {
		result := observatoryValue(v.Result)
		if result == "not_run" {
			result = "not run"
		}
		add(glyph(m.cfg.Caps, "○", "o") + " checked: " + result + "; " + observatoryValue(v.Reason))
	} else {
		add(glyph(m.cfg.Caps, "○", "o") + " checked: not run")
	}
	stage("ready", m.snap.Run.State == "ready_for_review")
	if len(m.snap.Usage) == 0 {
		add("usage: unknown (no usage recorded)")
	}
	for _, u := range m.snap.Usage {
		text := observatoryValue(u.Scope) + " / " + observatoryValue(u.Unit) + ": " + observatoryValue(u.Quantity) + " (" + observatoryValue(u.Label) + "); source: " + observatoryValue(u.Source)
		if u.Scope == "retail-equivalent" {
			text += "; estimate, not a charge"
		}
		add(text)
	}
	add("Billing: " + observatoryValue(m.snap.Run.BillingPosture))
	add(m.entitlementText())
	add("native exit: " + m.nativeExitText())
	if c := m.snap.Candidate; c != nil {
		add("candidate: " + observatoryValue(c.Commit) + "; base: " + observatoryValue(c.BaseRev))
	}
	for _, row := range m.verificationLines() {
		bounded := m.paneFact(row.text, width)
		if row.colour != "" {
			bounded.colour = row.colour
			for i := range bounded.spans {
				bounded.spans[i].colour = row.colour
			}
		}
		out = append(out, bounded)
	}
	return out
}

// A passed check can prove its individual claim in a mixed failed run. It
// still needs the same nonempty candidate binding as the overall verdict.
func (m *Model) matchingCandidateChecks() bool {
	c, v := m.snap.Candidate, m.snap.Verification
	return c != nil && c.Commit != "" && v != nil && v.CandidateCommit == c.Commit
}
func (m *Model) observatoryVerdict(width int) []line {
	var proven, unproven, unknown []string
	if v := m.snap.Verification; v != nil {
		for _, check := range v.Checks {
			if check.Status == "passed" && m.matchingCandidateChecks() {
				proven = append(proven, checkGlyph(m.cfg.Caps)+" "+observatoryValue(check.Name))
			} else {
				status := observatoryValue(check.Status)
				if status == "not_run" {
					status = "not run"
				}
				reason := observatoryValue(v.Reason)
				if check.Status == "passed" && !m.matchingCandidateChecks() {
					reason = "current candidate proof unavailable; " + reason
				}
				unproven = append(unproven, observatoryValue(check.Name)+": "+status+"; "+reason)
			}
		}
	} else {
		unknown = append(unknown, "checks: not run; "+observatoryValue(m.snap.Run.Reason))
	}
	paid := "unknown"
	if a := m.snap.Admission; a != nil {
		paid = observatoryValue(a.PaidContinuation)
	}
	if paid == "unknown" {
		unknown = append(unknown, "paid continuation: unknown")
	}
	if m.nativeExitText() == "unknown" {
		unknown = append(unknown, "native exit: unknown")
	}
	if entitlement := m.entitlementText(); entitlement == "entitlement: unknown" || entitlement == "unknown" {
		unknown = append(unknown, "entitlement: unknown")
	}
	if observatoryValue(m.snap.Run.BillingPosture) == "unknown" {
		unknown = append(unknown, "billing: unknown")
	}
	var out []line
	for _, group := range []struct {
		title string
		rows  []string
	}{{"PROVEN", proven}, {"NOT PROVEN", unproven}, {"UNKNOWN", unknown}} {
		if len(group.rows) == 0 {
			continue
		}
		title := m.paneFact(group.title, width)
		for i := range title.spans {
			title.spans[i].bold = true
		}
		out = append(out, title)
		for _, row := range group.rows {
			out = append(out, m.paneFact(row, width))
		}
	}
	return out
}
