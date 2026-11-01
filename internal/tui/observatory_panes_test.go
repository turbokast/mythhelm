package tui

import (
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/turbokast/mythhelm/internal/journal"
)

func TestAgentSummaryAndRaw(t *testing.T) {
	t.Parallel()
	m := observatoryFixture(t)
	m.snap.Attempt.State = "running"
	m.snap.AgentEvents = []journal.Event{{Type: "attempt.native_error", Payload: json.RawMessage(`{"message":"recorded native failure"}`), ObservedAt: m.snap.At}}
	if got := observatoryPlain(m.observatoryAgent(180)); !strings.Contains(got, "recorded native failure") || !strings.Contains(got, "native-model") {
		t.Fatalf("agent evidence missing: %q", got)
	}
	before := observatoryPlain(m.observatoryAgent(180))
	focus, index := m.focusPane, m.focusIndex
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if got := observatoryPlain(m.observatoryAgent(180)); !strings.Contains(got, `"message"`) {
		t.Fatalf("raw toggle missing: %q", got)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if got := observatoryPlain(m.observatoryAgent(180)); got != before || m.focusPane != focus || m.focusIndex != index {
		t.Fatalf("toggle lost view identity: %q", got)
	}
	m.filterOn = true
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if m.agentRaw || m.filterText != "v" {
		t.Fatal("v escaped editable filter")
	}
	m.filterOn = false
	m.openPalette()
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if m.agentRaw {
		t.Fatal("v escaped palette")
	}
	m.palette.open = false
	m.helpOpen = true
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if m.agentRaw {
		t.Fatal("v escaped help")
	}
	m.helpOpen = false
	m.dialog = dialogExitOptions
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if m.agentRaw {
		t.Fatal("v escaped dialog")
	}
	m.dialog = ""
	m.snap.AgentEvents = make([]journal.Event, 205)
	for i := range m.snap.AgentEvents {
		m.snap.AgentEvents[i] = journal.Event{Type: "attempt.native_error", Payload: json.RawMessage(fmt.Sprintf(`{"message":"row-%03d\u001b[31m"}`, i)), ObservedAt: m.snap.At}
	}
	m.snap.AgentEventsOmitted = 9
	for _, raw := range []bool{false, true} {
		m.agentRaw = raw
		rows := m.observatoryAgent(200)
		got := observatoryPlain(rows)
		if strings.Count(got, "attempt.native_error") != 200 || strings.Contains(got, "row-004") || !strings.Contains(got, "row-005") || !strings.Contains(got, "14 earlier agent events omitted") {
			t.Fatalf("tail bound/provenance lost: %q", got)
		}
		for _, row := range rows {
			if strings.ContainsAny(row.text, "\x1b\r\n") {
				t.Fatalf("unsafe event: %q", row.text)
			}
			if lipgloss.Width(row.text) > 200 {
				t.Fatal("unbounded row")
			}
			var mirror string
			for _, sp := range row.spans {
				mirror += sp.text
			}
			if row.text != mirror {
				t.Fatal("styled/plain divergence")
			}
		}
	}
	m.agentRaw = false
	m.snap.AgentEvents = []journal.Event{{Type: "attempt.state_changed", Payload: json.RawMessage(`{"state":"running"}`)}, {Type: "attempt.permission_denied", Payload: json.RawMessage(`{"permission":"denied"}`)}, {Type: "future.event", Payload: json.RawMessage(`broken`)}}
	got := observatoryPlain(m.observatoryAgent(200))
	for _, want := range []string{"state: running", "permission: denied", "future.event broken", "native-reported", "target: unknown"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary lacks %q: %q", want, got)
		}
	}
}
func TestUsageProvenanceRows(t *testing.T) {
	t.Parallel()
	m := observatoryFixture(t)
	m.snap.Usage = []journal.UsageRow{{Scope: "tokens", Unit: "input", Quantity: "0", Label: "reported", Source: "native"}, {Scope: "tokens", Unit: "input", Quantity: "unknown", Label: "unknown", Source: "missing"}}
	got := observatoryPlain(m.observatoryEvidence(180))
	for _, want := range []string{"0 (reported)", "unknown (unknown)", "native", "missing"} {
		if !strings.Contains(got, want) {
			t.Errorf("usage lacks %q: %q", want, got)
		}
	}
	for _, label := range []string{"observed", "estimated", "user-declared"} {
		m.snap.Usage = append(m.snap.Usage, journal.UsageRow{Scope: "tokens", Unit: "input", Quantity: "7", Label: label, Source: "receipt"})
	}
	m.snap.Usage = append(m.snap.Usage, journal.UsageRow{Scope: "retail-equivalent", Unit: "USD", Quantity: "1.23", Label: "estimated", Source: "receipt"})
	got = observatoryPlain(m.observatoryEvidence(400))
	for _, want := range []string{"7 (observed)", "7 (estimated)", "7 (user-declared)", "estimate, not a charge"} {
		if !strings.Contains(got, want) {
			t.Errorf("usage lost %q", want)
		}
	}
	if strings.Count(got, "tokens / input:") != 5 {
		t.Fatal("repeated observations aggregated")
	}
	m.snap.Usage = nil
	if got := observatoryPlain(m.observatoryEvidence(200)); !strings.Contains(got, "unknown (no usage recorded)") || strings.Contains(got, "0 (reported)") {
		t.Fatalf("missing usage invented: %q", got)
	}
}
func TestEvidenceStageProof(t *testing.T) {
	t.Parallel()
	m := observatoryFixture(t)
	if got := observatoryPlain(m.observatoryEvidence(180)); !strings.Contains(got, "✓ checked") {
		t.Fatalf("matching proof missing: %q", got)
	}
	m.snap.Verification.Result = "failed"
	if got := observatoryPlain(m.observatoryEvidence(180)); strings.Contains(got, "✓ checked") || !strings.Contains(got, "checked: failed") {
		t.Fatalf("failed check presented as proven: %q", got)
	}
	for _, result := range []string{"not_run", ""} {
		m.snap.Verification.Result = result
		if got := observatoryPlain(m.observatoryEvidence(200)); strings.Contains(got, "✓ checked") {
			t.Fatalf("unproven stage: %q", got)
		}
	}
	m.snap.Verification.Result = "passed"
	m.snap.Verification.CandidateCommit = "other"
	got := observatoryPlain(m.observatoryEvidence(200))
	if strings.Contains(got, "✓ checked") || !strings.Contains(got, "mismatch:") || !strings.Contains(got, "candidate:") {
		t.Fatalf("candidate mismatch lost: %q", got)
	}
	m.snap.Verification.CandidateCommit = ""
	m.snap.Candidate.Commit = ""
	if strings.Contains(observatoryPlain(m.observatoryEvidence(200)), "✓ checked") {
		t.Fatal("empty candidate proved")
	}
	m.snap.Admission.RunSeq = 0
	m.snap.Candidate = nil
	m.snap.Run.State = "created"
	got = observatoryPlain(m.observatoryEvidence(200))
	for _, stage := range []string{"✓ admitted", "● working", "✓ frozen", "✓ ready"} {
		if strings.Contains(got, stage) {
			t.Fatalf("missing source proved %s", stage)
		}
	}
	m.snap.Admission.RunSeq = 2
	m.snap.Run.State = "executing"
	got = observatoryPlain(m.observatoryEvidence(200))
	if !strings.Contains(got, "✓ admitted") || !strings.Contains(got, "● working") {
		t.Fatalf("recorded stages missing: %q", got)
	}
}
func TestVerdictGroups(t *testing.T) {
	t.Parallel()
	m := observatoryFixture(t)
	m.snap.Verification.Result = "failed"
	m.snap.Verification.Reason = "recorded check failure"
	m.snap.Verification.Checks = []journal.CheckRow{{Name: "build", Status: "passed"}, {Name: "test", Status: "failed"}}
	got := observatoryPlain(m.observatoryVerdict(180))
	for _, want := range []string{"PROVEN", "NOT PROVEN", "UNKNOWN", "✓ build", "test: failed", "recorded check failure"} {
		if !strings.Contains(got, want) {
			t.Errorf("mixed verdict lacks %q: %q", want, got)
		}
	}
}

func TestVerdictMissingAndKnownFacts(t *testing.T) {
	t.Parallel()
	m := observatoryFixture(t)
	m.snap.Admission.PaidContinuation = "unknown"
	m.snap.Run.BillingPosture = "unknown"
	m.snap.Verification = nil
	m.snap.Run.Reason = "cancelled before launch"
	got := observatoryPlain(m.observatoryVerdict(300))
	if strings.Contains(got, "PROVEN") || !strings.Contains(got, "checks: not run; cancelled before launch") {
		t.Fatalf("absent verification invented: %q", got)
	}
	for _, want := range []string{"paid continuation: unknown", "native exit: unknown", "entitlement: unknown", "billing: unknown"} {
		if !strings.Contains(got, want) {
			t.Fatalf("unknown fact omitted %q: %q", want, got)
		}
	}
	zero := 0
	m.snap.NativeExit = &viewmodel.NativeExit{ExitCode: &zero}
	m.snap.Admission.PaidContinuation = "off"
	m.snap.Admission.EntitlementStatement = "entitlement: user-declared"
	m.snap.Run.BillingPosture = "local-scripted"
	m.snap.Verification = observatoryFixture(t).snap.Verification
	got = observatoryPlain(m.observatoryVerdict(300))
	if strings.Contains(got, "UNKNOWN") || strings.Contains(got, "NOT PROVEN") || !strings.Contains(got, "PROVEN") {
		t.Fatalf("known zero/off or empty groups: %q", got)
	}
	m.snap.NativeExit = &viewmodel.NativeExit{}
	if got := observatoryPlain(m.observatoryVerdict(300)); !strings.Contains(got, "native exit: unknown") {
		t.Fatal("partial exit became zero")
	}
	for _, commit := range []string{"", "different"} {
		m.snap.Candidate.Commit = commit
		if got := observatoryPlain(m.observatoryVerdict(300)); strings.Contains(got, "✓ build") || !strings.Contains(got, "current candidate proof unavailable") {
			t.Fatalf("unbound check proved: %q", got)
		}
	}
}
