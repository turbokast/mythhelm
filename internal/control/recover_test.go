package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

func TestRecoverRefusedWhileALiveOwnerHoldsTheRun(t *testing.T) { // N2, I18, I23, v2 §6.4
	t.Parallel()
	f := newRecoverState(t, "run_rec_owned", "att_rec_owned")
	killWorker(t, f, nil, 2)
	holdRunOwner(t, f.dir, f.runID)
	_, err := Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_owned", f.runID))
	requireCode(t, err, CodeOwnershipUnresolved)
	if got := attemptState(t, f.db, f.attempt); got != "running" {
		t.Fatalf("attempt state = %q, want running: nothing is written under another owner", got)
	}
	if rows := attemptRows(t, f.db, f.runID); len(rows) != 1 {
		t.Fatalf("run holds %d attempts, want one: no continuation under another owner", len(rows))
	}
	if evs := recoveryEvents(t, f.j, f.runID); len(evs) != 0 {
		t.Fatalf("journal holds %d recovery outcomes, want none", len(evs))
	}
	if n := f.launcher.count(); n != 0 {
		t.Fatalf("launcher ran %d times, want zero", n)
	}
}

func TestRecoverFailsClosedWithoutARunDirectory(t *testing.T) { // I23, v2 §6.4
	t.Parallel()
	f := newRecoverState(t, "run_rec_nodir", "att_rec_nodir")
	killWorker(t, f, nil, 2)
	if err := os.RemoveAll(filepath.Join(f.dir, "runs", f.runID)); err != nil {
		t.Fatal(err)
	}
	_, err := Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_nodir", f.runID))
	requireCode(t, err, CodeOwnershipUnresolved)
	if rows := attemptRows(t, f.db, f.runID); len(rows) != 1 {
		t.Fatalf("run holds %d attempts, want one: no continuation without ownership", len(rows))
	}
	if n := f.launcher.count(); n != 0 {
		t.Fatalf("launcher ran %d times, want zero", n)
	}
}

func TestRecoveryNeverReplaysEffects(t *testing.T) { // I12 (v2 §6.2)
	t.Parallel()
	native := 4242
	cases := []struct {
		name    string
		live    bool
		settled string // "", "journaled" or "spooled"
		uncert  bool
		adopt   bool
	}{
		{name: "dead worker with ambiguous native effect is uncertain", live: false, uncert: true},
		{name: "live identity-matched worker with a running native is adopted", live: true, adopt: true},
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
			switch {
			case c.uncert:
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
			case c.adopt:
				if err != nil {
					t.Fatalf("recover: %v", err)
				}
				if rep := decodeReport(t, res.Body); rep.Outcome != RecoverReconnected {
					t.Fatalf("outcome = %q, want reconnected: the same live worker is adopted (AC-2.1)", rep.Outcome)
				}
				if got := attemptState(t, f.db, f.attempt); got != "running" {
					t.Fatalf("attempt state = %q, want running: adoption moves nothing", got)
				}
				if n := f.launcher.count(); n != 0 {
					t.Fatalf("launcher ran %d times for an adopted worker, want zero", n)
				}
			default:
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
	// recover is folded into the served set with its support-matrix rows.
	if !slices.Contains(servedMethods(), "recover") {
		t.Fatal("NewSupervisorServer does not serve recover")
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
	if rep.Markers.JournalSeq < 1 || rep.Markers.SpoolBytes < -1 || rep.Markers.IdentityStat == "" ||
		rep.Markers.LiveNow == "" || rep.Markers.ObservedLive == "" {
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

// spoolLaunchedEvent builds a spool-only attempt.launched envelope with a
// parseable payload, naming a native PID when native is non-nil.
func spoolLaunchedEvent(t *testing.T, runID, attemptID, eventID string, pid int, start time.Time, native *int) journal.Event {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"worker_pid": pid, "worker_start_time": start, "native_pid": native, "native_pgid": nil})
	if err != nil {
		t.Fatal(err)
	}
	ev := spoolEvent(runID, attemptID, "attempt.launched", eventID)
	ev.Payload = payload
	return ev
}

func TestScanSpoolFactsSkipsAcknowledgedLines(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_rec_spooloff", "att_rec_spooloff")
	appendSpoolEvent(t, f.attemptDir(), spoolEvent(f.runID, f.attempt,
		"attempt.native_result", "evt_spool_ack_1"))
	spool := filepath.Join(f.attemptDir(), "spool.jsonl")
	st, err := os.Stat(spool)
	if err != nil {
		t.Fatal(err)
	}
	acked := st.Size()
	appendSpoolEvent(t, f.attemptDir(), spoolLaunchedEvent(t, f.runID, f.attempt,
		"evt_spool_unack_1", f.pid, f.start, nil))
	all, ok := scanSpoolFacts(spool, f.attempt, 0)
	if !ok || len(all) != 2 {
		t.Fatalf("scan from zero = %d facts ok=%v, want the acknowledged result plus the unacknowledged launch", len(all), ok)
	}
	unacked, ok := scanSpoolFacts(spool, f.attempt, acked)
	if !ok || len(unacked) != 1 || !unacked[0].launched {
		t.Fatalf("scan past the acknowledged offset = %+v ok=%v, want only the unacknowledged launch", unacked, ok)
	}
}

func TestScanSpoolFactsRereadsAfterTruncation(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_rec_spooltrunc", "att_rec_spooltrunc")
	spool := filepath.Join(f.attemptDir(), "spool.jsonl")
	appendSpoolEvent(t, f.attemptDir(), spoolLaunchedEvent(t, f.runID, f.attempt,
		"evt_spool_trunc_1", f.pid, f.start, nil))
	first, ok := scanSpoolFacts(spool, f.attempt, 0)
	if !ok || len(first) != 1 || !first[0].launched {
		t.Fatalf("first scan = %+v ok=%v, want the launched fact", first, ok)
	}
	st, err := os.Stat(spool)
	if err != nil {
		t.Fatal(err)
	}
	consumed := st.Size()
	// The worker rotates the spool: a shorter file holding a fresh
	// native result, with the old offset past its end.
	line, err := json.Marshal(spoolEvent(f.runID, f.attempt, "attempt.native_result", "evt_spool_trunc_2"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spool, append(line, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(spool); err != nil || st.Size() >= consumed {
		t.Fatalf("rotated spool size breaks the fixture: want it below the old offset %d", consumed)
	}
	second, ok := scanSpoolFacts(spool, f.attempt, consumed)
	if !ok || len(second) != 1 || second[0].launched {
		t.Fatalf("scan after truncation = %+v ok=%v, want the fresh result re-read from the start", second, ok)
	}
}

func TestRecoveryIgnoresAcknowledgedSpool(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_rec_spoolack", "att_rec_spoolack")
	native := 4242
	killWorker(t, f, &native, 2)
	appendSpoolEvent(t, f.attemptDir(), spoolEvent(f.runID, f.attempt,
		"attempt.native_result", "evt_spool_ackres_1"))
	// Ingest journaled the spool up to its end: the spooled result is
	// acknowledged, so it must not settle the journaled native launch.
	st, err := os.Stat(filepath.Join(f.attemptDir(), "spool.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE attempts SET spool_offset = ? WHERE attempt_id = ?`,
		st.Size(), f.attempt); err != nil {
		t.Fatal(err)
	}
	res, err := Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_spoolack", f.runID))
	requireCode(t, err, CodeExternalEffectUncertain)
	if rep := decodeReport(t, res.Body); rep.Outcome != RecoverQuarantined {
		t.Fatalf("outcome = %q, want quarantined: the acknowledged result settles nothing", rep.Outcome)
	}
	if n := f.launcher.count(); n != 0 {
		t.Fatalf("launcher ran %d times under uncertainty, want zero", n)
	}
}

func TestOversizeSpoolQuarantinesUnverifiable(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_rec_spoolbig", "att_rec_spoolbig")
	killWorker(t, f, nil, 2)
	spool := filepath.Join(f.attemptDir(), "spool.jsonl")
	if err := os.WriteFile(spool, bytes.Repeat([]byte("x"), maxRecoverSpoolBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if facts, ok := scanSpoolFacts(spool, f.attempt, 0); ok || facts != nil {
		t.Fatalf("oversize scan = %+v ok=%v, want no facts and ok=false", facts, ok)
	}
	res, err := Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_spoolbig", f.runID))
	requireCode(t, err, CodeOwnershipUnresolved)
	rep := decodeReport(t, res.Body)
	if rep.Outcome != RecoverQuarantined || rep.Reason != "spool_unverifiable" {
		t.Fatalf("report = %+v, want quarantined with the spool reason: truncation is never silent", rep)
	}
	if got := attemptState(t, f.db, f.attempt); got != "quarantined" {
		t.Fatalf("attempt state = %q, want quarantined", got)
	}
	if n := f.launcher.count(); n != 0 {
		t.Fatalf("launcher ran %d times over unverifiable spool, want zero", n)
	}
	// The quarantine replays without re-examining while the spool stays
	// over the bound.
	again, err := Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_spoolbig_2", f.runID))
	requireCode(t, err, CodeOwnershipUnresolved)
	if string(again.Body) != string(res.Body) {
		t.Fatalf("replayed body %s differs from %s", again.Body, res.Body)
	}
	if got := f.examined.Load(); got != 1 {
		t.Fatalf("examination passes = %d after the replay, want one", got)
	}
}

func TestObservedLiveStrangerBlocksContinuation(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_rec_stranger", "att_rec_stranger")
	killWorker(t, f, nil, 2)
	// The journaled worker is dead, but the directory names a different
	// live worker (this test process under another nonce).
	writeWorkerJSON(t, f.stopState, workers.Identity{SchemaVersion: 1, RunID: f.runID,
		AttemptID: f.attempt, PID: f.pid, StartTime: f.start,
		LaunchToken: f.launchToken, Nonce: "someone-elses-nonce"})
	outcome, err := f.reconcile(t)
	requireCode(t, err, CodeOwnershipUnresolved)
	if outcome != RecoverQuarantined {
		t.Fatalf("Reconcile = %q, want quarantined: no continuation beside a live stranger", outcome)
	}
	if rows := attemptRows(t, f.db, f.runID); len(rows) != 1 {
		t.Fatalf("run holds %d attempts, want one: a stranger blocks admission", len(rows))
	}
	if n := f.launcher.count(); n != 0 {
		t.Fatalf("Reconcile launched %d natives beside a live stranger, want zero", n)
	}
}

// startSleeper spawns this test binary as a sleeper child that blocks
// until killed (the TestMain branch beside the lockholder), returning it
// once the process table agrees it is alive. The child is killed and
// reaped on cleanup.
func startSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0]) //nolint:gosec // G702: re-executes this test binary; TestMain selects the sleeper branch
	cmd.Env = append(os.Environ(), "MYTHHELM_TEST_HELPER=sleeper")
	var childOut, childErr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &childOut, &childErr
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting sleeper subprocess: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := workers.ProcessStartTime(cmd.Process.Pid); err == nil {
			return cmd
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("sleeper subprocess never appeared alive\nstdout: %s\nstderr: %s",
		childOut.String(), childErr.String())
	return nil
}

// killSleeper kills the sleeper child and waits for the process table to
// agree it is gone (the drain deadPID precedent).
func killSleeper(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	pid := cmd.Process.Pid
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("killing the sleeper: %v", err)
	}
	_ = cmd.Wait()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := workers.ProcessStartTime(pid); errors.Is(err, workers.ErrNoProcess) {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("sleeper pid %d never left the process table", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRecoveryProceedsWhenStrangerDies(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_rec_healstranger", "att_rec_healstranger")
	killWorker(t, f, nil, 2)
	sleeper := startSleeper(t)
	strangerStart, err := workers.ProcessStartTime(sleeper.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	writeWorkerJSON(t, f.stopState, workers.Identity{SchemaVersion: 1, RunID: f.runID,
		AttemptID: f.attempt, PID: sleeper.Process.Pid, StartTime: strangerStart,
		LaunchToken: f.launchToken, Nonce: "someone-elses-nonce"})
	identityPath := filepath.Join(f.attemptDir(), "worker.json")
	before, err := os.ReadFile(identityPath) //nolint:gosec // G304: a test temp path
	if err != nil {
		t.Fatal(err)
	}
	// While the stranger lives, the pass quarantines without admitting.
	first, err := Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_hs_1", f.runID))
	requireCode(t, err, CodeOwnershipUnresolved)
	if rep := decodeReport(t, first.Body); rep.Outcome != RecoverQuarantined {
		t.Fatalf("first outcome = %q, want quarantined beside the live stranger", rep.Outcome)
	}
	if rows := attemptRows(t, f.db, f.runID); len(rows) != 1 {
		t.Fatalf("run holds %d attempts, want one: a stranger blocks admission", len(rows))
	}
	// The stranger dies with its file untouched: the liveness flip alone
	// re-opens the pass, which continues.
	killSleeper(t, sleeper)
	after, err := os.ReadFile(identityPath) //nolint:gosec // G304: a test temp path
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("worker.json changed across the stranger's death: the flip must be liveness alone")
	}
	second, err := Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_hs_2", f.runID))
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if rep := decodeReport(t, second.Body); rep.Outcome != RecoverContinued {
		t.Fatalf("second outcome = %q, want continued once the stranger is gone", rep.Outcome)
	}
	if got := f.examined.Load(); got != 2 {
		t.Fatalf("examination passes = %d, want two: the flip opened a new pass", got)
	}
	if n := f.launcher.count(); n != 1 {
		t.Fatalf("launcher ran %d times, want the one continuation launch", n)
	}
	// The two recorded passes differ only in the observed-liveness
	// marker (plus the second pass's own admission): nothing else moved.
	evs := recoveryEvents(t, f.j, f.runID)
	if len(evs) != 2 {
		t.Fatalf("journal holds %d recovery outcomes, want one per pass", len(evs))
	}
	strip := func(m RecoveryMarkers) RecoveryMarkers {
		m.Attempts, m.ObservedLive = nil, ""
		return m
	}
	ma, merr := json.Marshal(strip(decodeReport(t, evs[0].Payload).Markers))
	mb, merr2 := json.Marshal(strip(decodeReport(t, evs[1].Payload).Markers))
	if merr != nil || merr2 != nil {
		t.Fatalf("encoding markers: %v %v", merr, merr2)
	}
	if !bytes.Equal(ma, mb) {
		t.Fatalf("stripped markers differ:\n%s\n%s", ma, mb)
	}
	if got := decodeReport(t, evs[0].Payload).Markers.ObservedLive; len(got) < 5 || got[:5] != "live:" {
		t.Fatalf("first observed_live = %q, want the live stranger", got)
	}
	if got := decodeReport(t, evs[1].Payload).Markers.ObservedLive; got != "gone" {
		t.Fatalf("second observed_live = %q, want gone", got)
	}
}

func TestEventlessContinuationExpiresToQuarantine(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_rec_expire", "att_rec_expire")
	killWorker(t, f, nil, 2)
	// An admitted continuation whose worker never journaled anything —
	// not even its launch — with a continued record older than the
	// newborn bound. (The journal is append-only, so the aged record is
	// appended old, never backdated.)
	cont := f.attempt + "-c2"
	if _, err := f.db.Exec(`INSERT INTO attempts (attempt_id, run_id, task_id, attempt_number, state,
		launch_token_sha256, workspace_path) VALUES (?, ?, 'task_1', 2, 'launching', ?, '/tmp/ws')`,
		cont, f.runID, shahex(f.launchToken)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE attempts SET state = 'interrupted' WHERE attempt_id = ?`, f.attempt); err != nil {
		t.Fatal(err)
	}
	aged, err := json.Marshal(RecoveryReport{RunID: f.runID, AttemptID: f.attempt,
		Outcome: RecoverContinued, NewAttemptID: cont})
	if err != nil {
		t.Fatal(err)
	}
	decidedAt := time.Now().Add(-(MaxRecoveryContinuationAge + time.Minute)).UTC()
	if err := f.j.Append(t.Context(), journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: "evt_recovery_aged_1",
		RunID: f.runID, TaskID: "task_1", AttemptID: f.attempt, ProducerID: "ctl_recover_" + f.runID,
		ProducerSequence: 1, Generation: 1, ObservedAt: decidedAt,
		Type: EventRecoveryDecided, Payload: aged,
	}, nil); err != nil {
		t.Fatal(err)
	}
	res, err := Execute(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_exp_1", f.runID))
	requireCode(t, err, CodeOwnershipUnresolved)
	rep := decodeReport(t, res.Body)
	if rep.Outcome != RecoverQuarantined || rep.Reason != "ownership_unresolved" {
		t.Fatalf("report = %+v, want the expired continuation quarantined, not replayed forever", rep)
	}
	if got := f.examined.Load(); got != 1 {
		t.Fatalf("examination passes = %d, want one: the expiry examined instead of replaying", got)
	}
	if n := f.launcher.count(); n != 0 {
		t.Fatalf("launcher ran %d times, want zero: the expiry relaunches nothing", n)
	}
	if got := attemptState(t, f.db, cont); got != "quarantined" {
		t.Fatalf("continuation state = %q, want quarantined", got)
	}
	if evs := recoveryEvents(t, f.j, f.runID); len(evs) != 2 {
		t.Fatalf("journal holds %d recovery outcomes, want the aged record plus the new pass", len(evs))
	}
}

// flakyLauncher fails its first launch, then records the rest: a spawn
// failure after the admission committed.
type flakyLauncher struct {
	mu     sync.Mutex
	calls  int
	fails  int
	bodies []RecoveryLaunch
}

func (l *flakyLauncher) launch(_ context.Context, r RecoveryLaunch) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	if l.calls == 1 {
		l.fails++
		return errors.New("spawn failed")
	}
	l.bodies = append(l.bodies, r)
	return nil
}

// TestRecoverRetriesFailedLaunch pins the durable post-commit handoff: a
// spawn failure after the admission committed leaves exactly one admitted
// continuation, and the same operation_id relaunches it instead of
// admitting another or replaying continued without a worker.
func TestRecoverRetriesFailedLaunch(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_rec_flaky", "att_rec_flaky")
	killWorker(t, f, nil, 2)
	flaky := &flakyLauncher{}
	deps := f.deps()
	deps.Launch = flaky.launch
	h := RecoverHandler(deps)
	in := recoverIntent(t, "op_rec_flaky", f.runID)
	if _, err := ExecuteLong(f.ctx(t), h, Peer{}, in); err == nil {
		t.Fatal("recover with a failing launcher returned no error")
	} else if _, ok := errors.AsType[*Error](err); ok {
		t.Fatalf("err = %v, want a transient failure, not a stored one", err)
	}
	rep, err := func() (RecoveryReport, error) {
		res, err := ExecuteLong(f.ctx(t), h, Peer{}, in)
		if err != nil {
			return RecoveryReport{}, err
		}
		return decodeReport(t, res.Body), nil
	}()
	if err != nil {
		t.Fatalf("retry after the spawn failure: %v", err)
	}
	if rep.Outcome != RecoverContinued {
		t.Fatalf("outcome = %q, want continued", rep.Outcome)
	}
	flaky.mu.Lock()
	calls, fails, launched := flaky.calls, flaky.fails, len(flaky.bodies)
	flaky.mu.Unlock()
	if calls != 2 || fails != 1 || launched != 1 {
		t.Fatalf("launcher calls = %d (%d fails, %d recorded), want the failure plus one relaunch", calls, fails, launched)
	}
	if rows := attemptRows(t, f.db, f.runID); len(rows) != 2 {
		t.Fatalf("run holds %d attempts, want the recovered one plus one continuation: the retry replayed, not re-admitted", len(rows))
	}
	if got := f.examined.Load(); got != 1 {
		t.Fatalf("examination passes = %d, want one: the retry replayed the record", got)
	}
	if evs := recoveryEvents(t, f.j, f.runID); len(evs) != 1 {
		t.Fatalf("journal holds %d recovery outcomes, want one", len(evs))
	}
}

// TestRecoverAdoptsHandshakeContinuation pins the handshake/recover split:
// the handshake records a continued outcome without spawning, and a later
// recover adopts the handshake-admitted continuation by launching it —
// unless a worker already runs there, in which case it launches nothing.
func TestRecoverAdoptsHandshakeContinuation(t *testing.T) {
	t.Parallel()
	t.Run("unspawned continuation launches", func(t *testing.T) {
		t.Parallel()
		f := newRecoverState(t, "run_rec_adopt", "att_rec_adopt")
		killWorker(t, f, nil, 2)
		_, rep, err := Reconnect(t.Context(), ReconnectDeps{DB: f.db, StateDir: f.dir}, f.runID)
		if err != nil {
			t.Fatalf("handshake: %v", err)
		}
		if rep.Outcome != RecoverContinued || rep.NewAttemptID == "" {
			t.Fatalf("handshake report = %+v, want an admitted continuation", rep)
		}
		res, err := ExecuteLong(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_adopt", f.runID))
		if err != nil {
			t.Fatalf("adopting recover: %v", err)
		}
		if got := decodeReport(t, res.Body); got.Outcome != RecoverContinued || got.NewAttemptID != rep.NewAttemptID {
			t.Fatalf("adopting report = %+v, want the handshake's continuation", got)
		}
		if n := f.launcher.count(); n != 1 {
			t.Fatalf("launcher ran %d times, want exactly one adoption launch", n)
		}
		if got := f.examined.Load(); got != 0 {
			t.Fatalf("examination passes = %d, want zero: the adoption replays the handshake's record", got)
		}
		if evs := recoveryEvents(t, f.j, f.runID); len(evs) != 1 {
			t.Fatalf("journal holds %d recovery outcomes, want the handshake's one", len(evs))
		}
	})
	t.Run("spawned continuation launches nothing", func(t *testing.T) {
		t.Parallel()
		f := newRecoverState(t, "run_rec_adopted", "att_rec_adopted")
		killWorker(t, f, nil, 2)
		_, rep, err := Reconnect(t.Context(), ReconnectDeps{DB: f.db, StateDir: f.dir}, f.runID)
		if err != nil {
			t.Fatalf("handshake: %v", err)
		}
		// A worker runs in the continuation directory: the adoption
		// must not spawn a second.
		contDir := workers.AttemptDir(f.dir, f.runID, rep.NewAttemptID)
		if err := os.MkdirAll(contDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(contDir, "spool.jsonl"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		res, err := ExecuteLong(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_adopted", f.runID))
		if err != nil {
			t.Fatalf("adopting recover: %v", err)
		}
		if got := decodeReport(t, res.Body); got.Outcome != RecoverContinued {
			t.Fatalf("adopting report = %+v, want continued", got)
		}
		if n := f.launcher.count(); n != 0 {
			t.Fatalf("launcher ran %d times beside a live spool, want zero", n)
		}
	})
	t.Run("dead worker identity without a spool launches nothing", func(t *testing.T) {
		t.Parallel()
		f := newRecoverState(t, "run_rec_deadid", "att_rec_deadid")
		killWorker(t, f, nil, 2)
		_, rep, err := Reconnect(t.Context(), ReconnectDeps{DB: f.db, StateDir: f.dir}, f.runID)
		if err != nil {
			t.Fatalf("handshake: %v", err)
		}
		// A worker wrote its identity here and is gone, its spool removed:
		// it may have launched the native, so nothing relaunches (I12).
		sleeper := startSleeper(t)
		start, err := workers.ProcessStartTime(sleeper.Process.Pid)
		if err != nil {
			t.Fatal(err)
		}
		killSleeper(t, sleeper)
		writeWorkerJSONAt(t, workers.AttemptDir(f.dir, f.runID, rep.NewAttemptID), workers.Identity{SchemaVersion: 1,
			RunID: f.runID, AttemptID: rep.NewAttemptID, PID: sleeper.Process.Pid, StartTime: start})
		if _, err := ExecuteLong(f.ctx(t), RecoverHandler(f.deps()), Peer{}, recoverIntent(t, "op_rec_deadid", f.runID)); err != nil {
			t.Fatalf("adopting recover: %v", err)
		}
		if n := f.launcher.count(); n != 0 {
			t.Fatalf("launcher ran %d times beside a dead worker's identity, want zero", n)
		}
	})
}

func TestValidPIDBounds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		pid     int
		windows bool
		want    bool
	}{
		{-1, false, false},
		{-1, true, false},
		{0, false, false},
		{0, true, false},
		{1, false, true},
		{1, true, true},
		{4242, false, true},
		{4242, true, true},
		{math.MaxInt32, false, true},
		{math.MaxInt32, true, true},
		{math.MaxInt32 + 1, false, false},
		{math.MaxInt32 + 1, true, true},
		{math.MaxUint32, false, false},
		{math.MaxUint32, true, true},
		{math.MaxUint32 + 1, false, false},
		{math.MaxUint32 + 1, true, false},
	}
	for _, c := range cases {
		if got := validPIDFor(c.pid, c.windows); got != c.want {
			t.Errorf("validPIDFor(%d, windows=%v) = %v, want %v", c.pid, c.windows, got, c.want)
		}
	}
	// The platform wrapper agrees with the table on this platform.
	if validPID(0) || validPID(-5) || !validPID(1) {
		t.Fatalf("validPID disagrees on 0/-5/1")
	}
}

func TestInvalidPIDsProbeUnknown(t *testing.T) {
	t.Parallel()
	t.Run("invalid observed PID quarantines", func(t *testing.T) {
		t.Parallel()
		f := newRecoverState(t, "run_rec_badpid_obs", "att_rec_badpid_obs")
		killWorker(t, f, nil, 2)
		deadStart := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		writeWorkerJSON(t, f.stopState, workers.Identity{SchemaVersion: 1, RunID: f.runID,
			AttemptID: f.attempt, PID: -1, StartTime: deadStart,
			LaunchToken: f.launchToken, Nonce: "someone-elses-nonce"})
		outcome, err := f.reconcile(t)
		requireCode(t, err, CodeOwnershipUnresolved)
		if outcome != RecoverQuarantined {
			t.Fatalf("Reconcile = %q, want quarantined: a garbage observed PID proves nothing", outcome)
		}
		if rows := attemptRows(t, f.db, f.runID); len(rows) != 1 {
			t.Fatalf("run holds %d attempts, want one: no admission on an unprovable probe", len(rows))
		}
	})
	t.Run("invalid journaled PID quarantines", func(t *testing.T) {
		t.Parallel()
		f := newRecoverState(t, "run_rec_badpid_jrn", "att_rec_badpid_jrn")
		deadStart := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		appendNativeLaunched(t, f.j, f.runID, f.attempt, -1, deadStart, nil, 2)
		writeWorkerJSON(t, f.stopState, workers.Identity{SchemaVersion: 1, RunID: f.runID,
			AttemptID: f.attempt, PID: -1, StartTime: deadStart,
			LaunchToken: f.launchToken, Nonce: f.rawNonce})
		outcome, err := f.reconcile(t)
		requireCode(t, err, CodeOwnershipUnresolved)
		if outcome != RecoverQuarantined {
			t.Fatalf("Reconcile = %q, want quarantined: a garbage journaled PID proves nothing", outcome)
		}
		if rows := attemptRows(t, f.db, f.runID); len(rows) != 1 {
			t.Fatalf("run holds %d attempts, want one: no admission on an unprovable probe", len(rows))
		}
	})
}
