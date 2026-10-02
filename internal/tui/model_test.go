package tui

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/tui/caps"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

// asciiCaps (fully suppressed) and fullCaps (structural colour plus unicode
// markers) come from diff_test.go, as does darkTokens.
func testConfig(c caps.Caps, t *testing.T) Config {
	t.Helper()
	return Config{RunID: "run_t6", StateDir: t.TempDir(), Caps: c, Tokens: darkTokens(t)}
}

// richSnapshot is a hand-built ready_for_review snapshot exercising every
// mission pane: goal, progress, lane, candidate diff and checks.
func richSnapshot() viewmodel.Snapshot {
	commit := strings.Repeat("c", 40)
	return viewmodel.Snapshot{
		Run: journal.RunRow{
			RunID: "run_t6", State: "ready_for_review",
			AdapterID: "builtin/fake", SourceRepo: "/repo",
			BillingPosture: "local-scripted", ExecutionProfile: "trusted-host",
		},
		Goal: "Add pagination to the audit log",
		Attempt: &journal.AttemptRow{
			AttemptID: "att_1", RunID: "run_t6", TaskID: "task_1",
			AttemptNumber: 1, State: "succeeded_native", WorkspacePath: "/tmp/tui ws",
		},
		LatestProgress: &viewmodel.Progress{AssistantTurns: 7, ToolUses: map[string]int{"read": 3}, Retries: 1, RunSeq: 9},
		Candidate: &journal.CandidateRow{
			AttemptID: "att_1", BaseRev: strings.Repeat("b", 40), Commit: commit,
			Tree: strings.Repeat("t", 40), PatchSHA256: strings.Repeat("d", 64),
			ChangedPaths: json.RawMessage(`["main.go"]`), Flags: json.RawMessage(`[]`),
		},
		Verification: &journal.VerificationRow{
			ID: "ver_1", RunID: "run_t6", CandidateCommit: commit, ConfigSHA256: "cfg",
			Result: "passed",
			Checks: []journal.CheckRow{{Name: "build", Argv: []string{"go", "build"}, Status: "passed"}},
		},
		Admission: &viewmodel.Admission{
			AdapterID: "builtin/fake", AdapterVersion: "1.0", AdapterSurface: "local",
			Qualified: true, PaidContinuation: "off", RunSeq: 2,
		},
		LastRunSeq: 12,
		At:         time.Now().UTC(),
	}
}

const smallDiffInput = `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 context row
-old row
+new row
+added garnish
 context tail
`

func mustParseDiff(t *testing.T, raw string) Diff {
	t.Helper()
	d, err := ParseDiff([]byte(raw))
	if err != nil {
		t.Fatalf("ParseDiff: %v", err)
	}
	return d
}

// stubDiff serves one parsed diff through the production loader seam, so
// tests install diffs the way setSnapshot loads them.
func stubDiff(d Diff) func(context.Context, string, string, string) (Diff, error) {
	return func(context.Context, string, string, string) (Diff, error) { return d, nil }
}

// readyModel builds a model with the rich snapshot and small diff installed,
// sized via WindowSizeMsg like the program does.
func readyModel(t *testing.T, c caps.Caps, width, height int) *Model {
	t.Helper()
	m := newModel(testConfig(c, t))
	m.loadDiff = stubDiff(mustParseDiff(t, smallDiffInput))
	m.setSnapshot(t.Context(), richSnapshot())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	mm, ok := updated.(*Model)
	if !ok {
		t.Fatalf("Update returned %T, want *Model", updated)
	}
	return mm
}

// assertWidthSafe fails when any view row exceeds width cells or the view
// exceeds height rows.
func assertWidthSafe(t *testing.T, view string, width, height int) {
	t.Helper()
	rows := strings.Split(view, "\n")
	if len(rows) > height {
		t.Errorf("view has %d rows, height budget is %d", len(rows), height)
	}
	for i, r := range rows {
		if w := lipgloss.Width(r); w > width {
			t.Errorf("row %d is %d cells wide, budget is %d: %q", i, w, width, r)
		}
	}
}

// journalBuilder journals a run's events and projections in a temp state dir,
// mirroring the viewmodel fixtures.
type journalBuilder struct {
	t   *testing.T
	dir string
	j   *journal.Journal
	seq int64
}

func newJournalBuilder(t *testing.T) *journalBuilder {
	t.Helper()
	dir := t.TempDir()
	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	return &journalBuilder{t: t, dir: dir, j: j}
}

func (b *journalBuilder) append(runID, typ, payload string, project func(*sql.Tx) error) {
	b.t.Helper()
	b.seq++
	ev := journal.Event{
		SchemaVersion:    journal.EnvelopeVersion,
		EventID:          ids.New("evt"),
		RunID:            runID,
		ProducerID:       "producer_test",
		ProducerSequence: b.seq,
		Generation:       1,
		ObservedAt:       time.Now().UTC(),
		Type:             typ,
		Payload:          json.RawMessage(payload),
	}
	if err := b.j.Append(b.t.Context(), ev, project); err != nil {
		b.t.Fatalf("Append(%s): %v", typ, err)
	}
}

func (b *journalBuilder) writeTask(runID string, content []byte) string {
	b.t.Helper()
	if err := os.MkdirAll(filepath.Join(b.dir, "runs", runID), 0o700); err != nil {
		b.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.dir, "runs", runID, "task.md"), content, 0o600); err != nil {
		b.t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func (b *journalBuilder) addRun(runID, state, taskSHA string) {
	b.t.Helper()
	now := time.Now().UTC()
	row := journal.RunRow{
		RunID: runID, State: state, AdapterID: "builtin/fake", SourceRepo: "/repo",
		TaskSHA256: taskSHA, BillingPosture: "local-scripted", ExecutionProfile: "trusted-host",
		CreatedAt: now, UpdatedAt: now,
	}
	b.append(runID, "run.created", `{"state":"`+state+`"}`, func(tx *sql.Tx) error {
		return journal.InsertRun(b.t.Context(), tx, row)
	})
}

func (b *journalBuilder) addAttempt(runID, attemptState string) string {
	b.t.Helper()
	attemptID := ids.New("att")
	row := journal.AttemptRow{
		AttemptID: attemptID, RunID: runID, TaskID: ids.New("task"), AttemptNumber: 1,
		State: attemptState, LaunchTokenSHA256: "token", WorkspacePath: "/tmp/tui-ws",
	}
	b.append(runID, "attempt.launch_intent_recorded", `{}`, func(tx *sql.Tx) error {
		return journal.InsertAttempt(b.t.Context(), tx, row)
	})
	return attemptID
}

func (b *journalBuilder) addCandidate(runID, attemptID, base, commit string) {
	b.t.Helper()
	row := journal.CandidateRow{
		AttemptID: attemptID, BaseRev: base, Commit: commit,
		Tree: strings.Repeat("t", 40), PatchSHA256: strings.Repeat("d", 64),
		ChangedPaths: json.RawMessage(`["main.go"]`), Flags: json.RawMessage(`[]`),
	}
	b.append(runID, "candidate.frozen", `{}`, func(tx *sql.Tx) error {
		return journal.InsertCandidate(b.t.Context(), tx, row)
	})
}

func (b *journalBuilder) addVerification(runID, commit, result string) {
	b.t.Helper()
	now := time.Now().UTC()
	row := journal.VerificationRow{
		ID: ids.New("ver"), RunID: runID, CandidateCommit: commit, ConfigSHA256: "cfg",
		Result: result, StartedAt: now, FinishedAt: now,
		Checks: []journal.CheckRow{{Name: "build", Argv: []string{"go", "build"}, Status: result}},
	}
	b.append(runID, "verification.completed", `{}`, func(tx *sql.Tx) error {
		return journal.InsertVerification(b.t.Context(), tx, row)
	})
}

func (b *journalBuilder) close() {
	b.t.Helper()
	if err := b.j.Close(); err != nil {
		b.t.Fatalf("journal.Close: %v", err)
	}
}

// TestMissionShowsRequiredFacts renders the §15.1 facts in the wide layout
// (AC-1.1): goal, next action, activity, diff and verification.
func TestMissionShowsRequiredFacts(t *testing.T) {
	t.Parallel()
	view := readyModel(t, fullCaps, 160, 30).View()
	for _, want := range []string{
		"Add pagination to the audit log",     // goal
		"Next action: review the candidate",   // next action
		"7 assistant turns (native-reported)", // activity
		"added garnish",                       // diff
		"verified ccccccc",                    // verification
	} {
		if !strings.Contains(view, want) {
			t.Errorf("wide view missing %q\n%s", want, view)
		}
	}
}

// TestNextActionTable maps every run state to its design §7 string,
// including stopping → "waiting for worker confirmation" (I06).
func TestNextActionTable(t *testing.T) {
	t.Parallel()
	commitB := strings.Repeat("b", 40)
	commitA := strings.Repeat("a", 40)
	verifiedSnap := func() viewmodel.Snapshot {
		s := richSnapshot()
		s.Run.State = "ready_for_review"
		return s
	}
	unverifiedSnap := func() viewmodel.Snapshot {
		s := richSnapshot()
		s.Run.State = "ready_for_review"
		s.Verification = nil
		return s
	}
	mismatchSnap := func() viewmodel.Snapshot {
		s := richSnapshot()
		s.Run.State = "ready_for_review"
		s.Candidate.Commit = commitB
		s.Verification.CandidateCommit = commitA
		return s
	}
	stateSnap := func(state string) viewmodel.Snapshot {
		s := richSnapshot()
		s.Run.State = state
		s.Verification = nil
		s.Candidate = nil
		return s
	}
	cases := []struct {
		name string
		snap viewmodel.Snapshot
		want string
	}{
		{"created", stateSnap("created"), "admitting — no action"},
		{"admission", stateSnap("admission"), "admitting — no action"},
		{"executing", stateSnap("executing"), "none — agent working (stop available)"},
		{"verifying", stateSnap("verifying"), "none — agent working (stop available)"},
		{"stopping", stateSnap("stopping"), "waiting for worker confirmation — stop requested, not confirmed"},
		{"ready verified", verifiedSnap(), "review the candidate, then apply or close"},
		{"ready unverified", unverifiedSnap(), "verification unavailable — apply needs explicit unverified acceptance"},
		{"ready mismatch", mismatchSnap(), "verification unavailable — apply needs explicit unverified acceptance"},
		{"blocked", stateSnap("blocked"), "read the reason; recover or start a new run"},
		{"failed", stateSnap("failed"), "read the reason; recover or start a new run"},
		{"interrupted", stateSnap("interrupted"), "recover the run"},
		{"recovering", stateSnap("recovering"), "recovery running — no action"},
		{"applying", stateSnap("applying"), "none — run is terminal"},
		{"completed", stateSnap("completed"), "none — run is terminal"},
		{"cancelled", stateSnap("cancelled"), "none — run is terminal"},
		{"unknown state", stateSnap("bogus"), "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := nextAction(tc.snap, fullCaps); got != tc.want {
				t.Errorf("nextAction = %q, want %q", got, tc.want)
			}
			// The ASCII twin carries the same text with no non-ASCII bytes.
			ascii := nextAction(tc.snap, asciiCaps)
			for i := 0; i < len(ascii); i++ {
				if ascii[i] >= 0x80 {
					t.Errorf("ASCII nextAction has non-ASCII byte: %q", ascii)
					break
				}
			}
			if strings.ReplaceAll(tc.want, "—", "-") != ascii {
				t.Errorf("ASCII nextAction = %q, want dash-folded %q", ascii, tc.want)
			}
		})
	}
}

// TestLaneLabels shows adapter id, execution profile, lane letter and
// attempt number on every lane, never a logo (AC-1.3).
func TestLaneLabels(t *testing.T) {
	t.Parallel()
	uni := readyModel(t, fullCaps, 160, 30).View()
	for _, want := range []string{"builtin/fake / trusted-host", "A · attempt 1"} {
		if !strings.Contains(uni, want) {
			t.Errorf("unicode view missing lane label %q\n%s", want, uni)
		}
	}
	ascii := readyModel(t, asciiCaps, 160, 30).View()
	for _, want := range []string{"builtin/fake / trusted-host", "A - attempt 1"} {
		if !strings.Contains(ascii, want) {
			t.Errorf("ASCII view missing lane label %q\n%s", want, ascii)
		}
	}
	// An unknown adapter renders unknown, never blank.
	m := newModel(testConfig(asciiCaps, t))
	m.loadDiff = stubDiff(mustParseDiff(t, smallDiffInput))
	snap := richSnapshot()
	snap.Run.State = "executing"
	snap.Run.AdapterID = ""
	snap.Admission = nil
	m.setSnapshot(t.Context(), snap)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	view := updated.(*Model).View()
	if !strings.Contains(view, "unknown / trusted-host") {
		t.Errorf("lane with unknown adapter missing its label\n%s", view)
	}
}

// TestStatusStripKindLabels renders kind-labelled admission figures, with
// unknown (never blank or 0) for absent values (AC-7.3, AC-1.4).
func TestStatusStripKindLabels(t *testing.T) {
	t.Parallel()
	known := readyModel(t, asciiCaps, 160, 30).View()
	for _, want := range []string{"qualified: true", "paid continuation: off"} {
		if !strings.Contains(known, want) {
			t.Errorf("status strip missing %q\n%s", want, known)
		}
	}
	m := newModel(testConfig(asciiCaps, t))
	snap := richSnapshot()
	snap.Run.State = "executing"
	snap.Admission = nil
	m.setSnapshot(t.Context(), snap)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	view := updated.(*Model).View()
	for _, want := range []string{"qualified: unknown", "paid continuation: unknown"} {
		if !strings.Contains(view, want) {
			t.Errorf("status strip missing %q\n%s", want, view)
		}
	}
	for _, bad := range []string{"qualified: 0", "paid continuation: 0", "qualified: \n", "paid continuation: \n"} {
		if strings.Contains(view, bad) {
			t.Errorf("status strip renders absent value as %q\n%s", bad, view)
		}
	}
}

// TestNativeResultNeverVerified journals a clean native exit with no
// verification events, loads through viewmodel.Load, and asserts the view
// renders the native exit with no verified/pass label (I07). The fixture is
// Load-built, never hand-constructed, so the scanned path is what is proven.
func TestNativeResultNeverVerified(t *testing.T) {
	t.Parallel()
	b := newJournalBuilder(t)
	runID := "run_native"
	taskSHA := b.writeTask(runID, []byte("# Add pagination to the audit log\n"))
	b.addRun(runID, "executing", taskSHA)
	b.addAttempt(runID, "running")
	b.append(runID, "attempt.progress", `{"assistant_turns":7,"retries":0}`, nil)
	b.append(runID, "attempt.native_result", `{"exit_code":0,"result_observed":true}`, nil)
	b.close()

	snap, err := viewmodel.Load(t.Context(), b.dir, runID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if snap.NativeExit == nil || snap.NativeExit.ExitCode == nil || *snap.NativeExit.ExitCode != 0 {
		t.Fatalf("NativeExit = %+v, want exit code 0", snap.NativeExit)
	}
	m := newModel(testConfig(fullCaps, t))
	m.setSnapshot(t.Context(), snap)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	view := updated.(*Model).View()
	if !strings.Contains(view, "native exit: 0") {
		t.Errorf("view missing native exit label\n%s", view)
	}
	for _, bad := range []string{"verified", "pass"} {
		if strings.Contains(view, bad) {
			t.Errorf("view renders %q from a bare native exit\n%s", bad, view)
		}
	}
}

// TestVerificationMismatchLabelsRevisions renders the checked revision beside
// the candidate revision with no verified label (AC-7.2).
func TestVerificationMismatchLabelsRevisions(t *testing.T) {
	t.Parallel()
	b := newJournalBuilder(t)
	runID := "run_mismatch"
	commitA, commitB := strings.Repeat("a", 40), strings.Repeat("b", 40)
	taskSHA := b.writeTask(runID, []byte("# Add pagination to the audit log\n"))
	b.addRun(runID, "ready_for_review", taskSHA)
	attemptID := b.addAttempt(runID, "succeeded_native")
	b.addCandidate(runID, attemptID, strings.Repeat("0", 40), commitB)
	b.addVerification(runID, commitA, "passed")
	b.close()

	snap, err := viewmodel.Load(t.Context(), b.dir, runID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	m := newModel(testConfig(fullCaps, t))
	m.loadDiff = stubDiff(mustParseDiff(t, smallDiffInput))
	m.setSnapshot(t.Context(), snap)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	view := updated.(*Model).View()
	if !strings.Contains(view, "checks ran against aaaaaaa — candidate is bbbbbbb") {
		t.Errorf("view missing paired-revision label\n%s", view)
	}
	// Word-boundary: the next-action string legitimately contains
	// "unverified", which must not trip the verified-label ban.
	if matched, _ := regexp.MatchString(`\bverified\b`, view); matched {
		t.Errorf("view renders a verified label for mismatched revisions\n%s", view)
	}
}

// TestTruncationFooterNamesExactCommand renders the provenance footer with
// the exact fallback command for line- and byte-truncated diffs.
func TestTruncationFooterNamesExactCommand(t *testing.T) {
	t.Parallel()
	base, commit := strings.Repeat("b", 40), strings.Repeat("c", 40)
	exact := `git -C "/tmp/tui ws" diff ` + base + " " + commit
	truncated := func(t *testing.T, d Diff) string {
		t.Helper()
		m := newModel(testConfig(fullCaps, t))
		m.loadDiff = stubDiff(d)
		snap := richSnapshot()
		snap.Run.State = "ready_for_review"
		snap.Candidate.BaseRev, snap.Candidate.Commit = base, commit
		m.setSnapshot(t.Context(), snap)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
		return updated.(*Model).View()
	}

	t.Run("lines", func(t *testing.T) {
		t.Parallel()
		input, _ := lineTruncationDiff(30000)
		d, err := ParseDiff([]byte(input))
		if err != nil {
			t.Fatalf("ParseDiff: %v", err)
		}
		if !d.Truncated || d.TruncateCap != truncateLines {
			t.Fatalf("Truncated=%v cap=%q, want lines cap", d.Truncated, d.TruncateCap)
		}
		view := truncated(t, d)
		counts := fmt.Sprintf("showing %d of %d lines", len(d.Lines), d.TotalLines())
		for _, want := range []string{counts, "50,000-line limit", exact} {
			if !strings.Contains(view, want) {
				t.Errorf("line-truncated view missing %q\n%s", want, view)
			}
		}
	})
	t.Run("bytes", func(t *testing.T) {
		t.Parallel()
		d, err := ParseDiff([]byte(strings.Repeat("x", 5<<20)))
		if err != nil {
			t.Fatalf("ParseDiff: %v", err)
		}
		if !d.Truncated || d.TruncateCap != truncateBytes {
			t.Fatalf("Truncated=%v cap=%q, want bytes cap", d.Truncated, d.TruncateCap)
		}
		view := truncated(t, d)
		for _, want := range []string{"showing 0 of 1 lines", "4 MiB byte limit", exact} {
			if !strings.Contains(view, want) {
				t.Errorf("byte-truncated view missing %q\n%s", want, view)
			}
		}
		if strings.Contains(view, "50,000") {
			t.Errorf("byte-truncated footer claims the line cap\n%s", view)
		}
	})
}

// TestEscapesRenderedInert journals ANSI escapes in task, reason and diff
// text and asserts View emits no escape introductions (§12.7). ColourNever
// caps isolate passthrough: with colour on, the theme's own styling
// legitimately introduces escapes.
func TestEscapesRenderedInert(t *testing.T) {
	t.Parallel()
	for _, c := range []caps.Caps{asciiCaps, {Colour: caps.ColourNever, Motion: caps.MotionFull, Icons: caps.IconsUnicode}} {
		m := newModel(testConfig(c, t))
		snap := richSnapshot()
		snap.Run.State = "executing"
		snap.Goal = "\x1b[31mred alert\x1b[0m"
		snap.Run.Reason = "\x1b]8;;http://evil.example\x07link\x1b]8;;\x07"
		evil := strings.Replace(smallDiffInput, "context row", "\x1b[2Jwiped row", 1)
		m.loadDiff = stubDiff(mustParseDiff(t, evil))
		m.setSnapshot(t.Context(), snap)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
		view := updated.(*Model).View()
		if strings.Contains(view, "\x1b") {
			t.Errorf("caps %+v: view leaks an escape introduction\n%q", c, view)
		}
		for _, want := range []string{"red alert", "link", "wiped row"} {
			if !strings.Contains(view, want) {
				t.Errorf("caps %+v: sanitised view lost %q\n%s", c, want, view)
			}
		}
	}
}

// TestNoFontDependentGlyphs asserts the ASCII golden is byte-clean, the
// unicode golden stays inside the styles.go geometric-shapes table with no
// private-use or emoji rune, and the ASCII twin carries every text label
// (AC-1.5, I13).
func TestNoFontDependentGlyphs(t *testing.T) {
	t.Parallel()
	mk := func(c caps.Caps) string {
		m := newModel(testConfig(c, t))
		snap := richSnapshot()
		snap.Goal = "Add pagination to the audit log " + strings.Repeat("with feeling ", 20)
		m.loadDiff = stubDiff(mustParseDiff(t, smallDiffInput))
		m.setSnapshot(t.Context(), snap)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
		return updated.(*Model).View()
	}
	ascii, uni := mk(asciiCaps), mk(fullCaps)

	for i := 0; i < len(ascii); i++ {
		if ascii[i] >= 0x80 {
			t.Errorf("ASCII golden has non-ASCII byte at %d: %q", i, ascii)
			break
		}
	}
	// The table itself carries no private-use or emoji rune.
	for _, r := range geometricShapes {
		if inPrivateUseOrEmoji(r) {
			t.Errorf("geometric-shapes table holds forbidden rune U+%04X", r)
		}
		if !inGeometricScope(r) {
			t.Errorf("geometric-shapes table holds out-of-scope rune U+%04X", r)
		}
	}
	allowed := make(map[rune]bool)
	for _, r := range geometricShapes {
		allowed[r] = true
	}
	for _, r := range uni {
		if r < 0x80 || r == '\n' {
			continue
		}
		if inPrivateUseOrEmoji(r) {
			t.Errorf("unicode golden holds forbidden rune U+%04X", r)
		}
		if !allowed[r] {
			t.Errorf("unicode golden holds rune U+%04X outside the geometric-shapes table", r)
		}
	}
	if !strings.Contains(uni, "…") {
		t.Errorf("unicode golden never exercises the truncation marker\n%s", uni)
	}
	// Label parity: the ASCII twin carries every text label.
	labels := []string{
		"Goal:", "TASKS", "AGENTS", "SELECTED CHANGE", "Next action:",
		"verified", "qualified:", "paid continuation:", "native exit:",
		"Tab focus", "Enter inspect", "attempt 1", "assistant turns",
	}
	for _, label := range labels {
		if !strings.Contains(uni, label) {
			t.Errorf("unicode golden missing label %q", label)
		}
		if !strings.Contains(ascii, label) {
			t.Errorf("ASCII golden missing label %q its unicode twin carries", label)
		}
	}
}

// inPrivateUseOrEmoji reports runes no golden may ever carry: private-use,
// emoji blocks, dingbats, variation selectors and the joiner.
func inPrivateUseOrEmoji(r rune) bool {
	switch {
	case r >= 0xE000 && r <= 0xF8FF:
		return true
	case r >= 0x1F000 && r <= 0x1FAFF:
		return true
	case r >= 0x2600 && r <= 0x27BF:
		return true
	case r >= 0x2B00 && r <= 0x2BFF:
		return true
	case r >= 0xFE00 && r <= 0xFE0F:
		return true
	case r == 0x200D:
		return true
	default:
		return false
	}
}

// inGeometricScope reports the blocks the geometric-shapes table draws from:
// box drawing, geometric shapes, and the middle dot, ellipsis and em dash.
func inGeometricScope(r rune) bool {
	switch {
	case r >= 0x2500 && r <= 0x257F:
		return true
	case r >= 0x25A0 && r <= 0x25FF:
		return true
	case r == '·' || r == '…' || r == '—':
		return true
	default:
		return false
	}
}

// TestLayeringNoCliImport fails when production sources under internal/tui
// reference the CLI package: the layering runs one way only. The forbidden
// reference is assembled below so this test never names it.
func TestLayeringNoCliImport(t *testing.T) {
	t.Parallel()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(self)
	var scanned []string
	var walk func(dir string)
	walk = func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if e.IsDir() {
				walk(p)
				continue
			}
			if strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
				scanned = append(scanned, p)
			}
		}
	}
	walk(root)
	if len(scanned) == 0 {
		t.Fatal("no production sources scanned; the absence check would false-pass")
	}
	bad := "internal" + "/" + "cl" + "i"
	for _, f := range scanned {
		body, err := os.ReadFile(f) // #nosec G304 -- test scans its own tree sources
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), bad) {
			t.Errorf("%s references the CLI package", f)
		}
	}
}
