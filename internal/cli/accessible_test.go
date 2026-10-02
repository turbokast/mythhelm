package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

// accessibleFixture journals a run's events and projections in a fresh temp
// state dir.
type accessibleFixture struct {
	t   *testing.T
	dir string
	j   *journal.Journal
	seq int64
}

func newAccessibleFixture(t *testing.T) *accessibleFixture {
	t.Helper()
	dir := t.TempDir()
	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return &accessibleFixture{t: t, dir: dir, j: j}
}

func (f *accessibleFixture) append(runID, typ, payload string, project func(*sql.Tx) error) {
	f.t.Helper()
	f.appendObserved(runID, "", typ, payload, time.Now().UTC(), project)
}

func (f *accessibleFixture) appendObserved(runID, attemptID, typ, payload string, at time.Time, project func(*sql.Tx) error) {
	f.t.Helper()
	f.seq++
	ev := journal.Event{
		SchemaVersion:    journal.EnvelopeVersion,
		EventID:          ids.New("evt"),
		RunID:            runID,
		AttemptID:        attemptID,
		ProducerID:       "producer_test",
		ProducerSequence: f.seq,
		Generation:       1,
		ObservedAt:       at,
		Type:             typ,
		Payload:          json.RawMessage(payload),
	}
	if err := f.j.Append(f.t.Context(), ev, project); err != nil {
		f.t.Fatalf("Append(%s): %v", typ, err)
	}
}

func (f *accessibleFixture) addRun(runID, state, taskSHA string) {
	f.t.Helper()
	now := time.Now().UTC()
	row := journal.RunRow{
		RunID: runID, State: state, AdapterID: "builtin/fake", SourceRepo: "/repo",
		TaskSHA256: taskSHA, BillingPosture: "local-scripted", ExecutionProfile: "trusted-host",
		CreatedAt: now, UpdatedAt: now,
	}
	f.append(runID, "run.created", `{"state":`+strconv.Quote(state)+`}`, func(tx *sql.Tx) error {
		return journal.InsertRun(f.t.Context(), tx, row)
	})
}

func (f *accessibleFixture) addAttempt(runID string) string {
	f.t.Helper()
	attemptID := ids.New("att")
	row := journal.AttemptRow{
		AttemptID: attemptID, RunID: runID, TaskID: ids.New("task"), AttemptNumber: 1,
		State: "running", LaunchTokenSHA256: "token", WorkspacePath: f.dir,
	}
	f.appendObserved(runID, attemptID, "attempt.launch_intent_recorded", `{}`, time.Now().UTC(), func(tx *sql.Tx) error {
		return journal.InsertAttempt(f.t.Context(), tx, row)
	})
	return attemptID
}

// writeTask stores the admitted task file and returns its SHA-256 hex digest.
func (f *accessibleFixture) writeTask(runID string, content []byte) string {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Join(f.dir, "runs", runID), 0o700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.dir, "runs", runID, "task.md"), content, 0o600); err != nil {
		f.t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// runAccessibleToEnd runs RunAccessible until the test cancels it, returning
// the full output. Cancellation waits for the first line (proof the run
// started, so a slow scheduler can never cancel before Load) plus nap, which
// uses a short poll cadence so the run observes several live ticks first.
func runAccessibleToEnd(t *testing.T, dir, runID string, poll, nap time.Duration) string {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	sb := &syncBuffer{}
	done := make(chan error, 1)
	go func() {
		done <- RunAccessible(ctx, AccessibleConfig{RunID: runID, StateDir: dir, Out: sb, Poll: poll})
	}()
	waitAccessibleLines(t, sb, 1)
	time.Sleep(nap)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("RunAccessible: %v", err)
	}
	return sb.String()
}

func accessibleLines(t *testing.T, out string) []string {
	t.Helper()
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("output does not end in a newline: %q", out)
	}
	return strings.Split(strings.TrimSuffix(out, "\n"), "\n")
}

// syncBuffer is a goroutine-safe writer for runs the test reads mid-stream.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (s *syncBuffer) lines() []string {
	out := s.String()
	if out == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(out, "\n"), "\n")
}

func waitAccessibleLines(t *testing.T, sb *syncBuffer, n int) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if len(sb.lines()) >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d lines, have %d", n, len(sb.lines()))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func eventSeq(t *testing.T, line string) int64 {
	t.Helper()
	rest, ok := strings.CutPrefix(line, "event ")
	if !ok {
		t.Fatalf("line is not an event line: %q", line)
	}
	num, _, _ := strings.Cut(rest, ":")
	seq, err := strconv.ParseInt(num, 10, 64)
	if err != nil {
		t.Fatalf("event line has no sequence: %q", line)
	}
	return seq
}

func TestAccessibleLabels(t *testing.T) {
	t.Parallel()
	f := newAccessibleFixture(t)
	runID := ids.New("run")
	taskSHA := f.writeTask(runID, []byte("# Ship the widget\n\nBody.\n"))
	f.addRun(runID, "executing", taskSHA)
	f.append(runID, "admission.decided", `{"adapter":{"id":"builtin/fake","version":"0.1",`+
		`"harness":"fake","surface":"scripted"},`+
		`"billing":{"mode":"local-scripted","qualified":true,"paid_continuation":"off"}}`, nil)
	f.addAttempt(runID)
	f.append(runID, "attempt.launched", `{"worker_pid":123,"native_pid":456}`, nil)
	f.append(runID, "attempt.progress", `{"assistant_turns":7,"tool_uses":{"Read":3},"retries":1}`, nil)

	out := runAccessibleToEnd(t, f.dir, runID, 10*time.Millisecond, 50*time.Millisecond)
	lines := accessibleLines(t, out)

	wantSummary := []string{
		"goal: Ship the widget",
		"run state: executing (reason: none)",
		"attempt 1: running",
		"candidate: none recorded",
		"verification: waiting",
		"progress: 7 assistant turns, 1 retries (native-reported)",
		"admission: adapter builtin/fake 0.1 (scripted); qualified: true; paid continuation: off",
		"next action: wait for worker to finish",
		"actions: stop",
	}
	if len(lines) < len(wantSummary)+1 {
		t.Fatalf("got %d lines, want at least %d:\n%s", len(lines), len(wantSummary)+1, out)
	}
	for i, want := range wantSummary {
		if lines[i] != want {
			t.Errorf("summary line %d = %q, want %q", i, lines[i], want)
		}
	}
	// The stream carries the five journaled events in order, then the trailer
	// echoing the max streamed sequence.
	stream := lines[len(wantSummary):]
	if len(stream) != 6 {
		t.Fatalf("got %d stream lines, want 5 events + trailer:\n%s", len(stream), out)
	}
	for i, line := range stream[:5] {
		if seq := eventSeq(t, line); seq != int64(i+1) {
			t.Errorf("stream line %d has sequence %d, want %d", i, seq, i+1)
		}
	}
	if stream[5] != "next-after: 5" {
		t.Errorf("trailer = %q, want %q", stream[5], "next-after: 5")
	}
	// Full action names, not single-key hints: the actions line names the
	// stop action in full.
	if !strings.Contains(out, "actions: stop") {
		t.Errorf("output lacks the full stop action name:\n%s", out)
	}
}

func TestAccessibleEmptyValuesRenderUnknown(t *testing.T) {
	t.Parallel()
	// Populated rows with empty stored values: every label renders unknown,
	// never blank (I09). The snapshot is built directly — no journal row
	// carries these blanks, the view-model leaves missing keys empty.
	emptySignal := ""
	snap := viewmodel.Snapshot{
		Goal:         "unknown",
		Attempt:      &journal.AttemptRow{AttemptNumber: 2},
		Candidate:    &journal.CandidateRow{ChangedPaths: json.RawMessage(`[]`)},
		Verification: &journal.VerificationRow{},
		Admission:    &viewmodel.Admission{},
		NativeExit:   &viewmodel.NativeExit{Signal: &emptySignal},
	}
	want := []string{
		"goal: unknown",
		"run state: unknown (reason: none)",
		"attempt 2: unknown",
		"candidate: unknown (0 files changed)",
		"verification: unknown (candidate unknown)",
		"progress: unknown",
		"admission: adapter unknown unknown (unknown); qualified: false; paid continuation: unknown",
		"native exit: signal unknown; no result frame observed",
		"next action: unknown",
		"actions: none available",
	}
	lines := accessibleSummary(snap)
	if len(lines) != len(want) {
		t.Fatalf("summary has %d lines, want %d: %q", len(lines), len(want), lines)
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("summary line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestAccessibleShowsSummary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		after      int64
		historyLen int
		want       bool
	}{
		{0, 0, true},
		{0, 1, true},
		{0, accessiblePageSize - 1, true},
		// A full page is still complete (EventsSince is uncapped), so a
		// fresh invocation over exactly accessiblePageSize events keeps
		// its labels; only an over-page history pages without them.
		{0, accessiblePageSize, true},
		{0, accessiblePageSize + 1, false},
		{1, 0, false},
		{int64(accessiblePageSize), 5, false},
	}
	for _, tc := range cases {
		if got := accessibleShowsSummary(tc.after, tc.historyLen); got != tc.want {
			t.Errorf("accessibleShowsSummary(%d, %d) = %t, want %t",
				tc.after, tc.historyLen, got, tc.want)
		}
	}
}

func TestAccessibleNoChatter(t *testing.T) {
	t.Parallel()
	f := newAccessibleFixture(t)
	runID := ids.New("run")
	f.addRun(runID, "executing", strings.Repeat("0", 64))

	out := runAccessibleToEnd(t, f.dir, runID, 5*time.Millisecond, 60*time.Millisecond)

	want := strings.Join([]string{
		"goal: unknown",
		"run state: executing (reason: none)",
		"attempt: none recorded",
		"candidate: none recorded",
		"verification: waiting",
		"progress: unknown",
		"admission: unknown",
		"next action: wait for worker to finish",
		"actions: stop",
		"event 1: run " + runID + ": created",
		"next-after: 1",
	}, "\n") + "\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestAccessibleNoCursorCodes(t *testing.T) {
	t.Parallel()
	f := newAccessibleFixture(t)
	runID := ids.New("run")
	taskSHA := f.writeTask(runID, []byte("# Do \x1b[2Jthings\n"))
	f.addRun(runID, "executing", taskSHA)
	// The hostile reason arrives through a state-change event; the stream
	// must sanitise it to visible text.
	f.append(runID, "run.state_changed", `{"state":"executing","reason":"ok`+"\\u001b"+`[31mRED`+"\\u001b"+`[0m"}`, nil)

	out := runAccessibleToEnd(t, f.dir, runID, 10*time.Millisecond, 50*time.Millisecond)

	if strings.Contains(out, "\x1b") {
		t.Errorf("output contains escape bytes: %q", out)
	}
	if !strings.Contains(out, "goal: Do things") {
		t.Errorf("hostile goal did not sanitise to visible text: %q", out)
	}
	if !strings.Contains(out, "run state: executing (reason: none)") {
		t.Errorf("run state line missing or altered: %q", out)
	}
	if !strings.Contains(out, "event 2: run: executing (okRED)") {
		t.Errorf("hostile event reason did not sanitise to visible text: %q", out)
	}
}

func TestAccessibleOrderedStream(t *testing.T) {
	t.Parallel()
	f := newAccessibleFixture(t)
	runID := ids.New("run")
	f.addRun(runID, "executing", strings.Repeat("0", 64))
	now := time.Now().UTC()
	// Observed timestamps deliberately shuffle: the stream must follow
	// run_sequence, never wall-clock order.
	f.appendObserved(runID, "", "attempt.progress", `{"assistant_turns":1}`, now.Add(2*time.Hour), nil)
	f.appendObserved(runID, "", "attempt.progress", `{"assistant_turns":2}`, now.Add(-2*time.Hour), nil)
	f.appendObserved(runID, "", "attempt.progress", `{"assistant_turns":3}`, now.Add(time.Hour), nil)

	stored, err := journal.OpenReadOnly(t.Context(), f.dir)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	events, err := stored.Events(t.Context(), runID, 0)
	_ = stored.Close()
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	ascending := true
	for i := 1; i < len(events); i++ {
		if !events[i].ObservedAt.After(events[i-1].ObservedAt) {
			ascending = false
		}
	}
	if ascending {
		t.Fatal("fixture timestamps are ascending; the test would pass vacuously")
	}

	out := runAccessibleToEnd(t, f.dir, runID, 10*time.Millisecond, 50*time.Millisecond)
	lines := accessibleLines(t, out)
	var seqs []int64
	for _, line := range lines {
		if strings.HasPrefix(line, "event ") {
			seqs = append(seqs, eventSeq(t, line))
		}
	}
	if len(seqs) != 4 {
		t.Fatalf("got %d event lines, want 4: %q", len(seqs), out)
	}
	for i, seq := range seqs {
		if seq != int64(i+1) {
			t.Fatalf("event order = %v, want run_sequence order [1 2 3 4]", seqs)
		}
	}
}

// testCells counts s in terminal cells with an independent minimal table:
// combining marks take none, the CJK and emoji blocks take two, the rest one.
func testCells(s string) int {
	n := 0
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r):
		case r >= 0x4E00 && r <= 0x9FFF:
			n += 2
		case r >= 0x1F300 && r <= 0x1FAFF:
			n += 2
		default:
			n++
		}
	}
	return n
}

func TestAccessibleWideSafe(t *testing.T) {
	t.Parallel()
	f := newAccessibleFixture(t)
	runID := ids.New("run")
	// Combining marks (0 cells), emoji (2 cells), a bidi override and 120
	// wide CJK cells: the goal line must truncate by cells, never by bytes.
	title := strings.Repeat("e\u0301", 4) + strings.Repeat("\U0001F600", 2) + "\u202e" + strings.Repeat("\u4E2D", 120)
	taskSHA := f.writeTask(runID, []byte("# "+title+"\n"))
	f.addRun(runID, "executing", taskSHA)

	out := runAccessibleToEnd(t, f.dir, runID, 10*time.Millisecond, 50*time.Millisecond)
	lines := accessibleLines(t, out)

	// Title budget is 200 - len("goal: ") - len("... (truncated)") = 179
	// cells: 4 combining + 4 emoji + 1 replacement + 85 wide (170) = 179.
	wantGoal := "goal: " + strings.Repeat("e\u0301", 4) + strings.Repeat("\U0001F600", 2) + "\uFFFD" +
		strings.Repeat("\u4E2D", 85) + "... (truncated)"
	if lines[0] != wantGoal {
		t.Errorf("goal line = %q, want %q", lines[0], wantGoal)
	}
	if got := testCells(lines[0]); got != 200 {
		t.Errorf("goal line is %d cells, want exactly 200", got)
	}
	for i, line := range lines {
		if got := testCells(line); got > 200 {
			t.Errorf("line %d is %d cells, want at most 200: %q", i, got, line)
		}
	}
	if !utf8.ValidString(out) {
		t.Error("output is not valid UTF-8: truncation split a rune")
	}
	for _, r := range out {
		if r == 0x061C || r == 0x200E || r == 0x200F ||
			(r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) {
			t.Fatalf("output retains bidi control U+%04X", r)
		}
	}
	// The status line adjacent to the hostile goal is byte-exact: truncation
	// and neutralisation never displace it. (The pipeline has no approval
	// state — waiting_approval is never entered — so the run-state status
	// line is the adjacency this renderer defends.)
	if len(lines) < 2 || lines[1] != "run state: executing (reason: none)" {
		t.Errorf("status line after hostile goal = %q, want the exact run state line", lines)
	}
}

func TestAccessibleResumeFromCursor(t *testing.T) {
	t.Parallel()
	f := newAccessibleFixture(t)
	runID := ids.New("run")
	f.addRun(runID, "executing", strings.Repeat("0", 64))
	const extra = 5
	for i := 0; i < accessiblePageSize+extra-1; i++ {
		f.append(runID, "attempt.progress", fmt.Sprintf(`{"assistant_turns":%d}`, i+1), nil)
	}
	total := int64(accessiblePageSize + extra)

	// The first invocation pages exactly: history streams synchronously, so
	// waiting for the page then cancelling (with no live tick intervening)
	// yields the page plus its trailer and nothing else.
	first := runAccessiblePage(t, f.dir, runID, 0, accessiblePageSize)
	if len(first) != accessiblePageSize+1 {
		t.Fatalf("first invocation yielded %d lines, want %d + trailer", len(first), accessiblePageSize)
	}
	for i, line := range first[:accessiblePageSize] {
		if seq := eventSeq(t, line); seq != int64(i+1) {
			t.Fatalf("first page line %d has sequence %d, want %d", i, seq, i+1)
		}
	}
	if first[accessiblePageSize] != fmt.Sprintf("next-after: %d", accessiblePageSize) {
		t.Errorf("first trailer = %q, want %q", first[accessiblePageSize], fmt.Sprintf("next-after: %d", accessiblePageSize))
	}

	second := runAccessiblePage(t, f.dir, runID, int64(accessiblePageSize), extra)
	if len(second) != extra+1 {
		t.Fatalf("second invocation yielded %d lines, want %d + trailer", len(second), extra)
	}
	for i, line := range second[:extra] {
		if seq := eventSeq(t, line); seq != int64(accessiblePageSize+i+1) {
			t.Fatalf("second page line %d has sequence %d, want %d (overlap or gap)", i, seq, accessiblePageSize+i+1)
		}
	}
	if second[extra] != fmt.Sprintf("next-after: %d", total) {
		t.Errorf("second trailer = %q, want %q", second[extra], fmt.Sprintf("next-after: %d", total))
	}

	// A resume at the end echoes its cursor: trailer only, no events.
	echo := runAccessiblePage(t, f.dir, runID, total, 0)
	if len(echo) != 1 || echo[0] != fmt.Sprintf("next-after: %d", total) {
		t.Errorf("past-end resume = %q, want only the echo trailer", echo)
	}
}

// runAccessiblePage runs one paging invocation: resume offset after, waiting
// for want event lines before cancelling. The hour-long poll cadence keeps
// live ticks out of the page so the line count is exact.
func runAccessiblePage(t *testing.T, dir, runID string, after int64, want int) []string {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	sb := &syncBuffer{}
	done := make(chan error, 1)
	go func() {
		done <- RunAccessible(ctx, AccessibleConfig{
			RunID: runID, StateDir: dir, Out: sb, Poll: time.Hour, After: after,
		})
	}()
	if want == 0 {
		// No history line will arrive to prove startup, so settle
		// briefly instead: cancelling during Load errors with no
		// trailer, while cancelling a live follower echoes cleanly.
		time.Sleep(200 * time.Millisecond)
	} else {
		waitAccessibleLines(t, sb, want)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("RunAccessible(after=%d): %v", after, err)
	}
	return accessibleLines(t, sb.String())
}
