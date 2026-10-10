package control

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/v2contract"
	"github.com/turbokast/mythhelm/internal/workers"
)

// Stream-4 control codes. They extend the control.go catalogue with the
// exact v2 §4.5 strings for the codes the stop method returns; control.go
// predates them. (CodeProcessLost already lives in control.go.)
const (
	CodeCancelIncomplete    Code = "cancel_incomplete"
	CodeOwnershipUnresolved Code = "ownership_unresolved"
)

// stopPollInterval is how often the stop Handler polls the spool for the
// worker's stopped report: the 100ms ingestInterval precedent.
const stopPollInterval = 100 * time.Millisecond

// Spool event types the stop Handler reads. The worker emits them (Task 2);
// the handler only matches them.
const (
	stoppedEvent       = "attempt.stopped"
	stopRequestedEvent = "attempt.stop_requested"
	launchedEvent      = "attempt.launched"
)

// maxStoppedLine bounds one spool line the stop wait scans: the 1 MiB
// supervisor maxSpoolLine. A line longer than this is corrupt and skipped.
const maxStoppedLine = 1 << 20

// errStopDeadline reports that the stopped report never arrived.
var errStopDeadline = errors.New("control: the stopped report never arrived")

// StopDeps are the stop Handler's dependencies.
type StopDeps struct {
	DB       *sql.DB
	Journal  *journal.Journal
	StateDir string
	// WaitDeadline bounds the post-delivery wait for the stopped report;
	// zero means MaxStopDeadline. PollInterval overrides the spool poll;
	// zero means stopPollInterval. Tests set both short.
	WaitDeadline time.Duration
	PollInterval time.Duration
	// entered, when non-nil, receives the operation_id as the Handler
	// starts its wait. It is a test seam; production leaves it nil.
	entered chan<- string
}

// StopHandler answers the stop intent: it verifies the attempt's pinned
// ladder version, takes the run's owner lock for the whole stop (a live
// owner refuses it with ownership_unresolved before anything is written),
// verifies the worker identity, persists stop_requested, delivers
// the request file, waits for the worker's stopped report up to the
// recorded deadline, and journals the receipt. The served path runs it
// through ExecuteLong: each Mutate scope commits on its own and the
// bounded wait holds no transaction, so a waiting stop never wedges other
// intents; a repeat while the first execution still waits blocks, then
// replays its stored Result. Under single-transaction Execute (the tests'
// direct path) both scopes join the one transaction and the wait holds
// the write lock, the Task 1 deviation ExecuteLong exists to fix.
//
// Crash windows on the served path: a stop that persisted stop_requested
// but died before storing a result re-enters idempotently on the same
// operation_id (recordStopRequested returns early for its own reason),
// waiting only the time left on the standing request's deadline. A stop
// that died between the receipt commit and the result store leaves the
// attempt terminal, so its retry reports revision_conflict: the stop
// happened, but its receipt was never stored.
func StopHandler(d StopDeps) Handler {
	return func(ctx context.Context, _ Peer, in Intent) (Result, error) {
		var p StopParams
		if err := strictParams(in, &p); err != nil {
			return Result{}, err
		}
		if p.AttemptID == "" || p.LadderVersion == "" {
			return Result{}, newError(CodeInvalidContract, "stop needs attempt_id and ladder_version")
		}
		if d.DB == nil || d.Journal == nil || d.StateDir == "" {
			return Result{}, newError(CodePersistenceUnavailable, "stop is not available: no ledger is attached")
		}
		runID, err := stopAttemptRun(ctx, d.DB, p.AttemptID)
		if err != nil {
			return Result{}, err
		}
		pinned, digest, err := PinnedAdmission(ctx, d.Journal, runID, p.AttemptID)
		if err != nil {
			return Result{}, err
		}
		if p.LadderVersion != pinned {
			return Result{}, newError(CodeRevisionConflict,
				"stop ladder version %q does not match the pinned version %q", p.LadderVersion, pinned)
		}
		release, err := acquireRunOwner(d.StateDir, runID, "stop")
		if err != nil {
			return Result{}, err
		}
		defer release()
		dir := workers.AttemptDir(d.StateDir, runID, p.AttemptID)
		if err := checkWorkerIdentity(ctx, d.Journal, dir, runID, p.AttemptID, digest); err != nil {
			return Result{}, err
		}
		if err := recordStopRequested(ctx, d.DB, in.OperationID, p.AttemptID); err != nil {
			return Result{}, err
		}
		// Delivery is a file write, not ledger I/O: a failure here is
		// transient, so the operation_id stays retryable.
		if err := workers.RequestStop(dir, in.OperationID); err != nil {
			return Result{}, fmt.Errorf("control: delivering the stop request: %w", err)
		}
		wait, poll := d.WaitDeadline, d.PollInterval
		if wait <= 0 {
			wait = MaxStopDeadline
		}
		if poll <= 0 {
			poll = stopPollInterval
		}
		if d.entered != nil {
			select {
			case d.entered <- in.OperationID:
			default:
			}
		}
		receipt, err := waitForStoppedReport(ctx, filepath.Join(dir, "spool.jsonl"), p.AttemptID,
			pinned, stopDeadline(dir, in.OperationID, wait), poll)
		if err != nil {
			if errors.Is(err, errStopDeadline) {
				return recordQuarantine(ctx, d.DB, p.AttemptID, wait)
			}
			return Result{}, err
		}
		return recordReceipt(ctx, d.DB, p.AttemptID, receipt)
	}
}

// stopAttemptRun resolves the attempt's run. An unknown attempt is
// invalid_contract.
func stopAttemptRun(ctx context.Context, db *sql.DB, attemptID string) (string, error) {
	var runID string
	err := db.QueryRowContext(ctx, `SELECT run_id FROM attempts WHERE attempt_id = ?`, attemptID).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", newError(CodeInvalidContract, "attempt %q is unknown", attemptID)
	}
	if err != nil {
		return "", newError(CodePersistenceUnavailable, "reading attempt: %v", err)
	}
	return runID, nil
}

// checkWorkerIdentity verifies the observed worker.json against the
// journaled launch identity and nonce digest (NFR-5), then proves the
// process is live. A mismatch is ownership_unresolved; a worker already
// gone is process_lost: reconcile, don't signal. Either way nothing is
// written and nothing is signalled.
func checkWorkerIdentity(ctx context.Context, j *journal.Journal, dir, runID, attemptID, digest string) error {
	pid, start, found, err := latestLaunched(ctx, j, runID, attemptID)
	if err != nil {
		return err
	}
	if !found {
		return newError(CodeOwnershipUnresolved, "attempt %q has no journaled launch identity", attemptID)
	}
	observed, err := workers.ReadIdentity(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newError(CodeProcessLost, "attempt %q has no worker identity file", attemptID)
		}
		if _, ok := errors.AsType[*os.PathError](err); ok {
			// The file is there but unreadable (permissions, a Windows
			// sharing violation): transient, so the operation_id stays
			// retryable once the file can be read again.
			return fmt.Errorf("control: reading the worker identity: %w", err)
		}
		return newError(CodeOwnershipUnresolved, "attempt %q worker identity is not verifiable: %v", attemptID, err)
	}
	expected := workers.Identity{PID: pid, StartTime: start, Nonce: digest}
	if !workers.MatchIdentity(expected, observed) {
		return newError(CodeOwnershipUnresolved, "attempt %q worker identity does not match the journaled one", attemptID)
	}
	live, err := workers.ProcessStartTime(observed.PID)
	if errors.Is(err, workers.ErrNoProcess) {
		return newError(CodeProcessLost, "attempt %q worker process is gone", attemptID)
	}
	if err != nil {
		return newError(CodeOwnershipUnresolved, "attempt %q worker liveness is not verifiable: %v", attemptID, err)
	}
	if !live.Equal(observed.StartTime) {
		// The PID was reused by another process: never signal a stranger.
		return newError(CodeOwnershipUnresolved, "attempt %q worker PID start time changed", attemptID)
	}
	return nil
}

// latestLaunched returns the worker PID and start time from the attempt's
// latest journaled attempt.launched event. Events with unusable payloads
// are skipped; found is false when none is usable.
func latestLaunched(ctx context.Context, j *journal.Journal, runID, attemptID string) (pid int, start time.Time, found bool, err error) {
	events, err := j.Events(ctx, runID, 0)
	if err != nil {
		return 0, time.Time{}, false, newError(CodePersistenceUnavailable, "reading the launch identity: %v", err)
	}
	for _, ev := range events {
		if ev.AttemptID != attemptID || ev.Type != launchedEvent {
			continue
		}
		var launched struct {
			WorkerPID       int       `json:"worker_pid"`
			WorkerStartTime time.Time `json:"worker_start_time"`
		}
		if err := json.Unmarshal(ev.Payload, &launched); err != nil {
			continue
		}
		pid, start, found = launched.WorkerPID, launched.WorkerStartTime, true
	}
	return pid, start, found, nil
}

// recordStopRequested persists the stop_requested transition with the
// intent's operation_id. A transition the lifecycle forbids — stopping an
// attempt that already stopped, or a second stop while one is in flight —
// is revision_conflict. Re-entrant for its own operation_id: a repeat
// after a crash finds stop_requested recorded for itself and returns
// without writing, so the retry waits on instead of conflicting.
func recordStopRequested(ctx context.Context, db *sql.DB, operationID, attemptID string) error {
	return Mutate(ctx, db, func(tx *sql.Tx) error {
		from, err := journal.CurrentAttemptState(ctx, tx, attemptID)
		if errors.Is(err, journal.ErrNotFound) {
			return newError(CodeInvalidContract, "attempt %q is unknown", attemptID)
		}
		if err != nil {
			return newError(CodePersistenceUnavailable, "reading attempt state: %v", err)
		}
		if from == string(v2contract.AttemptStopRequested) {
			reason, err := attemptReason(ctx, tx, attemptID)
			if err != nil {
				return err
			}
			if reason == operationID {
				return nil
			}
		}
		if err := v2contract.CheckAttemptTransition(v2contract.AttemptState(from), v2contract.AttemptStopRequested); err != nil {
			return newError(CodeRevisionConflict, "attempt %q cannot stop from %s", attemptID, from)
		}
		if err := journal.SetAttemptState(ctx, tx, attemptID, string(v2contract.AttemptStopRequested), operationID); err != nil {
			return newError(CodePersistenceUnavailable, "recording stop_requested: %v", err)
		}
		return nil
	})
}

// attemptReason reads attemptID's projected reason inside tx.
func attemptReason(ctx context.Context, tx *sql.Tx, attemptID string) (string, error) {
	var reason sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT reason FROM attempts WHERE attempt_id = ?`, attemptID).Scan(&reason); err != nil {
		return "", newError(CodePersistenceUnavailable, "reading attempt reason: %v", err)
	}
	return reason.String, nil
}

// stopDeadline returns the wait deadline for a delivered request: the
// standing request's recorded time plus the wait when the standing
// request is this operation's, so a restart repeat waits only the
// remaining time — or quarantines at once when none remains. Any other
// shape (no standing request, another operation's, unreadable) waits the
// full wait from now.
func stopDeadline(dir, operationID string, wait time.Duration) time.Time {
	raw, err := os.ReadFile(filepath.Join(dir, "stop.request")) //nolint:gosec // G304: fixed request name under the ledger-resolved attempt directory
	if err != nil {
		return time.Now().Add(wait)
	}
	var req struct {
		RequestID   string    `json:"request_id"`
		RequestedAt time.Time `json:"requested_at"`
	}
	if err := json.Unmarshal(raw, &req); err != nil || req.RequestID != operationID || req.RequestedAt.IsZero() {
		return time.Now().Add(wait)
	}
	return req.RequestedAt.Add(wait)
}

// recordReceipt journals the stopped report as the Result body with the
// checked terminal transition for the reported outcome: a confirmed
// report stops the attempt, an unconfirmed one interrupts it with the
// unresolved remainder named in the receipt.
func recordReceipt(ctx context.Context, db *sql.DB, attemptID string, receipt *StopReceipt) (Result, error) {
	body, err := json.Marshal(receipt)
	if err != nil {
		return Result{}, fmt.Errorf("control: encoding the stop receipt: %w", err)
	}
	target, reason := v2contract.AttemptStopped, ""
	if !receipt.Confirmed {
		target, reason = v2contract.AttemptInterrupted, "stop_unconfirmed"
	}
	err = Mutate(ctx, db, func(tx *sql.Tx) error {
		from, err := journal.CurrentAttemptState(ctx, tx, attemptID)
		if err != nil {
			return newError(CodePersistenceUnavailable, "reading attempt state: %v", err)
		}
		if err := v2contract.CheckAttemptTransition(v2contract.AttemptState(from), target); err != nil {
			return newError(CodeRevisionConflict, "attempt %q cannot move from %s to %s", attemptID, from, target)
		}
		if err := journal.SetAttemptState(ctx, tx, attemptID, string(target), reason); err != nil {
			return newError(CodePersistenceUnavailable, "recording the stop outcome: %v", err)
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Body: body}, nil
}

// recordQuarantine concludes an unconfirmed stop: the attempt is
// quarantined and no further process is signalled. The quarantine is a
// failure with intentional writes, so it returns the failure in the
// Result with a nil error: Execute rolls a handler's writes back when
// the handler itself returns a *Error, which would lose the quarantine.
// The stored Result still replays as cancel_incomplete.
func recordQuarantine(ctx context.Context, db *sql.DB, attemptID string, wait time.Duration) (Result, error) {
	err := Mutate(ctx, db, func(tx *sql.Tx) error {
		from, err := journal.CurrentAttemptState(ctx, tx, attemptID)
		if err != nil {
			return newError(CodePersistenceUnavailable, "reading attempt state: %v", err)
		}
		if err := v2contract.CheckAttemptTransition(v2contract.AttemptState(from), v2contract.AttemptQuarantined); err != nil {
			return newError(CodeRevisionConflict, "attempt %q cannot move from %s to quarantined", attemptID, from)
		}
		if err := journal.SetAttemptState(ctx, tx, attemptID, string(v2contract.AttemptQuarantined), string(CodeCancelIncomplete)); err != nil {
			return newError(CodePersistenceUnavailable, "recording the quarantine: %v", err)
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	failure := newError(CodeCancelIncomplete, "stop of attempt %q did not confirm within %s; the attempt is quarantined", attemptID, wait)
	return Result{Error: failure}, nil
}

// waitForStoppedReport polls the spool for the attempt's stopped report
// until the deadline, following the supervisor Ingest line discipline:
// complete lines past a byte offset, a torn last line left for the next
// poll. Reports that fail validation are skipped, never confirmed, and so
// is a report that precedes the worker's attempt.stop_requested line: a
// worker that concluded before the request still spools a stopped line,
// which does not answer it. A missing or unreadable spool means the worker
// has not reported yet. It returns errStopDeadline when the deadline
// expires with no valid report.
func waitForStoppedReport(ctx context.Context, spoolPath, attemptID, pinned string, deadline time.Time, poll time.Duration) (*StopReceipt, error) {
	var offset int64
	requested := false
	for {
		var receipt *StopReceipt
		receipt, offset, requested = scanStopped(spoolPath, attemptID, pinned, offset, requested)
		if receipt != nil {
			return receipt, nil
		}
		if !time.Now().Before(deadline) {
			return nil, errStopDeadline
		}
		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// scanStopped reads the complete spool lines past offset and returns the
// last valid stopped report for the attempt that follows a stop_requested
// line, with the offset past every complete line consumed and whether a
// stop_requested line has been seen (requested carries it between polls).
// Oversize lines are corrupt and skipped; a torn last line is left for the
// next poll.
func scanStopped(spoolPath, attemptID, pinned string, offset int64, requested bool) (*StopReceipt, int64, bool) {
	f, err := os.Open(spoolPath) //nolint:gosec // G304: fixed spool name under the ledger-resolved attempt directory
	if err != nil {
		return nil, offset, requested
	}
	defer func() { _ = f.Close() }()
	if st, err := f.Stat(); err != nil || st.Size() < offset {
		if err != nil {
			return nil, offset, requested
		}
		// The spool shrank (rotation or rewrite): re-read from the
		// start so a stopped report already in the new content is
		// not skipped past into a wrongful timeout.
		offset, requested = 0, false
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, requested
	}
	data, err := io.ReadAll(io.LimitReader(f, maxStoppedLine+1))
	if err != nil {
		return nil, offset, requested
	}
	complete, torn := splitCompleteLines(data)
	if len(complete) == 0 && len(data) == maxStoppedLine+1 {
		// No newline in a full chunk: the line exceeds the bound, so it
		// is corrupt. Skip the chunk; fragments of it can never parse
		// as a report.
		return nil, offset + int64(len(data)), requested
	}
	var receipt *StopReceipt
	for _, line := range complete {
		switch {
		case strings.Contains(string(line), stopRequestedEvent):
			requested = requested || isStopRequested(line, attemptID)
		case requested && strings.Contains(string(line), stoppedEvent):
			if rep, ok := parseStoppedReport(line, attemptID, pinned); ok {
				receipt = rep
			}
		}
	}
	return receipt, offset + int64(len(data)-len(torn)), requested
}

// isStopRequested reports whether line is the attempt's stop_requested
// event, the worker's record that it took a stop request.
func isStopRequested(line []byte, attemptID string) bool {
	var ev journal.Event
	return json.Unmarshal(line, &ev) == nil && ev.Type == stopRequestedEvent && ev.AttemptID == attemptID
}

// splitCompleteLines splits data into newline-terminated lines and the
// torn tail after the last newline.
func splitCompleteLines(data []byte) (complete [][]byte, torn []byte) {
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			return complete, data
		}
		complete = append(complete, data[:i])
		data = data[i+1:]
	}
	return complete, nil
}

// parseStoppedReport decodes one spool line as the attempt's stopped
// report. The envelope decodes strictly; the payload decodes leniently
// with required fields checked: confirmed and sent must be present, sent
// must hold known signals with positive graces, and the ladder version
// must equal the pinned one. The receipt is confirmed only when the
// worker also resolved every descendant: unresolved PIDs or identities,
// or a failed descendant scan, make it unconfirmed.
func parseStoppedReport(line []byte, attemptID, pinned string) (*StopReceipt, bool) {
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	var ev journal.Event
	if err := dec.Decode(&ev); err != nil {
		return nil, false
	}
	if ev.Type != stoppedEvent || ev.AttemptID != attemptID {
		return nil, false
	}
	var payload struct {
		Confirmed      *bool             `json:"confirmed"`
		Sent           *json.RawMessage  `json:"sent"`
		LadderVersion  *string           `json:"ladder_version"`
		UnresolvedPIDs []int             `json:"unresolved_pids"`
		DescendantScan string            `json:"descendant_scan"`
		Identities     []json.RawMessage `json:"unresolved_identities"`
	}
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return nil, false
	}
	if payload.Confirmed == nil || payload.Sent == nil || payload.LadderVersion == nil || *payload.LadderVersion != pinned {
		return nil, false
	}
	var sent []adapter.StopStep
	if err := json.Unmarshal(*payload.Sent, &sent); err != nil || sent == nil {
		return nil, false
	}
	for _, s := range sent {
		switch s.Signal {
		case adapter.StopInterrupt, adapter.StopTerminate, adapter.StopKill:
		default:
			return nil, false
		}
		if s.Grace <= 0 {
			return nil, false
		}
	}
	// A confirmed group with escaped descendants, or a descendant scan
	// that failed, leaves ownership unresolved: the worker concludes the
	// attempt interrupted in both cases, so the receipt is not confirmed
	// (AC-1.2, I09).
	confirmed := *payload.Confirmed && len(payload.UnresolvedPIDs) == 0 &&
		len(payload.Identities) == 0 && payload.DescendantScan != "failed"
	return &StopReceipt{AttemptID: attemptID, LadderVersion: pinned, Sent: sent,
		Confirmed: confirmed, UnresolvedPIDs: payload.UnresolvedPIDs}, true
}
