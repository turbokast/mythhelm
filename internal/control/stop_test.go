package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/ownerlock"
	"github.com/turbokast/mythhelm/internal/v2contract"
	"github.com/turbokast/mythhelm/internal/workers"
)

func TestStopLadderValidation(t *testing.T) {
	t.Parallel()
	step := func(signal adapter.StopSignal, grace time.Duration) adapter.StopStep {
		return adapter.StopStep{Signal: signal, Grace: grace}
	}
	cases := []struct {
		name    string
		ladder  StopLadder
		wantErr string // "" means valid; otherwise a field the message must name
	}{
		{
			name:    "empty version",
			ladder:  StopLadder{Steps: []adapter.StopStep{step(adapter.StopInterrupt, time.Second)}},
			wantErr: "version",
		},
		{
			name:    "zero steps",
			ladder:  StopLadder{Version: "stop-ladder/v1"},
			wantErr: "steps",
		},
		{
			name:    "unknown signal",
			ladder:  StopLadder{Version: "stop-ladder/v1", Steps: []adapter.StopStep{step("hangup", time.Second)}},
			wantErr: "signal",
		},
		{
			name:    "zero grace",
			ladder:  StopLadder{Version: "stop-ladder/v1", Steps: []adapter.StopStep{step(adapter.StopKill, 0)}},
			wantErr: "grace",
		},
		{
			name:    "negative grace",
			ladder:  StopLadder{Version: "stop-ladder/v1", Steps: []adapter.StopStep{step(adapter.StopKill, -time.Second)}},
			wantErr: "grace",
		},
		{
			name:    "rung grace above max",
			ladder:  StopLadder{Version: "stop-ladder/v1", Steps: []adapter.StopStep{step(adapter.StopKill, MaxStopRungGrace+time.Second)}},
			wantErr: "grace",
		},
		{
			name:    "total above max deadline",
			ladder:  StopLadder{Version: "stop-ladder/v1", Steps: []adapter.StopStep{step(adapter.StopInterrupt, MaxStopRungGrace), step(adapter.StopTerminate, MaxStopRungGrace), step(adapter.StopTerminate, MaxStopRungGrace), step(adapter.StopKill, MaxStopRungGrace), step(adapter.StopKill, time.Second)}},
			wantErr: "total",
		},
		{
			name:   "total exactly max deadline passes",
			ladder: StopLadder{Version: "stop-ladder/v1", Steps: []adapter.StopStep{step(adapter.StopInterrupt, MaxStopRungGrace), step(adapter.StopTerminate, MaxStopRungGrace), step(adapter.StopTerminate, MaxStopRungGrace), step(adapter.StopKill, MaxStopRungGrace)}},
		},
		{
			name:   "one rung passes",
			ladder: StopLadder{Version: "stop-ladder/v1", Steps: []adapter.StopStep{step(adapter.StopKill, time.Second)}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := c.ladder.Validate()
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, &Error{Code: CodeInvalidContract}) {
				t.Fatalf("Validate = %v, want invalid_contract", err)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("Validate = %v, want it to name %q", err, c.wantErr)
			}
		})
	}
}

// stopState is a scripted stop attempt: a state directory with its journal,
// a running attempt row, an admission pin, a launched event and a worker.json
// naming this test process, which is always alive.
type stopState struct {
	dir      string
	db       *sql.DB
	j        *journal.Journal
	runID    string
	attempt  string
	version  string
	rawNonce string
	pid      int
	start    time.Time
}

func newStopState(t *testing.T, runID, attemptID string) *stopState {
	t.Helper()
	dir := t.TempDir()
	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, journal.DBName))+
		"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	seedRun(t, db, runID)
	if _, err := db.Exec(`INSERT INTO attempts (attempt_id, run_id, task_id, attempt_number, state, launch_token_sha256, workspace_path)
		VALUES (?, ?, 'task_1', 1, 'running', 'x', '/tmp/ws')`, attemptID, runID); err != nil {
		t.Fatal(err)
	}
	start, err := workers.ProcessStartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	f := &stopState{dir: dir, db: db, j: j, runID: runID, attempt: attemptID,
		version: "stop-ladder/v1", rawNonce: "test-raw-nonce", pid: os.Getpid(), start: start}
	appendPin(t, j, runID, attemptID, f.version, workers.NonceDigest(f.rawNonce), 1)
	appendLaunched(t, j, runID, attemptID, f.pid, f.start, 1)
	writeWorkerJSON(t, f, workers.Identity{SchemaVersion: 1, RunID: runID, AttemptID: attemptID,
		PID: f.pid, StartTime: f.start, LaunchToken: "tok_test", Nonce: f.rawNonce})
	return f
}

func appendLaunched(t *testing.T, j *journal.Journal, runID, attemptID string, pid int, start time.Time, seq int64) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"worker_pid": pid, "worker_start_time": start})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Append(t.Context(), journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: fmt.Sprintf("evt_launch_%s_%d", attemptID, seq),
		RunID: runID, AttemptID: attemptID, ProducerID: "wrk_" + attemptID,
		ProducerSequence: seq, Generation: 1, ObservedAt: time.Now().UTC(),
		Type: "attempt.launched", Payload: payload,
	}, nil); err != nil {
		t.Fatal(err)
	}
}

func writeWorkerJSON(t *testing.T, f *stopState, id workers.Identity) {
	t.Helper()
	dir := workers.AttemptDir(f.dir, f.runID, f.attempt)
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

// writeStopped spools one attempt.stopped line for the attempt, as the
// worker's spool.emit would: a journal.Event envelope per line.
func writeStopped(t *testing.T, f *stopState, payload string, seq int64) {
	t.Helper()
	line, err := json.Marshal(journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: "evt_stopped_" + f.attempt,
		RunID: f.runID, AttemptID: f.attempt, ProducerID: "wrk_" + f.attempt,
		ProducerSequence: seq, Generation: 1, ObservedAt: time.Now().UTC(),
		Type: "attempt.stopped", Payload: json.RawMessage(payload),
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := workers.AttemptDir(f.dir, f.runID, f.attempt)
	if err := os.MkdirAll(dir, 0o700); err != nil {
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

func stopIntent(t *testing.T, op, attemptID, version string) Intent {
	t.Helper()
	params, err := json.Marshal(StopParams{AttemptID: attemptID, LadderVersion: version})
	if err != nil {
		t.Fatal(err)
	}
	return Intent{OperationID: op, Method: "stop", Params: params}
}

func (f *stopState) deps() StopDeps {
	return StopDeps{DB: f.db, Journal: f.j, StateDir: f.dir, WaitDeadline: 2 * time.Second, PollInterval: 5 * time.Millisecond}
}

func (f *stopState) ctx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	return WithLedger(ctx, f.db)
}

func (f *stopState) attemptDir() string {
	return workers.AttemptDir(f.dir, f.runID, f.attempt)
}

func stopRequestID(t *testing.T, f *stopState) (string, bool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(f.attemptDir(), "stop.request"))
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	var req struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	return req.RequestID, true
}

func attemptState(t *testing.T, db *sql.DB, attemptID string) string {
	t.Helper()
	var state string
	if err := db.QueryRow(`SELECT state FROM attempts WHERE attempt_id = ?`, attemptID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestStopIntentWritesRequestFile(t *testing.T) {
	t.Parallel()
	f := newStopState(t, "run_stop_file", "att_stop_file")
	writeStopped(t, f, `{"confirmed":true,"sent":[{"signal":"interrupt","grace":1000000000}],"ladder_version":"stop-ladder/v1"}`, 2)
	srv := NewServer(nil)
	if err := srv.RegisterStop(f.deps()); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Dispatch(f.ctx(t), Peer{}, stopIntent(t, "op_stop_file", f.attempt, f.version)); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if got, ok := stopRequestID(t, f); !ok || got != "op_stop_file" {
		t.Fatalf("stop.request request_id = %q, want the operation_id", got)
	}

	// A version-mismatched intent writes no request file.
	f2 := newStopState(t, "run_stop_file2", "att_stop_file2")
	srv2 := NewServer(nil)
	if err := srv2.RegisterStop(f2.deps()); err != nil {
		t.Fatal(err)
	}
	_, err := srv2.Dispatch(f2.ctx(t), Peer{}, stopIntent(t, "op_stop_file_bad", f2.attempt, "stop-ladder/v0"))
	requireCode(t, err, CodeRevisionConflict)
	if got, ok := stopRequestID(t, f2); ok {
		t.Fatalf("version-mismatched stop left stop.request %q", got)
	}
}

func TestStopIntentIdempotent(t *testing.T) {
	t.Parallel()
	f := newStopState(t, "run_stop_idem", "att_stop_idem")
	writeStopped(t, f, `{"confirmed":true,"sent":[],"ladder_version":"stop-ladder/v1"}`, 2)
	ctx := f.ctx(t)
	h := StopHandler(f.deps())
	first, err := Execute(ctx, h, Peer{}, stopIntent(t, "op_stop_idem", f.attempt, f.version))
	if err != nil {
		t.Fatalf("first stop: %v", err)
	}
	second, err := Execute(ctx, h, Peer{}, stopIntent(t, "op_stop_idem", f.attempt, f.version))
	if err != nil {
		t.Fatalf("repeat stop: %v", err)
	}
	if len(first.Body) == 0 || string(first.Body) != string(second.Body) {
		t.Fatalf("repeat body %s differs from %s", second.Body, first.Body)
	}
	if n := count(t, f.db, "operations"); n != 1 {
		t.Fatalf("operations holds %d rows, want the one execution", n)
	}
	// The same ID with different params is revision_conflict without
	// executing: an unknown attempt would be invalid_contract if the
	// handler ran, so revision_conflict proves Execute refused it.
	_, err = Execute(ctx, h, Peer{}, stopIntent(t, "op_stop_idem", "att_unknown", f.version))
	requireCode(t, err, CodeRevisionConflict)
	if n := count(t, f.db, "operations"); n != 1 {
		t.Fatalf("operations holds %d rows after the conflicting reuse, want 1", n)
	}
}

func TestStopVersionMismatchWritesNoRequest(t *testing.T) {
	t.Parallel()
	f := newStopState(t, "run_stop_ver", "att_stop_ver")
	before := count(t, f.db, "journal")
	_, err := Execute(f.ctx(t), StopHandler(f.deps()), Peer{}, stopIntent(t, "op_stop_ver", f.attempt, "stop-ladder/v0"))
	requireCode(t, err, CodeRevisionConflict)
	if got, ok := stopRequestID(t, f); ok {
		t.Fatalf("version-mismatched stop left stop.request %q", got)
	}
	if got := attemptState(t, f.db, f.attempt); got != "running" {
		t.Fatalf("attempt state = %q, want running: no stop_requested transition", got)
	}
	if got := count(t, f.db, "journal"); got != before {
		t.Fatalf("journal holds %d events, want %d: nothing journaled", got, before)
	}
	// Unversioned params (the pre-change shape, with no ladder version)
	// cannot pass the version gate: without LadderVersion the intent is
	// malformed. The gate is structural — callers pass StopParams, never
	// a bare step list — so a missing version fails closed here.
	_, err = Execute(f.ctx(t), StopHandler(f.deps()), Peer{}, stopIntent(t, "op_stop_ver2", f.attempt, ""))
	requireCode(t, err, CodeInvalidContract)
}

func TestStopDeadlineQuarantines(t *testing.T) { // I06 (v2 §6.4), I09 (v2 §4.2)
	t.Parallel()
	f := newStopState(t, "run_stop_quar", "att_stop_quar")
	deps := f.deps()
	deps.WaitDeadline = 300 * time.Millisecond
	start := time.Now()
	_, err := Execute(f.ctx(t), StopHandler(deps), Peer{}, stopIntent(t, "op_stop_quar", f.attempt, f.version))
	elapsed := time.Since(start)
	requireCode(t, err, CodeCancelIncomplete)
	if d := v2contract.CodeCancelIncomplete.DefaultDisposition(); d != v2contract.DispositionAfterReconciliation {
		t.Fatalf("cancel_incomplete disposition = %q, want after_reconciliation", d)
	}
	if string(CodeCancelIncomplete) != string(v2contract.CodeCancelIncomplete) {
		t.Fatalf("control code %q differs from the v2 catalogue string", CodeCancelIncomplete)
	}
	if elapsed < deps.WaitDeadline {
		t.Fatalf("quarantined after %s, want the wait to expire at %s", elapsed, deps.WaitDeadline)
	}
	if got := attemptState(t, f.db, f.attempt); got != "quarantined" {
		t.Fatalf("attempt state = %q, want quarantined", got)
	}
	reqPath := filepath.Join(f.attemptDir(), "stop.request")
	before, err := os.ReadFile(reqPath) //nolint:gosec // G304: a test temp path
	if err != nil {
		t.Fatalf("the standing stop.request is gone: %v", err)
	}
	// A repeat replays the stored failure without delivering again.
	_, err = Execute(f.ctx(t), StopHandler(deps), Peer{}, stopIntent(t, "op_stop_quar", f.attempt, f.version))
	requireCode(t, err, CodeCancelIncomplete)
	after, err := os.ReadFile(reqPath) //nolint:gosec // G304: a test temp path
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("stop.request changed after the deadline: %q vs %q", before, after)
	}
	if n := count(t, f.db, "operations"); n != 1 {
		t.Fatalf("operations holds %d rows, want the one execution", n)
	}
}

func TestStopAgainstDeadWorkerProcessLost(t *testing.T) {
	t.Parallel()
	f := newStopState(t, "run_stop_dead", "att_stop_dead")
	deadStart := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	// The journaled launch and worker.json agree on a PID with no live
	// process behind it: the identity matches no live process.
	appendLaunched(t, f.j, f.runID, f.attempt, 1<<30, deadStart, 2)
	writeWorkerJSON(t, f, workers.Identity{SchemaVersion: 1, RunID: f.runID, AttemptID: f.attempt,
		PID: 1 << 30, StartTime: deadStart, LaunchToken: "tok_test", Nonce: f.rawNonce})
	_, err := Execute(f.ctx(t), StopHandler(f.deps()), Peer{}, stopIntent(t, "op_stop_dead", f.attempt, f.version))
	requireCode(t, err, CodeProcessLost)
	if got, ok := stopRequestID(t, f); ok {
		t.Fatalf("process_lost stop left stop.request %q: nothing signalled", got)
	}
	if got := attemptState(t, f.db, f.attempt); got != "running" {
		t.Fatalf("attempt state = %q, want running", got)
	}
}

func TestStopConfirmedReceiptAssemblesFromStoppedReport(t *testing.T) { // I06 (v2 §6.4)
	t.Parallel()
	f := newStopState(t, "run_stop_conf", "att_stop_conf")
	writeStopped(t, f, `{"confirmed":true,"sent":[{"signal":"interrupt","grace":1000000000},{"signal":"terminate","grace":2000000000}],"ladder_version":"stop-ladder/v1","unresolved_pids":[],"signals_sent":["interrupt","terminate"],"descendant_scan":"proc-environ","unresolved_identities":[]}`, 2)
	res, err := Execute(f.ctx(t), StopHandler(f.deps()), Peer{}, stopIntent(t, "op_stop_conf", f.attempt, f.version))
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	var receipt StopReceipt
	if err := json.Unmarshal(res.Body, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.AttemptID != f.attempt || receipt.LadderVersion != f.version || !receipt.Confirmed || receipt.Quarantined {
		t.Fatalf("receipt = %+v, want the pinned version and confirmed=true", receipt)
	}
	wantSent := []adapter.StopStep{{Signal: adapter.StopInterrupt, Grace: time.Second}, {Signal: adapter.StopTerminate, Grace: 2 * time.Second}}
	if len(receipt.Sent) != len(wantSent) {
		t.Fatalf("receipt sent = %+v, want %+v", receipt.Sent, wantSent)
	}
	for i, want := range wantSent {
		if receipt.Sent[i] != want {
			t.Fatalf("receipt sent[%d] = %+v, want %+v", i, receipt.Sent[i], want)
		}
	}
	if len(receipt.UnresolvedPIDs) != 0 {
		t.Fatalf("receipt unresolved_pids = %v, want none", receipt.UnresolvedPIDs)
	}
	golden := `{"attempt_id":"att_stop_conf","ladder_version":"stop-ladder/v1","sent":[{"signal":"interrupt","grace":1000000000},{"signal":"terminate","grace":2000000000}],"confirmed":true,"quarantined":false}`
	if string(res.Body) != golden {
		t.Fatalf("receipt body %s, want %s", res.Body, golden)
	}
	if got := attemptState(t, f.db, f.attempt); got != "stopped" {
		t.Fatalf("attempt state = %q, want stopped", got)
	}

	// A report with the wrong ladder version, or with no sent pairs, is
	// not a confirmation: the wait expires into cancel_incomplete.
	for _, c := range []struct{ name, runID, attempt, payload string }{
		{"wrong version", "run_stop_conf_wv", "att_stop_conf_wv",
			`{"confirmed":true,"sent":[{"signal":"interrupt","grace":1000000000}],"ladder_version":"stop-ladder/v0"}`},
		{"missing sent", "run_stop_conf_ms", "att_stop_conf_ms",
			`{"confirmed":true,"ladder_version":"stop-ladder/v1"}`},
	} {
		f2 := newStopState(t, c.runID, c.attempt)
		writeStopped(t, f2, c.payload, 2)
		deps := f2.deps()
		deps.WaitDeadline = 300 * time.Millisecond
		_, err := Execute(f2.ctx(t), StopHandler(deps), Peer{}, stopIntent(t, "op_"+c.attempt, f2.attempt, f2.version))
		if !errors.Is(err, &Error{Code: CodeCancelIncomplete}) {
			t.Errorf("%s: err = %v, want cancel_incomplete, never a confirmed receipt", c.name, err)
		}
	}
}

func TestStopUnconfirmedReportYieldsInterruptedReceipt(t *testing.T) { // I06 (v2 §6.4), I09 (v2 §4.2)
	t.Parallel()
	f := newStopState(t, "run_stop_unc", "att_stop_unc")
	writeStopped(t, f, `{"confirmed":false,"sent":[{"signal":"kill","grace":1000000000}],"ladder_version":"stop-ladder/v1","unresolved_pids":[4242]}`, 2)
	res, err := Execute(f.ctx(t), StopHandler(f.deps()), Peer{}, stopIntent(t, "op_stop_unc", f.attempt, f.version))
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	var receipt StopReceipt
	if err := json.Unmarshal(res.Body, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Confirmed || len(receipt.UnresolvedPIDs) != 1 || receipt.UnresolvedPIDs[0] != 4242 {
		t.Fatalf("receipt = %+v, want confirmed=false with the unresolved PID", receipt)
	}
	if got := attemptState(t, f.db, f.attempt); got != "interrupted" {
		t.Fatalf("attempt state = %q, want interrupted", got)
	}
}

func TestStopUnresolvedDescendantsAreNotConfirmed(t *testing.T) { // AC-1.2, I06 (v2 §6.4), I09 (v2 §4.2)
	t.Parallel()
	cases := []struct{ name, extra string }{
		{"descendant scan failed", `"unresolved_pids":[],"descendant_scan":"failed","unresolved_identities":[]`},
		{"unresolved pids", `"unresolved_pids":[4242],"descendant_scan":"proc-environ","unresolved_identities":[]`},
		{"unresolved identities", `"unresolved_pids":[],"descendant_scan":"proc-environ","unresolved_identities":[{"pid":4242,"start_time":"2026-01-01T00:00:00Z"}]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newStopState(t, "run_stop_desc", "att_stop_desc")
			writeStopped(t, f, `{"confirmed":true,"sent":[{"signal":"kill","grace":1000000000}],"ladder_version":"stop-ladder/v1",`+c.extra+`}`, 2)
			res, err := Execute(f.ctx(t), StopHandler(f.deps()), Peer{}, stopIntent(t, "op_stop_desc", f.attempt, f.version))
			if err != nil {
				t.Fatalf("stop: %v", err)
			}
			var receipt StopReceipt
			if err := json.Unmarshal(res.Body, &receipt); err != nil {
				t.Fatal(err)
			}
			if receipt.Confirmed {
				t.Fatalf("receipt = %+v, want confirmed=false: descendants are unresolved", receipt)
			}
			if got := attemptState(t, f.db, f.attempt); got != "interrupted" {
				t.Fatalf("attempt state = %q, want interrupted, never stopped", got)
			}
		})
	}
}

// holdRunOwner takes the run's owner lock the way a live legacy pipeline
// does, for the life of the test.
func holdRunOwner(t *testing.T, stateDir, runID string) {
	t.Helper()
	release, err := ownerlock.Acquire(filepath.Join(stateDir, "runs", runID))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
}

func TestStopRefusedWhileALiveOwnerHoldsTheRun(t *testing.T) { // N2, I18, I23, v2 §6.4
	t.Parallel()
	f := newStopState(t, "run_stop_owned", "att_stop_owned")
	holdRunOwner(t, f.dir, f.runID)
	_, err := Execute(f.ctx(t), StopHandler(f.deps()), Peer{}, stopIntent(t, "op_stop_owned", f.attempt, f.version))
	requireCode(t, err, CodeOwnershipUnresolved)
	if got := attemptState(t, f.db, f.attempt); got != "running" {
		t.Fatalf("attempt state = %q, want running: nothing is written under another owner", got)
	}
	if got, ok := stopRequestID(t, f); ok {
		t.Fatalf("refused stop left stop.request %q", got)
	}
}

func TestStopReleasesTheRunOwnerWhenDone(t *testing.T) { // v2 §6.4
	t.Parallel()
	f := newStopState(t, "run_stop_rel", "att_stop_rel")
	writeStopped(t, f, `{"confirmed":true,"sent":[{"signal":"kill","grace":1000000000}],"ladder_version":"stop-ladder/v1"}`, 2)
	if _, err := Execute(f.ctx(t), StopHandler(f.deps()), Peer{}, stopIntent(t, "op_stop_rel", f.attempt, f.version)); err != nil {
		t.Fatalf("stop: %v", err)
	}
	release, err := ownerlock.Acquire(filepath.Join(f.dir, "runs", f.runID))
	if err != nil {
		t.Fatalf("owner lock still held after the stop returned: %v", err)
	}
	release()
}

func TestStopLedgerUnavailablePersistenceUnavailable(t *testing.T) {
	t.Parallel()
	f := newStopState(t, "run_stop_down", "att_stop_down")
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := Execute(f.ctx(t), StopHandler(f.deps()), Peer{}, stopIntent(t, "op_stop_down", f.attempt, f.version))
	requireCode(t, err, CodePersistenceUnavailable)
	if got, ok := stopRequestID(t, f); ok {
		t.Fatalf("failed stop left stop.request %q", got)
	}
}

func TestStopRepeatDuringWaitJoins(t *testing.T) {
	t.Parallel()
	f := newStopState(t, "run_stop_join", "att_stop_join")
	deps := f.deps()
	deps.WaitDeadline = 10 * time.Second
	entered := make(chan string, 4)
	deps.entered = entered
	h := StopHandler(deps)
	in := stopIntent(t, "op_stop_join", f.attempt, f.version)
	var wg sync.WaitGroup
	var res1, res2 Result
	var err1, err2 error
	started1 := make(chan struct{})
	wg.Go(func() {
		close(started1)
		res1, err1 = Execute(f.ctx(t), h, Peer{}, in)
	})
	<-started1
	select {
	case got := <-entered:
		if got != "op_stop_join" {
			t.Fatalf("entered = %q, want the operation_id", got)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the first stop never reached its wait")
	}
	started2 := make(chan struct{})
	wg.Go(func() {
		close(started2)
		res2, err2 = Execute(f.ctx(t), h, Peer{}, in)
	})
	<-started2
	writeStopped(t, f, `{"confirmed":true,"sent":[{"signal":"kill","grace":1000000000}],"ladder_version":"stop-ladder/v1"}`, 2)
	wg.Wait()
	if err1 != nil || err2 != nil {
		t.Fatalf("err1 = %v, err2 = %v, want the shared receipt", err1, err2)
	}
	if len(res1.Body) == 0 || string(res1.Body) != string(res2.Body) {
		t.Fatalf("bodies %s and %s are not byte-identical", res1.Body, res2.Body)
	}
	if n := count(t, f.db, "operations"); n != 1 {
		t.Fatalf("operations holds %d rows, want the one journaled receipt", n)
	}
	if got, ok := stopRequestID(t, f); !ok || got != "op_stop_join" {
		t.Fatalf("stop.request request_id = %q, want the one standing request", got)
	}
}

func TestStopIdentityMismatchOwnershipUnresolved(t *testing.T) { // NFR-5, I06 (v2 §2)
	t.Parallel()
	f := newStopState(t, "run_stop_own", "att_stop_own")
	// Re-pin with a digest the observed raw nonce does not match, while
	// PID and start time agree and the worker is alive.
	appendPin(t, f.j, f.runID, f.attempt, f.version, workers.NonceDigest("some-other-nonce"), 2)
	_, err := Execute(f.ctx(t), StopHandler(f.deps()), Peer{}, stopIntent(t, "op_stop_own", f.attempt, f.version))
	requireCode(t, err, CodeOwnershipUnresolved)
	if got, ok := stopRequestID(t, f); ok {
		t.Fatalf("ownership_unresolved stop left stop.request %q: nothing signalled", got)
	}
	if got := attemptState(t, f.db, f.attempt); got != "running" {
		t.Fatalf("attempt state = %q, want running", got)
	}
}

func TestStopIdentityReadErrorReleasesTheOperation(t *testing.T) {
	t.Parallel()
	f := newStopState(t, "run_stop_flaky", "att_stop_flaky")
	writeStopped(t, f, `{"confirmed":true,"sent":[],"ladder_version":"stop-ladder/v1"}`, 2)
	// worker.json as a directory stands in for an unreadable identity
	// file (permissions, a Windows sharing violation): reading it fails
	// on every platform, even for root.
	idPath := filepath.Join(f.attemptDir(), "worker.json")
	if err := os.Remove(idPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(idPath, 0o700); err != nil {
		t.Fatal(err)
	}
	h := StopHandler(f.deps())
	in := stopIntent(t, "op_stop_flaky", f.attempt, f.version)
	_, err := Execute(f.ctx(t), h, Peer{}, in)
	if err == nil {
		t.Fatal("stop with an unreadable identity returned no error")
	}
	if _, ok := errors.AsType[*Error](err); ok {
		t.Fatalf("err = %v, want a transient failure, not a stored one", err)
	}
	if n := count(t, f.db, "operations"); n != 0 {
		t.Fatalf("operations holds %d rows, want none: nothing stored", n)
	}
	// Once the file reads again, the same operation_id executes.
	if err := os.Remove(idPath); err != nil {
		t.Fatal(err)
	}
	writeWorkerJSON(t, f, workers.Identity{SchemaVersion: 1, RunID: f.runID, AttemptID: f.attempt,
		PID: f.pid, StartTime: f.start, LaunchToken: "tok_test", Nonce: f.rawNonce})
	res, err := Execute(f.ctx(t), h, Peer{}, in)
	if err != nil {
		t.Fatalf("retry after the file healed: %v", err)
	}
	if len(res.Body) == 0 {
		t.Fatal("retry returned an empty receipt")
	}
}

func TestStopOnTerminalAttemptConflicts(t *testing.T) {
	t.Parallel()
	f := newStopState(t, "run_stop_term", "att_stop_term")
	if _, err := f.db.Exec(`UPDATE attempts SET state = 'stopped' WHERE attempt_id = ?`, f.attempt); err != nil {
		t.Fatal(err)
	}
	_, err := Execute(f.ctx(t), StopHandler(f.deps()), Peer{}, stopIntent(t, "op_stop_term", f.attempt, f.version))
	requireCode(t, err, CodeRevisionConflict)
	if got, ok := stopRequestID(t, f); ok {
		t.Fatalf("conflicting stop left stop.request %q", got)
	}
}

// writeStopRequestAt stands a stop.request with a controlled timestamp, as
// RequestStop would have written it at at.
func writeStopRequestAt(t *testing.T, dir, requestID string, at time.Time) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"request_id": requestID, "requested_at": at.UTC().Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stop.request"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestStopRestartRepeatReenters pins the served crash window: when a stop
// persisted stop_requested and delivered its request but died before
// storing a result, the same operation_id re-enters idempotently —
// instead of conflicting — and completes on the worker's report.
func TestStopRestartRepeatReenters(t *testing.T) {
	t.Parallel()
	f := newStopState(t, "run_stop_restart", "att_stop_restart")
	const op = "op_stop_restart"
	if _, err := f.db.Exec(`UPDATE attempts SET state = 'stop_requested', reason = ? WHERE attempt_id = ?`,
		op, f.attempt); err != nil {
		t.Fatal(err)
	}
	writeStopRequestAt(t, f.attemptDir(), op, time.Now().UTC())
	writeStopped(t, f, `{"confirmed":true,"sent":[{"signal":"kill","grace":1000000000}],"ladder_version":"stop-ladder/v1"}`, 2)
	res, err := ExecuteLong(f.ctx(t), StopHandler(f.deps()), Peer{}, stopIntent(t, op, f.attempt, f.version))
	if err != nil {
		t.Fatalf("restart repeat: %v", err)
	}
	var receipt StopReceipt
	if err := json.Unmarshal(res.Body, &receipt); err != nil {
		t.Fatal(err)
	}
	if !receipt.Confirmed || receipt.LadderVersion != f.version {
		t.Fatalf("receipt = %+v, want the confirmed pinned receipt", receipt)
	}
	if got := attemptState(t, f.db, f.attempt); got != "stopped" {
		t.Fatalf("attempt state = %q, want stopped", got)
	}
}

// TestStopRestartAfterDeadlineQuarantinesAtOnce pins the remaining-time
// rule: a restart repeat waits only the time left on the recorded
// request's deadline, so a request whose deadline already passed
// quarantines at once instead of waiting another full deadline.
func TestStopRestartAfterDeadlineQuarantinesAtOnce(t *testing.T) {
	t.Parallel()
	f := newStopState(t, "run_stop_expired", "att_stop_expired")
	const op = "op_stop_expired"
	if _, err := f.db.Exec(`UPDATE attempts SET state = 'stop_requested', reason = ? WHERE attempt_id = ?`,
		op, f.attempt); err != nil {
		t.Fatal(err)
	}
	writeStopRequestAt(t, f.attemptDir(), op, time.Now().Add(-time.Hour).UTC())
	deps := f.deps()
	deps.WaitDeadline = 30 * time.Second
	start := time.Now()
	_, err := ExecuteLong(f.ctx(t), StopHandler(deps), Peer{}, stopIntent(t, op, f.attempt, f.version))
	requireCode(t, err, CodeCancelIncomplete)
	if elapsed := time.Since(start); elapsed >= 10*time.Second {
		t.Fatalf("expired restart repeat waited %s, want the quarantine at once", elapsed)
	}
	if got := attemptState(t, f.db, f.attempt); got != "quarantined" {
		t.Fatalf("attempt state = %q, want quarantined", got)
	}
}

// TestScanStoppedRereadsAfterTruncation pins the shrink path: when the
// spool is rewritten to a shorter file, the next scan re-reads from the
// start instead of skipping past the new content into a wrongful
// timeout.
func TestScanStoppedRereadsAfterTruncation(t *testing.T) {
	t.Parallel()
	f := newStopState(t, "run_stop_trunc", "att_stop_trunc")
	writeStopped(t, f, `{"confirmed":true,"sent":[{"signal":"interrupt","grace":1000000000},{"signal":"terminate","grace":2000000000}],"ladder_version":"stop-ladder/v1","unresolved_pids":[]}`, 2)
	spool := filepath.Join(f.attemptDir(), "spool.jsonl")
	first, consumed := scanStopped(spool, f.attempt, f.version, 0)
	if first == nil || !first.Confirmed || len(first.Sent) != 2 {
		t.Fatalf("first scan receipt = %+v, want the two-step confirmed report", first)
	}
	// The worker rotates the spool: a shorter file holding a fresh
	// stopped report, with the old offset past its end.
	if err := os.Truncate(spool, 0); err != nil {
		t.Fatal(err)
	}
	writeStopped(t, f, `{"confirmed":true,"sent":[],"ladder_version":"stop-ladder/v1"}`, 3)
	st, err := os.Stat(spool)
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() >= consumed {
		t.Fatalf("rotated spool size = %d, want it below the old offset %d", st.Size(), consumed)
	}
	second, next := scanStopped(spool, f.attempt, f.version, consumed)
	if second == nil || !second.Confirmed || len(second.Sent) != 0 {
		t.Fatalf("scan after truncation receipt = %+v, want the fresh report from the new content", second)
	}
	if next != st.Size() {
		t.Fatalf("scan after truncation offset = %d, want the full new size %d", next, st.Size())
	}
}
