package control

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/v2contract"
	"github.com/turbokast/mythhelm/internal/workers"
)

// recoverState is a scripted recovery run: a stopState (journal plus ledger,
// a running attempt, an admission pin, a launched event and a worker.json
// naming this test process, which is always alive) with the attempt row
// carrying the sha256 of a known launch token.
type recoverState struct {
	*stopState
	launchToken string
	launcher    *recordingLauncher
	examined    *atomic.Int64
}

type recordingLauncher struct {
	mu    sync.Mutex
	calls []RecoveryLaunch
}

func (l *recordingLauncher) launch(_ context.Context, r RecoveryLaunch) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, r)
	return nil
}

func (l *recordingLauncher) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.calls)
}

func shahex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func newRecoverState(t *testing.T, runID, attemptID string) *recoverState {
	t.Helper()
	f := newStopState(t, runID, attemptID)
	token := "tok_" + attemptID
	if _, err := f.db.Exec(`UPDATE attempts SET launch_token_sha256 = ? WHERE attempt_id = ?`,
		shahex(token), attemptID); err != nil {
		t.Fatal(err)
	}
	writeWorkerJSON(t, f, workers.Identity{SchemaVersion: 1, RunID: runID, AttemptID: attemptID,
		PID: f.pid, StartTime: f.start, LaunchToken: token, Nonce: f.rawNonce})
	return &recoverState{stopState: f, launchToken: token,
		launcher: &recordingLauncher{}, examined: &atomic.Int64{}}
}

func (f *recoverState) deps() RecoverDeps {
	return RecoverDeps{DB: f.db, StateDir: f.dir, Launch: f.launcher.launch, ExamineCount: f.examined}
}

func (f *recoverState) reconcile(t *testing.T) (RecoverOutcome, error) {
	t.Helper()
	return Reconcile(t.Context(), f.deps(), f.runID)
}

// appendNativeLaunched journals one attempt.launched event with literal wire
// keys, naming a native PID when native is non-nil.
func appendNativeLaunched(t *testing.T, j *journal.Journal, runID, attemptID string, pid int, start time.Time, native *int, seq int64) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"worker_pid": pid, "worker_start_time": start, "native_pid": native, "native_pgid": nil})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Append(t.Context(), journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: fmt.Sprintf("evt_nlaunch_%s_%d", attemptID, seq),
		RunID: runID, AttemptID: attemptID, ProducerID: "wrk_" + attemptID,
		ProducerSequence: seq, Generation: 1, ObservedAt: time.Now().UTC(),
		Type: "attempt.launched", Payload: payload,
	}, nil); err != nil {
		t.Fatal(err)
	}
}

// appendNativeResult journals one attempt.native_result event for the
// attempt on the worker producer.
func appendNativeResult(t *testing.T, j *journal.Journal, runID, attemptID string, seq int64) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"result_observed": true})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Append(t.Context(), journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: fmt.Sprintf("evt_nresult_%s_%d", attemptID, seq),
		RunID: runID, AttemptID: attemptID, ProducerID: "wrk_" + attemptID,
		ProducerSequence: seq, Generation: 1, ObservedAt: time.Now().UTC(),
		Type: "attempt.native_result", Payload: payload,
	}, nil); err != nil {
		t.Fatal(err)
	}
}

// appendSpoolEvent appends one journal.Event envelope line to the attempt's
// spool.jsonl without journaling it: unacknowledged spool.
func appendSpoolEvent(t *testing.T, dir string, ev journal.Event) {
	t.Helper()
	line, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	spool, err := os.OpenFile(filepath.Join(dir, "spool.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600) //nolint:gosec // G304: a test temp path
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = spool.Close() }()
	if _, err := spool.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}
}

func spoolEvent(runID, attemptID, typ, eventID string) journal.Event {
	return journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: eventID,
		RunID: runID, AttemptID: attemptID, ProducerID: "wrk_" + attemptID,
		ProducerSequence: 99, Generation: 1, ObservedAt: time.Now().UTC(),
		Type: typ, Payload: json.RawMessage(`{"result_observed":true}`),
	}
}

func insertPartialCandidate(t *testing.T, db *sql.DB, attemptID, commit string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO candidates (attempt_id, base_rev, candidate_commit, tree_id, patch_sha256, changed_paths, flags, partial)
		VALUES (?, 'base', ?, 'tree', 'patch', '[]', '[]', 1)`, attemptID, commit); err != nil {
		t.Fatal(err)
	}
}

// recoveryEvents returns the run's journaled recovery outcomes in run_sequence
// order.
func recoveryEvents(t *testing.T, j *journal.Journal, runID string) []journal.Event {
	t.Helper()
	events, err := j.Events(t.Context(), runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []journal.Event
	for _, ev := range events {
		if ev.Type == EventRecoveryDecided {
			out = append(out, ev)
		}
	}
	return out
}

func decodeReport(t *testing.T, raw json.RawMessage) RecoveryReport {
	t.Helper()
	var rep RecoveryReport
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatal(err)
	}
	return rep
}

// attemptRows returns the run's (attempt_id, state, launch digest, number)
// ordered by attempt_number.
func attemptRows(t *testing.T, db *sql.DB, runID string) []struct {
	ID, State, Launch string
	Number            int64
} {
	t.Helper()
	rows, err := db.Query(`SELECT attempt_id, state, launch_token_sha256, attempt_number FROM attempts
		WHERE run_id = ? ORDER BY attempt_number`, runID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []struct {
		ID, State, Launch string
		Number            int64
	}
	for rows.Next() {
		var r struct {
			ID, State, Launch string
			Number            int64
		}
		if err := rows.Scan(&r.ID, &r.State, &r.Launch, &r.Number); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func recoverIntent(t *testing.T, op, runID string) Intent {
	t.Helper()
	params, err := json.Marshal(RecoverParams{RunID: runID})
	if err != nil {
		t.Fatal(err)
	}
	return Intent{OperationID: op, Method: "recover", Params: params}
}

// killWorker fixtures a dead worker: the journaled launch and worker.json
// agree on a PID with no live process behind it. It returns the dead start
// time.
func killWorker(t *testing.T, f *recoverState, native *int, seq int64) {
	t.Helper()
	deadStart := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	appendNativeLaunched(t, f.j, f.runID, f.attempt, 1<<30, deadStart, native, seq)
	writeWorkerJSON(t, f.stopState, workers.Identity{SchemaVersion: 1, RunID: f.runID, AttemptID: f.attempt,
		PID: 1 << 30, StartTime: deadStart, LaunchToken: f.launchToken, Nonce: f.rawNonce})
}

func TestReconcileChoosesOneOutcome(t *testing.T) { // FR-2 AC-2.1
	t.Parallel()
	if EventRecoveryDecided != "attempt.recovery_decided" {
		t.Fatalf("EventRecoveryDecided = %q, want the wire type", EventRecoveryDecided)
	}
	for _, outcome := range []RecoverOutcome{RecoverReconnected, RecoverContinued, RecoverPartialCandidate, RecoverQuarantined} {
		switch outcome {
		case "reconnected", "continued", "partial_candidate", "quarantined":
		default:
			t.Fatalf("RecoverOutcome %q is not one of the four v2 §6.4 dispositions", outcome)
		}
	}
	cases := []struct {
		name    string
		fixture func(t *testing.T, f *recoverState)
		outcome RecoverOutcome
		code    Code // "" means a nil error
		check   func(t *testing.T, f *recoverState, rep RecoveryReport)
	}{
		{
			name:    "live same worker reconnects",
			fixture: func(*testing.T, *recoverState) {},
			outcome: RecoverReconnected,
			check: func(t *testing.T, f *recoverState, _ RecoveryReport) {
				if got := attemptState(t, f.db, f.attempt); got != "running" {
					t.Fatalf("attempt state = %q, want running: reconnect moves nothing", got)
				}
				if len(attemptRows(t, f.db, f.runID)) != 1 {
					t.Fatalf("recover admitted an attempt on reconnect")
				}
			},
		},
		{
			name: "dead worker with clean workspace continues",
			fixture: func(t *testing.T, f *recoverState) {
				killWorker(t, f, nil, 2)
			},
			outcome: RecoverContinued,
			check: func(t *testing.T, f *recoverState, rep RecoveryReport) {
				rows := attemptRows(t, f.db, f.runID)
				if len(rows) != 2 {
					t.Fatalf("run holds %d attempts, want the recovered one plus its continuation", len(rows))
				}
				if rows[0].State != "interrupted" {
					t.Fatalf("recovered attempt state = %q, want interrupted", rows[0].State)
				}
				if rows[1].Number != 2 || rows[1].State != "launching" {
					t.Fatalf("continuation = %+v, want number 2 in launching", rows[1])
				}
				if rows[1].Launch != rows[0].Launch {
					t.Fatalf("continuation launch %q differs from %q: same launch identity required",
						rows[1].Launch, rows[0].Launch)
				}
				if rep.NewAttemptID != rows[1].ID {
					t.Fatalf("report new_attempt_id = %q, want %q", rep.NewAttemptID, rows[1].ID)
				}
			},
		},
		{
			name: "dead worker with partial output exposes the candidate",
			fixture: func(t *testing.T, f *recoverState) {
				killWorker(t, f, nil, 2)
				insertPartialCandidate(t, f.db, f.attempt, "commit_partial_1")
			},
			outcome: RecoverPartialCandidate,
			check: func(t *testing.T, f *recoverState, rep RecoveryReport) {
				if rep.CandidateCommit != "commit_partial_1" {
					t.Fatalf("report candidate_commit = %q, want the partial candidate", rep.CandidateCommit)
				}
				if len(attemptRows(t, f.db, f.runID)) != 1 {
					t.Fatalf("recover admitted an attempt for a partial candidate")
				}
				if got := attemptState(t, f.db, f.attempt); got != "interrupted" {
					t.Fatalf("attempt state = %q, want interrupted", got)
				}
			},
		},
		{
			name: "unverifiable identity quarantines",
			fixture: func(t *testing.T, f *recoverState) {
				// PID and start time agree with a live process, but the raw
				// nonce digests to a different value than the journaled one.
				writeWorkerJSON(t, f.stopState, workers.Identity{SchemaVersion: 1, RunID: f.runID,
					AttemptID: f.attempt, PID: f.pid, StartTime: f.start,
					LaunchToken: f.launchToken, Nonce: "someone-elses-nonce"})
			},
			outcome: RecoverQuarantined,
			code:    CodeOwnershipUnresolved,
			check: func(t *testing.T, f *recoverState, rep RecoveryReport) {
				if rep.Reason != "ownership_unresolved" {
					t.Fatalf("report reason = %q, want ownership_unresolved", rep.Reason)
				}
				if got := attemptState(t, f.db, f.attempt); got != "quarantined" {
					t.Fatalf("attempt state = %q, want quarantined", got)
				}
			},
		},
		{
			name: "live same worker with stale journal generation quarantines",
			fixture: func(t *testing.T, f *recoverState) {
				// The worker's producer moved to generation 2 (a restart
				// adopted it) while its journaled launch sits at 1.
				if _, err := f.db.Exec(`UPDATE producers SET generation = 2 WHERE producer_id = ?`,
					"wrk_"+f.attempt); err != nil {
					t.Fatal(err)
				}
			},
			outcome: RecoverQuarantined,
			code:    CodeOwnershipUnresolved,
			check: func(t *testing.T, f *recoverState, rep RecoveryReport) {
				if rep.Reason != "stale_generation" {
					t.Fatalf("report reason = %q, want stale_generation", rep.Reason)
				}
				if got := attemptState(t, f.db, f.attempt); got != "quarantined" {
					t.Fatalf("attempt state = %q, want quarantined", got)
				}
				// The stale evidence is retained for reconciliation, never
				// reaped: the launched event is still journaled.
				found := false
				events, err := f.j.Events(t.Context(), f.runID, 0)
				if err != nil {
					t.Fatal(err)
				}
				for _, ev := range events {
					if ev.Type == "attempt.launched" && ev.AttemptID == f.attempt {
						found = true
					}
				}
				if !found {
					t.Fatalf("the stale launched event is gone: evidence must be retained")
				}
			},
		},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newRecoverState(t, fmt.Sprintf("run_rec_%d", i), "att_rec_1")
			c.fixture(t, f)
			outcome, err := f.reconcile(t)
			if outcome != c.outcome {
				t.Fatalf("Reconcile = %q, want %q (err %v)", outcome, c.outcome, err)
			}
			if c.code == "" {
				if err != nil {
					t.Fatalf("Reconcile err = %v, want nil", err)
				}
			} else {
				requireCode(t, err, c.code)
			}
			// Reconcile decides and records; it never launches a native, even
			// for a continuation. Only the recover handler launches, on a
			// fresh continued outcome (I12).
			if n := f.launcher.count(); n != 0 {
				t.Fatalf("Reconcile launched %d natives, want zero", n)
			}
			if got := f.examined.Load(); got != 1 {
				t.Fatalf("examination passes = %d, want exactly one", got)
			}
			evs := recoveryEvents(t, f.j, f.runID)
			if len(evs) != 1 {
				t.Fatalf("journal holds %d recovery outcomes, want exactly one", len(evs))
			}
			rep := decodeReport(t, evs[0].Payload)
			if rep.RunID != f.runID || rep.AttemptID != f.attempt || rep.Outcome != c.outcome {
				t.Fatalf("journaled report = %+v, want the %q outcome", rep, c.outcome)
			}
			// The marker covers non-recovery events only, so the outcome
			// event itself never counts as fresh evidence on the next pass.
			var wantSeq int64
			all, err := f.j.Events(t.Context(), f.runID, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range all {
				if e.Type != EventRecoveryDecided && e.RunSequence > wantSeq {
					wantSeq = e.RunSequence
				}
			}
			if rep.Markers.JournalSeq != wantSeq {
				t.Fatalf("report journal_seq = %d, want the latest non-recovery run_sequence %d",
					rep.Markers.JournalSeq, wantSeq)
			}
			if len(rep.Markers.Attempts) == 0 {
				t.Fatalf("report markers name no attempts: %+v", rep.Markers)
			}
			c.check(t, f, rep)
			// I06: no recovery path enters a terminal attempt state while
			// ownership is in question — quarantined and interrupted are
			// recoverable, and reconnect/continue move nothing terminal.
			for _, r := range attemptRows(t, f.db, f.runID) {
				switch r.State {
				case "stopped", "succeeded_native", "failed_native":
					t.Fatalf("attempt %q entered terminal state %q through recovery", r.ID, r.State)
				}
			}
		})
	}
}

func TestReconcileUnknownRunInvalid(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_rec_unknown", "att_rec_unknown")
	outcome, err := Reconcile(t.Context(), f.deps(), "run_no_such_run")
	requireCode(t, err, CodeInvalidContract)
	if outcome != "" {
		t.Fatalf("Reconcile = %q, want no outcome for an unknown run", outcome)
	}
	if got := f.examined.Load(); got != 0 {
		t.Fatalf("examination passes = %d, want none: unknown runs fail before any read", got)
	}
	if evs := recoveryEvents(t, f.j, "run_no_such_run"); len(evs) != 0 {
		t.Fatalf("journal holds %d recovery outcomes for an unknown run, want none", len(evs))
	}
	// A run with no attempts has nothing to reconcile either.
	seedRun(t, f.db, "run_rec_empty")
	outcome, err = Reconcile(t.Context(), f.deps(), "run_rec_empty")
	requireCode(t, err, CodeInvalidContract)
	if outcome != "" {
		t.Fatalf("Reconcile = %q, want no outcome for a run with no attempts", outcome)
	}
	if evs := recoveryEvents(t, f.j, "run_rec_empty"); len(evs) != 0 {
		t.Fatalf("journal holds %d recovery outcomes for an attempt-less run, want none", len(evs))
	}
}

func TestSecondPassWithoutFreshEvidenceReplays(t *testing.T) { // FR-2 AC-2.2
	t.Parallel()
	f := newRecoverState(t, "run_rec_replay", "att_rec_replay")
	writeWorkerJSON(t, f.stopState, workers.Identity{SchemaVersion: 1, RunID: f.runID,
		AttemptID: f.attempt, PID: f.pid, StartTime: f.start,
		LaunchToken: f.launchToken, Nonce: "someone-elses-nonce"})
	srv := NewServer(nil)
	if err := srv.RegisterRecover(f.deps()); err != nil {
		t.Fatal(err)
	}
	ctx := f.ctx(t)
	first, err := srv.Dispatch(ctx, Peer{}, recoverIntent(t, "op_rec_1", f.runID))
	requireCode(t, err, CodeOwnershipUnresolved)
	if got := f.examined.Load(); got != 1 {
		t.Fatalf("examination passes = %d, want one", got)
	}
	if len(recoveryEvents(t, f.j, f.runID)) != 1 {
		t.Fatalf("journal holds no recorded pass after the quarantine")
	}
	// The identical intent replays through Execute without re-examining.
	again, err := srv.Dispatch(ctx, Peer{}, recoverIntent(t, "op_rec_1", f.runID))
	requireCode(t, err, CodeOwnershipUnresolved)
	if string(again.Body) != string(first.Body) {
		t.Fatalf("replayed body %s differs from %s", again.Body, first.Body)
	}
	if got := f.examined.Load(); got != 1 {
		t.Fatalf("examination passes = %d after the identical repeat, want one", got)
	}
	// A fresh operation_id with unchanged evidence replays the stored
	// outcome from the one-pass record instead of re-examining.
	replay, err := srv.Dispatch(ctx, Peer{}, recoverIntent(t, "op_rec_2", f.runID))
	requireCode(t, err, CodeOwnershipUnresolved)
	if string(replay.Body) != string(first.Body) {
		t.Fatalf("one-pass replay body %s differs from %s", replay.Body, first.Body)
	}
	if got := f.examined.Load(); got != 1 {
		t.Fatalf("examination passes = %d after the fresh repeat, want one: no new pass", got)
	}
	if evs := recoveryEvents(t, f.j, f.runID); len(evs) != 1 {
		t.Fatalf("journal holds %d recovery outcomes after the replay, want one", len(evs))
	}
	if n := f.launcher.count(); n != 0 {
		t.Fatalf("replays launched %d natives, want zero", n)
	}
	// New spool bytes open a new pass, which examines and records again.
	appendSpoolEvent(t, f.attemptDir(), spoolEvent(f.runID, f.attempt, "attempt.progress", "evt_spool_progress_1"))
	third, err := srv.Dispatch(ctx, Peer{}, recoverIntent(t, "op_rec_3", f.runID))
	requireCode(t, err, CodeOwnershipUnresolved)
	if got := f.examined.Load(); got != 2 {
		t.Fatalf("examination passes = %d after new spool bytes, want two: a new pass", got)
	}
	if evs := recoveryEvents(t, f.j, f.runID); len(evs) != 2 {
		t.Fatalf("journal holds %d recovery outcomes after the new pass, want two", len(evs))
	}
	if decodeReport(t, third.Body).Outcome != RecoverQuarantined {
		t.Fatalf("new pass outcome = %q, want quarantined: the identity is still unverifiable",
			decodeReport(t, third.Body).Outcome)
	}
}

func TestContinuationKeepsLaunchIdentity(t *testing.T) { // FR-2 AC-2.3
	t.Parallel()
	t.Run("continued attempt carries the same launch", func(t *testing.T) {
		t.Parallel()
		f := newRecoverState(t, "run_rec_cont", "att_rec_cont")
		killWorker(t, f, nil, 2)
		res, err := Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_cont", f.runID))
		if err != nil {
			t.Fatalf("recover: %v", err)
		}
		rep := decodeReport(t, res.Body)
		if rep.Outcome != RecoverContinued {
			t.Fatalf("outcome = %q, want continued", rep.Outcome)
		}
		rows := attemptRows(t, f.db, f.runID)
		if len(rows) != 2 || rows[1].Launch != rows[0].Launch {
			t.Fatalf("attempts = %+v, want a continuation carrying the same launch digest", rows)
		}
		if n := f.launcher.count(); n != 1 {
			t.Fatalf("launcher ran %d times, want exactly one launch of the continuation", n)
		}
		got := f.launcher.calls[0]
		if got.AttemptID != rows[1].ID || got.LaunchIDSHA256 != rows[1].Launch || got.RunID != f.runID {
			t.Fatalf("launch = %+v, want the continuation with the same launch digest", got)
		}
	})
	t.Run("forged launch starts nothing", func(t *testing.T) {
		t.Parallel()
		f := newRecoverState(t, "run_rec_forge", "att_rec_forge")
		// A live worker whose every identity field matches except the
		// launch token it claims.
		writeWorkerJSON(t, f.stopState, workers.Identity{SchemaVersion: 1, RunID: f.runID,
			AttemptID: f.attempt, PID: f.pid, StartTime: f.start,
			LaunchToken: "tok_forged", Nonce: f.rawNonce})
		_, err := Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_forge", f.runID))
		requireCode(t, err, CodeRevisionConflict)
		if rows := attemptRows(t, f.db, f.runID); len(rows) != 1 {
			t.Fatalf("run holds %d attempts, want one: a forgery starts nothing", len(rows))
		}
		if got := attemptState(t, f.db, f.attempt); got != "running" {
			t.Fatalf("attempt state = %q, want running: a forgery moves nothing", got)
		}
		if n := f.launcher.count(); n != 0 {
			t.Fatalf("launcher ran %d times for a forgery, want zero", n)
		}
		if evs := recoveryEvents(t, f.j, f.runID); len(evs) != 0 {
			t.Fatalf("journal holds %d recovery outcomes for a forgery, want none: refusals record nothing", len(evs))
		}
	})
}

func TestRecoveryNeverReplaysEffects(t *testing.T) { // I12 (v2 §6.2)
	t.Parallel()
	native := 4242
	cases := []struct {
		name    string
		live    bool
		settled string // "", "journaled" or "spooled"
		uncert  bool
	}{
		{name: "dead worker with ambiguous native effect is uncertain", live: false, uncert: true},
		{name: "live worker with ambiguous native effect is uncertain", live: true, uncert: true},
		{name: "journaled native result settles the effect", live: false, settled: "journaled"},
		{name: "spooled native result settles the effect", live: false, settled: "spooled"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newRecoverState(t, "run_rec_fx", "att_rec_fx")
			if c.live {
				appendNativeLaunched(t, f.j, f.runID, f.attempt, f.pid, f.start, &native, 2)
			} else {
				killWorker(t, f, &native, 2)
			}
			switch c.settled {
			case "journaled":
				appendNativeResult(t, f.j, f.runID, f.attempt, 3)
			case "spooled":
				appendSpoolEvent(t, f.attemptDir(), spoolEvent(f.runID, f.attempt,
					"attempt.native_result", "evt_spool_nresult_1"))
			}
			res, err := Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_fx", f.runID))
			if c.uncert {
				requireCode(t, err, CodeExternalEffectUncertain)
				if string(CodeExternalEffectUncertain) != string(v2contract.CodeExternalEffectUncertain) {
					t.Fatalf("control code %q differs from the v2 catalogue string", CodeExternalEffectUncertain)
				}
				if d := v2contract.CodeExternalEffectUncertain.DefaultDisposition(); d != v2contract.DispositionAfterReconciliation {
					t.Fatalf("external_effect_uncertain disposition = %q, want after_reconciliation", d)
				}
				rep := decodeReport(t, res.Body)
				if rep.Outcome != RecoverQuarantined || rep.Reason != "external_effect_uncertain" {
					t.Fatalf("report = %+v, want quarantined with the uncertain reason", rep)
				}
				if got := attemptState(t, f.db, f.attempt); got != "quarantined" {
					t.Fatalf("attempt state = %q, want quarantined", got)
				}
				if rows := attemptRows(t, f.db, f.runID); len(rows) != 1 {
					t.Fatalf("run holds %d attempts, want one: uncertainty starts nothing", len(rows))
				}
			} else {
				if err != nil {
					t.Fatalf("recover: %v", err)
				}
				if rep := decodeReport(t, res.Body); rep.Outcome != RecoverContinued {
					t.Fatalf("outcome = %q, want continued: the settled effect clears the way", rep.Outcome)
				}
			}
			if n := f.launcher.count(); c.uncert && n != 0 {
				t.Fatalf("launcher ran %d times under uncertainty, want zero: never relaunch by replay", n)
			}
		})
	}
}

func TestConcurrentFreshRecoversRecordOnce(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_rec_race", "att_rec_race")
	killWorker(t, f, nil, 2)
	const n = 4
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Go(func() {
			_, errs[i] = Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{},
				recoverIntent(t, fmt.Sprintf("op_rec_race_%d", i), f.runID))
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("recover %d: %v", i, err)
		}
	}
	// One winner examined and recorded; every loser replayed its record
	// instead of admitting its own continuation.
	if got := f.examined.Load(); got != 1 {
		t.Fatalf("examination passes = %d, want one", got)
	}
	if evs := recoveryEvents(t, f.j, f.runID); len(evs) != 1 {
		t.Fatalf("journal holds %d recovery outcomes, want one", len(evs))
	}
	if rows := attemptRows(t, f.db, f.runID); len(rows) != 2 {
		t.Fatalf("run holds %d attempts, want the recovered one plus one continuation", len(rows))
	}
	if n := f.launcher.count(); n != 1 {
		t.Fatalf("launcher ran %d times, want exactly one: replays never relaunch", n)
	}
}

func TestQuarantineHealsOnFreshEvidence(t *testing.T) { // FR-2 AC-2.2
	t.Parallel()
	f := newRecoverState(t, "run_rec_heal", "att_rec_heal")
	// First the worker is alive but unverifiable: the pass quarantines.
	writeWorkerJSON(t, f.stopState, workers.Identity{SchemaVersion: 1, RunID: f.runID,
		AttemptID: f.attempt, PID: f.pid, StartTime: f.start,
		LaunchToken: f.launchToken, Nonce: "someone-elses-nonce"})
	_, err := Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_heal_1", f.runID))
	requireCode(t, err, CodeOwnershipUnresolved)
	if got := attemptState(t, f.db, f.attempt); got != "quarantined" {
		t.Fatalf("attempt state = %q, want quarantined", got)
	}
	// Then the unverifiable worker dies with a clean workspace: the
	// liveness flip is fresh evidence, so the next pass continues.
	deadStart := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	appendNativeLaunched(t, f.j, f.runID, f.attempt, 1<<30, deadStart, nil, 2)
	writeWorkerJSON(t, f.stopState, workers.Identity{SchemaVersion: 1, RunID: f.runID,
		AttemptID: f.attempt, PID: 1 << 30, StartTime: deadStart,
		LaunchToken: f.launchToken, Nonce: f.rawNonce})
	res, err := Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_heal_2", f.runID))
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if rep := decodeReport(t, res.Body); rep.Outcome != RecoverContinued {
		t.Fatalf("outcome = %q, want continued after the worker provably died", rep.Outcome)
	}
	if got := f.examined.Load(); got != 2 {
		t.Fatalf("examination passes = %d, want two: fresh evidence opened a new pass", got)
	}
	rows := attemptRows(t, f.db, f.runID)
	if len(rows) != 2 || rows[0].State != "quarantined" || rows[1].State != "launching" {
		t.Fatalf("attempts = %+v, want the quarantined verdict kept beside its continuation", rows)
	}
	if evs := recoveryEvents(t, f.j, f.runID); len(evs) != 2 {
		t.Fatalf("journal holds %d recovery outcomes, want one per pass", len(evs))
	}
}

// writeWorkerJSONAt writes a worker.json into an arbitrary attempt
// directory: continuations live beside, not inside, the recovered attempt.
func writeWorkerJSONAt(t *testing.T, dir string, id workers.Identity) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worker.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestContinuationHandsOffToLaunchedWorker(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_rec_handoff", "att_rec_handoff")
	killWorker(t, f, nil, 2)
	res, err := Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_ho_1", f.runID))
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	cont := decodeReport(t, res.Body).NewAttemptID
	if cont == "" {
		t.Fatalf("no continuation was admitted")
	}
	// While the continuation has journaled nothing it stands as admitted.
	replay, err := Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_ho_2", f.runID))
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if rep := decodeReport(t, replay.Body); rep.Outcome != RecoverContinued || rep.NewAttemptID != cont {
		t.Fatalf("handoff replay = %+v, want the stored continuation", rep)
	}
	if got := f.examined.Load(); got != 1 {
		t.Fatalf("examination passes = %d, want one: the handoff replays", got)
	}
	// Its first journaled evidence — pin plus launch — opens its first
	// examination, which reconnects to the live worker.
	rawB := "test-raw-nonce-b"
	appendPin(t, f.j, f.runID, cont, f.version, workers.NonceDigest(rawB), 1)
	appendLaunched(t, f.j, f.runID, cont, f.pid, f.start, 1)
	writeWorkerJSONAt(t, workers.AttemptDir(f.dir, f.runID, cont), workers.Identity{
		SchemaVersion: 1, RunID: f.runID, AttemptID: cont, PID: f.pid, StartTime: f.start,
		LaunchToken: f.launchToken, Nonce: rawB})
	res, err = Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_ho_3", f.runID))
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	rep := decodeReport(t, res.Body)
	if rep.Outcome != RecoverReconnected || rep.AttemptID != cont {
		t.Fatalf("report = %+v, want a reconnect to the launched continuation", rep)
	}
	if got := f.examined.Load(); got != 2 {
		t.Fatalf("examination passes = %d, want two", got)
	}
	if evs := recoveryEvents(t, f.j, f.runID); len(evs) != 2 {
		t.Fatalf("journal holds %d recovery outcomes, want one per pass", len(evs))
	}
}

func TestRecoverParamsStrict(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_rec_params", "att_rec_params")
	h := RecoverHandler(f.deps())
	for name, params := range map[string]string{
		"unknown field": `{"run_id":"r","launch_id":"x"}`,
		"empty run":     `{"run_id":""}`,
		"not an object": `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h(t.Context(), Peer{}, Intent{OperationID: "op_rec_params", Method: "recover", Params: json.RawMessage(params)})
			requireCode(t, err, CodeInvalidContract)
		})
	}
	_, err := h(t.Context(), Peer{}, Intent{OperationID: "op_rec_params", Method: "recover"})
	requireCode(t, err, CodeInvalidContract)
}

func TestRecoverLedgerUnavailablePersistenceUnavailable(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_rec_closed", "att_rec_closed")
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := Reconcile(t.Context(), f.deps(), f.runID)
	requireCode(t, err, CodePersistenceUnavailable)
	if evs := recoveryEvents(t, f.j, f.runID); len(evs) != 0 {
		t.Fatalf("journal holds %d recovery outcomes after a ledger failure, want none", len(evs))
	}
}

func TestRegisterRecoverDuplicate(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_rec_dup", "att_rec_dup")
	srv := NewServer(nil)
	if err := srv.RegisterRecover(f.deps()); err != nil {
		t.Fatal(err)
	}
	if err := srv.RegisterRecover(f.deps()); err == nil {
		t.Fatalf("second RegisterRecover succeeded, want the duplicate named")
	}
	// recover stays out of the served set until Task 4 folds it in with
	// its support-matrix rows (the Task 1 stop precedent).
	for _, m := range servedMethods() {
		if m == "recover" {
			t.Fatalf("NewSupervisorServer serves recover before Task 4 wires it")
		}
	}
}

func TestRecoveryReportGolden(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_rec_golden", "att_rec_golden")
	res, err := Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_golden", f.runID))
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	rep := decodeReport(t, res.Body)
	// The stable prefix of the report is golden; markers vary with file
	// metadata, so they are asserted structurally below.
	stable, err := json.Marshal(struct {
		RunID           string         `json:"run_id"`
		AttemptID       string         `json:"attempt_id"`
		Outcome         RecoverOutcome `json:"outcome"`
		NewAttemptID    string         `json:"new_attempt_id,omitempty"`
		CandidateCommit string         `json:"candidate_commit,omitempty"`
		Reason          string         `json:"reason,omitempty"`
	}{rep.RunID, rep.AttemptID, rep.Outcome, rep.NewAttemptID, rep.CandidateCommit, rep.Reason})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"run_id":"run_rec_golden","attempt_id":"att_rec_golden","outcome":"reconnected"}`; string(stable) != want {
		t.Fatalf("report prefix = %s, want %s", stable, want)
	}
	if rep.Markers.JournalSeq < 1 || rep.Markers.SpoolBytes < -1 || rep.Markers.IdentityStat == "" || rep.Markers.LiveNow == "" {
		t.Fatalf("report markers are incomplete: %+v", rep.Markers)
	}
	if len(rep.Markers.Attempts) != 1 || rep.Markers.Attempts[0].AttemptID != f.attempt {
		t.Fatalf("report markers name %+v, want the recovered attempt", rep.Markers.Attempts)
	}
	// The journaled outcome event carries the same report the intent
	// returned: one shape, golden once.
	evs := recoveryEvents(t, f.j, f.runID)
	if len(evs) != 1 || string(evs[0].Payload) != string(res.Body) {
		t.Fatalf("journaled outcome differs from the returned report")
	}
}
