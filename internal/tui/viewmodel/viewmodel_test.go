package viewmodel_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

// builder journals a run's events and projections in a fresh temp state dir.
type builder struct {
	t      *testing.T
	dir    string
	j      *journal.Journal
	seq    int64
	events int
}

func newBuilder(t *testing.T) *builder {
	t.Helper()
	dir := t.TempDir()
	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	return &builder{t: t, dir: dir, j: j}
}

func (b *builder) append(runID, typ, payload string, project func(*sql.Tx) error) {
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
	b.events++
}

func (b *builder) addRun(runID, state, taskSHA string) {
	b.t.Helper()
	now := time.Now().UTC()
	row := journal.RunRow{
		RunID: runID, State: state, AdapterID: "builtin/fake", SourceRepo: "/repo",
		TaskSHA256: taskSHA, BillingPosture: "local-scripted", ExecutionProfile: "trusted-host",
		CreatedAt: now, UpdatedAt: now,
	}
	b.append(runID, "run.created", `{"state":`+quote(state)+`}`, func(tx *sql.Tx) error {
		return journal.InsertRun(b.t.Context(), tx, row)
	})
}

func (b *builder) addAttempt(runID string) string {
	b.t.Helper()
	attemptID := ids.New("att")
	row := journal.AttemptRow{
		AttemptID: attemptID, RunID: runID, TaskID: ids.New("task"), AttemptNumber: 1,
		State: "succeeded_native", LaunchTokenSHA256: "token", WorkspacePath: b.dir,
	}
	b.append(runID, "attempt.launch_intent_recorded", `{}`, func(tx *sql.Tx) error {
		return journal.InsertAttempt(b.t.Context(), tx, row)
	})
	return attemptID
}

func (b *builder) addCandidate(runID, attemptID, commit string) {
	b.t.Helper()
	row := journal.CandidateRow{
		AttemptID: attemptID, BaseRev: strings.Repeat("b", 40), Commit: commit,
		Tree: strings.Repeat("c", 40), PatchSHA256: strings.Repeat("d", 64),
		ChangedPaths: json.RawMessage(`["main.go"]`), Flags: json.RawMessage(`[]`),
	}
	b.append(runID, "candidate.frozen", `{}`, func(tx *sql.Tx) error {
		return journal.InsertCandidate(b.t.Context(), tx, row)
	})
}

func (b *builder) addVerification(runID, commit string) {
	b.t.Helper()
	now := time.Now().UTC()
	row := journal.VerificationRow{
		ID: ids.New("ver"), RunID: runID, CandidateCommit: commit, ConfigSHA256: "cfg",
		Result: "pass", StartedAt: now, FinishedAt: now,
		Checks: []journal.CheckRow{{Name: "build", Argv: []string{"go", "build"}, Status: "pass"}},
	}
	b.append(runID, "verification.completed", `{}`, func(tx *sql.Tx) error {
		return journal.InsertVerification(b.t.Context(), tx, row)
	})
}

// writeTask stores the admitted task file and returns its SHA-256 hex digest.
func (b *builder) writeTask(runID string, content []byte) string {
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

// writeReceipt installs receipt.json and journals its receipt.written event.
func (b *builder) writeReceipt(runID, state, title string) {
	b.t.Helper()
	runDir := filepath.Join(b.dir, "runs", runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		b.t.Fatal(err)
	}
	r := supervisor.Receipt{
		"schema_version": 1, "run_id": runID, "state": state,
		"requested_outcome": map[string]any{"title": title},
	}
	sha, err := supervisor.WriteReceipt(runDir, r)
	if err != nil {
		b.t.Fatal(err)
	}
	b.append(runID, "receipt.written", `{"schema_version":1,"sha256":`+quote(sha)+`,"path":"receipt.json"}`, nil)
}

func (b *builder) close() {
	b.t.Helper()
	if err := b.j.Close(); err != nil {
		b.t.Fatal(err)
	}
}

// reopen opens the same state dir for further appends after close.
func (b *builder) reopen() {
	b.t.Helper()
	j, err := journal.Open(b.t.Context(), b.dir)
	if err != nil {
		b.t.Fatalf("journal.Open: %v", err)
	}
	b.j = j
}

const (
	progressPayload  = `{"assistant_turns":7,"tool_uses":{"Read":2},"retries":1}`
	nativePayload    = `{"exit_code":0,"signal":null,"result_observed":true}`
	admissionPayload = `{"adapter":{"id":"builtin/fake","version":"0.1","harness":"fake",` +
		`"surface":"scripted"},"billing":{"mode":"local-scripted","qualified":true,"paid_continuation":"off"}}`
)

func TestLoadSnapshot(t *testing.T) {
	t.Parallel()
	b := newBuilder(t)
	runID := "run_snapshot"
	commit := strings.Repeat("a", 40)
	taskSHA := b.writeTask(runID, []byte("# Ship the widget\nDo the thing.\n"))
	b.addRun(runID, "ready_for_review", taskSHA)
	attemptID := b.addAttempt(runID)
	b.addCandidate(runID, attemptID, commit)
	b.addVerification(runID, commit)
	b.append(runID, "admission.decided", admissionPayload, nil)
	b.append(runID, "attempt.progress", progressPayload, nil)
	b.append(runID, "attempt.native_result", nativePayload, nil)
	b.writeReceipt(runID, "ready_for_review", "Ship the widget")
	wantEvents := int64(b.events)
	b.close()

	snap, err := viewmodel.Load(t.Context(), b.dir, runID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if snap.Run.RunID != runID || snap.Run.State != "ready_for_review" {
		t.Errorf("Run = %+v, want ID %s state ready_for_review", snap.Run, runID)
	}
	if snap.Attempt == nil || snap.Attempt.AttemptID != attemptID {
		t.Errorf("Attempt = %+v, want ID %s", snap.Attempt, attemptID)
	}
	if snap.Candidate == nil || snap.Candidate.Commit != commit {
		t.Errorf("Candidate = %+v, want commit %s", snap.Candidate, commit)
	}
	if snap.Verification == nil || snap.Verification.Result != "pass" {
		t.Errorf("Verification = %+v, want result pass", snap.Verification)
	}
	if snap.Receipt == nil || snap.Receipt["run_id"] != runID {
		t.Errorf("Receipt = %v, want run_id %s", snap.Receipt, runID)
	}
	if snap.Goal != "Ship the widget" {
		t.Errorf("Goal = %q, want receipt title", snap.Goal)
	}
	if snap.LatestProgress == nil || snap.LatestProgress.AssistantTurns != 7 {
		t.Errorf("LatestProgress = %+v, want 7 turns", snap.LatestProgress)
	}
	if snap.NativeExit == nil || snap.NativeExit.ExitCode == nil || *snap.NativeExit.ExitCode != 0 {
		t.Errorf("NativeExit = %+v, want exit code 0", snap.NativeExit)
	}
	if snap.Admission == nil || snap.Admission.AdapterID != "builtin/fake" {
		t.Errorf("Admission = %+v, want adapter builtin/fake", snap.Admission)
	}
	if snap.LastRunSeq != wantEvents {
		t.Errorf("LastRunSeq = %d, want %d journaled events", snap.LastRunSeq, wantEvents)
	}
	if snap.At.IsZero() {
		t.Error("At is zero, want snapshot time")
	}

	if _, err := viewmodel.Load(t.Context(), b.dir, ""); !errors.Is(err, journal.ErrNotFound) {
		t.Errorf("Load(empty runID) err = %v, want wrapping journal.ErrNotFound", err)
	}
}

func TestLoadScansProgressAndNativeExit(t *testing.T) {
	t.Parallel()
	t.Run("folded", func(t *testing.T) {
		t.Parallel()
		b := newBuilder(t)
		runID := "run_fold"
		b.addRun(runID, "executing", strings.Repeat("e", 64))
		b.append(runID, "attempt.progress", `{"assistant_turns":3,"tool_uses":{},"retries":0}`, nil)
		b.append(runID, "attempt.progress", progressPayload, nil)
		b.append(runID, "attempt.native_result", nativePayload, nil)
		progressSeq, nativeSeq := int64(3), int64(4)
		b.close()

		snap, err := viewmodel.Load(t.Context(), b.dir, runID)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if snap.LatestProgress == nil || snap.LatestProgress.AssistantTurns != 7 {
			t.Fatalf("LatestProgress = %+v, want latest 7 turns", snap.LatestProgress)
		}
		if snap.LatestProgress.RunSeq != progressSeq {
			t.Errorf("LatestProgress.RunSeq = %d, want %d", snap.LatestProgress.RunSeq, progressSeq)
		}
		if got := snap.LatestProgress.ToolUses["Read"]; got != 2 {
			t.Errorf("ToolUses[Read] = %d, want 2", got)
		}
		if snap.LatestProgress.Retries != 1 {
			t.Errorf("Retries = %d, want 1", snap.LatestProgress.Retries)
		}
		if snap.NativeExit == nil || snap.NativeExit.ExitCode == nil || *snap.NativeExit.ExitCode != 0 {
			t.Fatalf("NativeExit = %+v, want exit code 0", snap.NativeExit)
		}
		if snap.NativeExit.Signal != nil {
			t.Errorf("Signal = %v, want nil for a clean exit", *snap.NativeExit.Signal)
		}
		if !snap.NativeExit.ResultObserved {
			t.Error("ResultObserved = false, want true")
		}
		if snap.NativeExit.RunSeq != nativeSeq {
			t.Errorf("NativeExit.RunSeq = %d, want %d", snap.NativeExit.RunSeq, nativeSeq)
		}
	})
	t.Run("absent stays nil", func(t *testing.T) { // I09
		t.Parallel()
		b := newBuilder(t)
		runID := "run_bare"
		b.addRun(runID, "executing", strings.Repeat("e", 64))
		b.close()

		snap, err := viewmodel.Load(t.Context(), b.dir, runID)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if snap.LatestProgress != nil {
			t.Errorf("LatestProgress = %+v, want nil without progress events", snap.LatestProgress)
		}
		if snap.NativeExit != nil {
			t.Errorf("NativeExit = %+v, want nil without native-result events", snap.NativeExit)
		}
	})
}

func TestLoadScopesAttemptEvidence(t *testing.T) {
	t.Parallel()
	b := newBuilder(t)
	runID := "run_retry"
	b.addRun(runID, "executing", strings.Repeat("e", 64))
	oldAttempt := b.addAttempt(runID)
	appendAttempt := func(typ, payload, attemptID string) {
		t.Helper()
		b.seq++
		ev := journal.Event{
			SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"),
			RunID: runID, AttemptID: attemptID, ProducerID: "producer_test",
			ProducerSequence: b.seq, Generation: 1, ObservedAt: time.Now().UTC(),
			Type: typ, Payload: json.RawMessage(payload),
		}
		if err := b.j.Append(t.Context(), ev, nil); err != nil {
			t.Fatalf("Append(%s): %v", typ, err)
		}
	}
	appendAttempt("attempt.progress", progressPayload, oldAttempt)
	appendAttempt("attempt.native_result", nativePayload, oldAttempt)
	newAttempt := ids.New("att")
	b.append(runID, "attempt.launch_intent_recorded", `{}`, func(tx *sql.Tx) error {
		return journal.InsertAttempt(t.Context(), tx, journal.AttemptRow{
			AttemptID: newAttempt, RunID: runID, TaskID: ids.New("task"), AttemptNumber: 2,
			State: "running", LaunchTokenSHA256: "token", WorkspacePath: b.dir,
		})
	})
	b.close()

	snap, err := viewmodel.Load(t.Context(), b.dir, runID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if snap.Attempt == nil || snap.Attempt.AttemptID != newAttempt {
		t.Fatalf("Attempt = %+v, want the running retry %s", snap.Attempt, newAttempt)
	}
	// The previous attempt's progress and result must not masquerade as the
	// retry's launch evidence: a new running retry waits for its own ack.
	if snap.LatestProgress != nil {
		t.Errorf("LatestProgress = %+v, want nil (stale attempt evidence)", snap.LatestProgress)
	}
	if snap.NativeExit != nil {
		t.Errorf("NativeExit = %+v, want nil (stale attempt evidence)", snap.NativeExit)
	}
}

func TestLoadScansAdmission(t *testing.T) {
	t.Parallel()
	t.Run("folded", func(t *testing.T) {
		t.Parallel()
		b := newBuilder(t)
		runID := "run_adm"
		b.addRun(runID, "executing", strings.Repeat("e", 64))
		b.append(runID, "admission.decided", admissionPayload, nil)
		b.close()

		snap, err := viewmodel.Load(t.Context(), b.dir, runID)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if snap.Admission == nil {
			t.Fatal("Admission is nil, want folded admission")
		}
		adm := snap.Admission
		if adm.AdapterID != "builtin/fake" || adm.AdapterVersion != "0.1" || adm.AdapterSurface != "scripted" {
			t.Errorf("adapter = %+v, want builtin/fake 0.1 scripted", adm)
		}
		if !adm.Qualified || adm.PaidContinuation != "off" {
			t.Errorf("billing = qualified %v paid_continuation %q, want true off", adm.Qualified, adm.PaidContinuation)
		}
		if adm.RunSeq != 2 {
			t.Errorf("RunSeq = %d, want 2", adm.RunSeq)
		}
	})
	t.Run("absent stays nil", func(t *testing.T) { // I09
		t.Parallel()
		b := newBuilder(t)
		runID := "run_noadm"
		b.addRun(runID, "executing", strings.Repeat("e", 64))
		b.close()

		snap, err := viewmodel.Load(t.Context(), b.dir, runID)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if snap.Admission != nil {
			t.Errorf("Admission = %+v, want nil without admission.decided", snap.Admission)
		}
	})
}

func TestLoadScanFailures(t *testing.T) {
	t.Parallel()
	// The journal rejects non-JSON payloads (CHECK json_valid), so a
	// malformed payload is valid JSON of the wrong shape.
	t.Run("bad progress skipped", func(t *testing.T) {
		t.Parallel()
		b := newBuilder(t)
		runID := "run_badprog"
		b.addRun(runID, "executing", strings.Repeat("e", 64))
		b.append(runID, "attempt.progress", progressPayload, nil)
		b.append(runID, "attempt.progress", `{"assistant_turns":"seven"}`, nil)
		wantSeq := int64(b.events)
		b.close()

		snap, err := viewmodel.Load(t.Context(), b.dir, runID)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if snap.LatestProgress == nil || snap.LatestProgress.AssistantTurns != 7 {
			t.Errorf("LatestProgress = %+v, want latest good 7 turns", snap.LatestProgress)
		}
		if snap.LastRunSeq != wantSeq {
			t.Errorf("LastRunSeq = %d, want %d (advanced past bad payload)", snap.LastRunSeq, wantSeq)
		}
	})
	for _, tc := range []struct {
		name    string
		typ     string
		payload string
	}{
		{name: "bad native result fails", typ: "attempt.native_result", payload: `{"exit_code":"zero"}`},
		{name: "bad admission fails", typ: "admission.decided", payload: `{"adapter":42}`},
	} {
		t.Run(tc.name, func(t *testing.T) { // I07
			t.Parallel()
			b := newBuilder(t)
			runID := "run_badscan"
			b.addRun(runID, "executing", strings.Repeat("e", 64))
			b.append(runID, tc.typ, tc.payload, nil)
			badSeq := b.events
			b.close()

			_, err := viewmodel.Load(t.Context(), b.dir, runID)
			if err == nil {
				t.Fatalf("Load with malformed %s succeeded, want error", tc.typ)
			}
			if !strings.Contains(err.Error(), tc.typ) {
				t.Errorf("error %q does not name event type %s", err, tc.typ)
			}
			if !strings.Contains(err.Error(), strconv.Itoa(badSeq)) {
				t.Errorf("error %q does not name run_sequence %d", err, badSeq)
			}
		})
	}
}

func TestLoadNoReceiptWritten(t *testing.T) {
	t.Parallel()
	b := newBuilder(t)
	runID := "run_midrun"
	b.addRun(runID, "executing", strings.Repeat("e", 64))
	b.append(runID, "attempt.progress", progressPayload, nil)
	b.append(runID, "attempt.native_result", nativePayload, nil)
	b.close()

	// No receipt.json exists: calling ReadReceipt blindly would fail here
	// with a wrapped read error instead of returning a nil Receipt.
	snap, err := viewmodel.Load(t.Context(), b.dir, runID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if snap.Receipt != nil {
		t.Errorf("Receipt = %v, want nil without receipt.written", snap.Receipt)
	}
	if snap.LatestProgress == nil || snap.NativeExit == nil {
		t.Errorf("Progress = %+v NativeExit = %+v, want both folded", snap.LatestProgress, snap.NativeExit)
	}
}

func TestLoadReceiptWrittenWithoutFile(t *testing.T) {
	t.Parallel()
	b := newBuilder(t)
	runID := "run_nofile"
	b.addRun(runID, "ready_for_review", strings.Repeat("e", 64))
	b.writeReceipt(runID, "ready_for_review", "Gone")
	if err := os.Remove(filepath.Join(b.dir, "runs", runID, "receipt.json")); err != nil {
		t.Fatal(err)
	}
	b.close()

	_, err := viewmodel.Load(t.Context(), b.dir, runID)
	if err == nil {
		t.Fatal("Load with journaled receipt but deleted file succeeded, want error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Load err = %v, want the wrapped receipt read failure", err)
	}
}

func TestLoadCorruptReceipt(t *testing.T) {
	t.Parallel()
	t.Run("sha mismatch", func(t *testing.T) {
		t.Parallel()
		b := newBuilder(t)
		runID := "run_corrupt"
		b.addRun(runID, "ready_for_review", strings.Repeat("e", 64))
		b.writeReceipt(runID, "ready_for_review", "Corrupt")
		if err := os.WriteFile(filepath.Join(b.dir, "runs", runID, "receipt.json"), []byte("{not the journaled bytes}"), 0o600); err != nil {
			t.Fatal(err)
		}
		b.close()

		if _, err := viewmodel.Load(t.Context(), b.dir, runID); err == nil {
			t.Fatal("Load with SHA-mismatched receipt succeeded, want error")
		}
	})
	t.Run("json syntax error", func(t *testing.T) {
		t.Parallel()
		b := newBuilder(t)
		runID := "run_badjson"
		b.addRun(runID, "ready_for_review", strings.Repeat("e", 64))
		runDir := filepath.Join(b.dir, "runs", runID)
		if err := os.MkdirAll(runDir, 0o700); err != nil {
			t.Fatal(err)
		}
		// Journal the corrupt bytes' own digest so the failure is the JSON
		// decode, not the SHA check.
		bad := []byte(`{"run_id": broken}`)
		sum := sha256.Sum256(bad)
		if err := os.WriteFile(filepath.Join(runDir, "receipt.json"), bad, 0o600); err != nil {
			t.Fatal(err)
		}
		b.append(runID, "receipt.written", `{"sha256":`+quote(hex.EncodeToString(sum[:]))+`}`, nil)
		b.close()

		_, err := viewmodel.Load(t.Context(), b.dir, runID)
		if err == nil {
			t.Fatal("Load with syntactically invalid receipt succeeded, want error")
		}
		if _, ok := errors.AsType[*json.SyntaxError](err); !ok {
			t.Errorf("Load err = %v, want the JSON syntax failure", err)
		}
	})
}

func TestLoadEventsQueryError(t *testing.T) {
	t.Parallel()
	b := newBuilder(t)
	runID := "run_nojournal"
	b.addRun(runID, "executing", strings.Repeat("e", 64))
	b.addAttempt(runID)
	b.close()

	// Drop the journal table with the projections intact: every projection
	// read still succeeds, so only the event scan can report the failure.
	db, err := sql.Open("sqlite", filepath.Join(b.dir, journal.DBName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "DROP TABLE journal"); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := viewmodel.Load(t.Context(), b.dir, runID); err == nil {
		t.Fatal("Load with dropped journal table succeeded, want error")
	}
}

func TestLoadPartialRun(t *testing.T) {
	t.Parallel()
	b := newBuilder(t)
	runID := "run_partial"
	b.addRun(runID, "admission", strings.Repeat("e", 64))
	b.close()

	snap, err := viewmodel.Load(t.Context(), b.dir, runID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if snap.Attempt != nil || snap.Candidate != nil || snap.Verification != nil || snap.Receipt != nil {
		t.Errorf("Attempt/Candidate/Verification/Receipt = %+v/%+v/%+v/%v, want all nil",
			snap.Attempt, snap.Candidate, snap.Verification, snap.Receipt)
	}
	if snap.Run.RunID != runID {
		t.Errorf("Run.RunID = %q, want %s", snap.Run.RunID, runID)
	}
}

func TestReceiptStateMismatch(t *testing.T) {
	t.Parallel()
	t.Run("mismatch fails", func(t *testing.T) {
		t.Parallel()
		b := newBuilder(t)
		runID := "run_mismatch"
		b.addRun(runID, "ready_for_review", strings.Repeat("e", 64))
		b.writeReceipt(runID, "completed", "Mismatch")
		b.close()

		_, err := viewmodel.Load(t.Context(), b.dir, runID)
		if err == nil {
			t.Fatal("Load with receipt/run state mismatch succeeded, want error")
		}
		if !strings.Contains(err.Error(), "does not match its run projection") {
			t.Errorf("Load err = %v, want the agreement-check failure", err)
		}
	})
	t.Run("agreement passes", func(t *testing.T) {
		t.Parallel()
		b := newBuilder(t)
		runID := "run_agree"
		b.addRun(runID, "ready_for_review", strings.Repeat("e", 64))
		b.writeReceipt(runID, "ready_for_review", "Agree")
		b.close()

		snap, err := viewmodel.Load(t.Context(), b.dir, runID)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if snap.Receipt == nil {
			t.Error("Receipt is nil, want loaded receipt")
		}
	})
}

func TestEventsSinceIncremental(t *testing.T) {
	t.Parallel()
	b := newBuilder(t)
	runID := "run_stream"
	b.addRun(runID, "executing", strings.Repeat("e", 64))
	b.append(runID, "attempt.progress", progressPayload, nil)
	b.close()

	snap, err := viewmodel.Load(t.Context(), b.dir, runID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	b.reopen()
	b.append(runID, "attempt.progress", `{"assistant_turns":8,"tool_uses":{},"retries":1}`, nil)
	b.append(runID, "attempt.native_result", nativePayload, nil)
	b.close()

	got, err := viewmodel.EventsSince(t.Context(), b.dir, runID, snap.LastRunSeq)
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("EventsSince returned %d events, want the 2 journaled after the snapshot", len(got))
	}
	if got[0].Type != "attempt.progress" || got[1].Type != "attempt.native_result" {
		t.Errorf("event types = %s,%s, want progress then native_result", got[0].Type, got[1].Type)
	}
	if got[0].RunSequence != snap.LastRunSeq+1 || got[1].RunSequence != snap.LastRunSeq+2 {
		t.Errorf("sequences = %d,%d, want %d,%d in order",
			got[0].RunSequence, got[1].RunSequence, snap.LastRunSeq+1, snap.LastRunSeq+2)
	}
}

func TestNoDatabaseEmptyState(t *testing.T) {
	t.Parallel()
	if _, err := viewmodel.Load(t.Context(), t.TempDir(), "run_missing"); !errors.Is(err, journal.ErrNoDatabase) {
		t.Errorf("Load without database err = %v, want journal.ErrNoDatabase", err)
	}
}

func TestLoadGoalFromTaskFile(t *testing.T) {
	t.Parallel()
	t.Run("digest match yields title", func(t *testing.T) {
		t.Parallel()
		b := newBuilder(t)
		runID := "run_goal"
		taskSHA := b.writeTask(runID, []byte("# Fix the leak\nDetails.\n"))
		b.addRun(runID, "executing", taskSHA)
		b.close()

		snap, err := viewmodel.Load(t.Context(), b.dir, runID)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if snap.Goal != "Fix the leak" {
			t.Errorf("Goal = %q, want the digest-checked task title", snap.Goal)
		}
	})
	t.Run("digest mismatch fails", func(t *testing.T) { // I07
		t.Parallel()
		b := newBuilder(t)
		runID := "run_tampered"
		b.writeTask(runID, []byte("# Original\n"))
		b.addRun(runID, "executing", strings.Repeat("e", 64))
		b.close()

		if _, err := viewmodel.Load(t.Context(), b.dir, runID); err == nil {
			t.Fatal("Load with tampered task file succeeded, want error")
		}
	})
	t.Run("missing file yields unknown", func(t *testing.T) {
		t.Parallel()
		b := newBuilder(t)
		runID := "run_notask"
		b.addRun(runID, "executing", strings.Repeat("e", 64))
		b.close()

		snap, err := viewmodel.Load(t.Context(), b.dir, runID)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if snap.Goal != "unknown" {
			t.Errorf("Goal = %q, want unknown without a task file", snap.Goal)
		}
	})
}

// TestViewmodelNeverWrites fails when the package's production sources
// reference the read-write journal entry points (I18). Fixtures in test
// files legitimately use them, so only non-test sources are scanned. The
// forbidden references are assembled below so this test never names them.
func TestViewmodelNeverWrites(t *testing.T) { // I18
	t.Parallel()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(self), "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var scanned []string
	for _, f := range files {
		if !strings.HasSuffix(f, "_test.go") {
			scanned = append(scanned, f)
		}
	}
	if len(scanned) == 0 {
		t.Fatal("no production sources scanned; the absence check would false-pass")
	}
	pkg, dot, open, appendCall := "journal", ".", "Ope"+"n(", "Appe"+"nd("
	for _, f := range scanned {
		body, err := os.ReadFile(f) // #nosec G304 -- test scans its own package sources
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{pkg + dot + open, pkg + dot + appendCall} {
			if strings.Contains(string(body), bad) {
				t.Errorf("%s references a journal write entry point", f)
			}
		}
	}
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
