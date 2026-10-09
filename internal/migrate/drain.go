package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/v2contract"
	"github.com/turbokast/mythhelm/internal/workers"
)

// DrainReport names every v1 run and its drain outcome: runs that finished
// cleanly, runs adopted under their live launch identity, and runs
// quarantined for stream-4 reconciliation (supervisor-migration design §4).
type DrainReport struct {
	Drained     []string `json:"drained"`     // run IDs that finished cleanly
	Adopted     []string `json:"adopted"`     // run IDs adopted same-launch-identity
	Quarantined []string `json:"quarantined"` // run IDs quarantined with evidence refs
}

// spawnWorker starts a replacement worker. Drain never calls it: a live
// worker is adopted under its own launch identity and a dead worker's
// run is quarantined, never relaunched (I12). Tests replace it with a
// recording fake to prove no launch spawns.
var spawnWorker = workers.Spawn

const (
	// drainDefaultTimeout bounds the quiesce when the caller's context
	// carries no deadline. In-flight runs normally finish or release
	// their owner lock long before it; a lock still held past it aborts
	// the drain with ownership_unresolved.
	drainDefaultTimeout = 5 * time.Minute
	// drainPollInterval paces the wait for a held owner lock to release.
	drainPollInterval = 50 * time.Millisecond
	// drainStabilizePasses bounds the re-enumeration that catches runs
	// admitted in the race between the migration lock and the first
	// enumeration. New admissions are refused while the lock is held, so
	// a second pass normally finds nothing; runs still appearing after
	// the last pass abort the drain.
	drainStabilizePasses = 3
)

// errDrainTimeout reports an owner lock held past the drain deadline.
var errDrainTimeout = errors.New("migrate: owner lock held past the drain deadline")

// Drain stops new admissions (via the held migration lock) and quiesces the
// legacy owner sites, returning each run's outcome in the report. Drain
// writes no ledger row: in-flight runs finish their admitted work, so the
// backup taken next contains every accepted legacy write. Quarantine and
// adopt outcomes persist after the backup from the DrainReport.
//
// Drain holds the instance lock for its whole run, so no service writer
// starts mid-drain, and the state directory's owner lock, so no legacy
// admission starts mid-drain; both release before Drain returns. On any
// failure Drain returns a zero report: nothing is half-adopted.
func Drain(ctx context.Context, db *sql.DB) (DrainReport, error) {
	if db == nil {
		return DrainReport{}, persistenceError("drain", "the ledger handle is nil")
	}
	if err := ctx.Err(); err != nil {
		return DrainReport{}, err
	}
	stateDir, err := drainStateDir(ctx, db)
	if err != nil {
		return DrainReport{}, err
	}
	releaseInstance, err := control.AcquireInstance(stateDir)
	if err != nil {
		return DrainReport{}, lockError("drain", err,
			"a supervisor is already running; stop it and retry the migration")
	}
	defer releaseInstance()
	releaseMigration, err := supervisor.AcquireOwner(stateDir)
	if err != nil {
		return DrainReport{}, lockError("drain", err,
			"another migration holds the state directory; retry after it finishes")
	}
	defer releaseMigration()

	deadline := drainDeadline(ctx)
	var releases []func()
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	seen := map[string]bool{}
	report := DrainReport{}
	for pass := range drainStabilizePasses {
		runs, err := enumerateRuns(ctx, db, stateDir)
		if err != nil {
			return DrainReport{}, err
		}
		fresh := runs[:0]
		for _, r := range runs {
			if !seen[r.id] {
				seen[r.id] = true
				fresh = append(fresh, r)
			}
		}
		if len(fresh) == 0 {
			return report, nil
		}
		if pass == drainStabilizePasses-1 {
			return DrainReport{}, ownershipError("drain", fresh[0].id,
				fmt.Sprintf("run %s started during the drain; retry the migration after it settles", fresh[0].id))
		}
		for _, r := range fresh {
			release, err := quiesceRun(ctx, stateDir, r.id, deadline)
			if err != nil {
				return DrainReport{}, err
			}
			releases = append(releases, release)
			if err := classifyRun(ctx, db, stateDir, &report, r); err != nil {
				return DrainReport{}, err
			}
		}
	}
	return report, nil
}

// drainRun is one enumerated v1 run awaiting quiesce.
type drainRun struct {
	id    string
	state string
}

// drainDeadline is the quiesce deadline: the caller's, or the default when
// the caller sets none.
func drainDeadline(ctx context.Context) time.Time {
	if deadline, ok := ctx.Deadline(); ok {
		return deadline
	}
	return time.Now().Add(drainDefaultTimeout)
}

// drainStateDir resolves the state directory holding db: the parent of the
// main database file. Drain takes a *sql.DB rather than a path so the
// caller (Task 6's Apply) owns the handle lifecycle.
func drainStateDir(ctx context.Context, db *sql.DB) (string, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA database_list")
	if err != nil {
		return "", persistenceError("drain", fmt.Sprintf("listing databases: %v", err))
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var seq int
		var name, file string
		if err := rows.Scan(&seq, &name, &file); err != nil {
			return "", persistenceError("drain", fmt.Sprintf("reading database list: %v", err))
		}
		if name == "main" {
			if file == "" {
				return "", persistenceError("drain", "the ledger has no database file")
			}
			return filepath.Dir(file), nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", persistenceError("drain", fmt.Sprintf("reading database list: %v", err))
	}
	return "", persistenceError("drain", "the ledger has no main database")
}

// enumerateRuns lists every v1 run Drain must quiesce: the runs table plus
// any run directory the table does not know yet (a run recorded between
// the migration lock and this query), in run ID order.
func enumerateRuns(ctx context.Context, db *sql.DB, stateDir string) ([]drainRun, error) {
	byID := map[string]drainRun{}
	rows, err := db.QueryContext(ctx, "SELECT run_id, state FROM runs ORDER BY run_id")
	if err != nil {
		return nil, persistenceError("drain", fmt.Sprintf("listing runs: %v", err))
	}
	for rows.Next() {
		var r drainRun
		if err := rows.Scan(&r.id, &r.state); err != nil {
			_ = rows.Close()
			return nil, persistenceError("drain", fmt.Sprintf("reading runs: %v", err))
		}
		byID[r.id] = r
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, persistenceError("drain", fmt.Sprintf("reading runs: %v", err))
	}
	_ = rows.Close()
	entries, err := os.ReadDir(filepath.Join(stateDir, "runs"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, persistenceError("drain", fmt.Sprintf("listing run directories: %v", err))
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, ok := byID[entry.Name()]; !ok {
			byID[entry.Name()] = drainRun{id: entry.Name()}
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	runs := make([]drainRun, 0, len(ids))
	for _, id := range ids {
		runs = append(runs, byID[id])
	}
	return runs, nil
}

// quiesceRun waits for runID's owner lock to release and holds it for the
// rest of the drain, so no legacy writer starts on the run mid-drain. A
// lock still held past the deadline aborts with ownership_unresolved.
func quiesceRun(ctx context.Context, stateDir, runID string, deadline time.Time) (func(), error) {
	runDir := filepath.Join(stateDir, "runs", runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return nil, persistenceError("drain", fmt.Sprintf("preparing run %s: %v", runID, err))
	}
	for {
		release, err := supervisor.AcquireOwner(runDir)
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, supervisor.ErrOwnerHeld) {
			return nil, persistenceError("drain", fmt.Sprintf("locking run %s: %v", runID, err))
		}
		if time.Now().After(deadline) {
			return nil, drainTimeoutError(runID)
		}
		select {
		case <-ctx.Done():
			// The context's deadline is the drain's deadline: expiring
			// it while the lock is held is the ownership timeout, not
			// a caller abort. An explicit cancel still propagates.
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, drainTimeoutError(runID)
			}
			return nil, ctx.Err()
		case <-time.After(drainPollInterval):
		}
	}
}

// classifyRun sorts a quiesced run into its report bucket: a run in a
// terminal state finished cleanly; an active run with a live, fully
// verified worker adopts under that launch identity; any other active run
// quarantines for stream-4 reconciliation. Nothing relaunches: a dead
// worker's launch identity is never reused by a new launch (I12).
func classifyRun(ctx context.Context, db *sql.DB, stateDir string, report *DrainReport, r drainRun) error {
	if terminalRunState(r.state) {
		report.Drained = append(report.Drained, r.id)
		return nil
	}
	adopt, err := adoptable(ctx, db, stateDir, r.id)
	if err != nil {
		return err
	}
	if adopt {
		report.Adopted = append(report.Adopted, r.id)
		return nil
	}
	report.Quarantined = append(report.Quarantined, r.id)
	return nil
}

// adoptable verifies the run's latest attempt for same-launch-identity
// adoption: the identity file must name this run and attempt, its launch
// token must match the durable attempt row, every journaled
// attempt.launched observation must name the same worker PID and start
// time, and that PID must still be that process. Any gap quarantines
// rather than adopts (I12); only ledger I/O fails the drain.
func adoptable(ctx context.Context, db *sql.DB, stateDir, runID string) (bool, error) {
	var attemptID, tokenSHA string
	err := db.QueryRowContext(ctx, `SELECT attempt_id, launch_token_sha256 FROM attempts
		WHERE run_id = ? ORDER BY attempt_number DESC LIMIT 1`, runID).Scan(&attemptID, &tokenSHA)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, persistenceError("drain", fmt.Sprintf("reading attempts of run %s: %v", runID, err))
	}
	id, err := workers.ReadIdentity(workers.AttemptDir(stateDir, runID, attemptID))
	if err != nil {
		return false, nil
	}
	sum := sha256.Sum256([]byte(id.LaunchToken))
	if id.RunID != runID || id.AttemptID != attemptID || id.PID <= 0 || id.StartTime.IsZero() ||
		hex.EncodeToString(sum[:]) != tokenSHA {
		return false, nil
	}
	launched, err := launchedObservations(ctx, db, runID, attemptID)
	if err != nil {
		return false, err
	}
	if len(launched) == 0 {
		// No journaled launch observation: unlike recovery, Drain holds
		// the owner lock, so no owner is left to journal one. Adopting
		// without the durable cross-check would trust the file alone.
		return false, nil
	}
	for _, ob := range launched {
		if ob.pid != id.PID || !ob.start.Equal(id.StartTime) {
			return false, nil
		}
	}
	start, err := workers.ProcessStartTime(id.PID)
	if err != nil {
		return false, nil
	}
	return start.Equal(id.StartTime), nil
}

// launchedObservation is one journaled attempt.launched worker identity.
type launchedObservation struct {
	pid   int
	start time.Time
}

// launchedObservations reads every attempt.launched payload journaled for
// the attempt. A corrupt payload fails the drain rather than adopting
// around it: adoption must not interpret around corrupt evidence.
func launchedObservations(ctx context.Context, db *sql.DB, runID, attemptID string) ([]launchedObservation, error) {
	rows, err := db.QueryContext(ctx, `SELECT payload FROM journal
		WHERE run_id = ? AND attempt_id = ? AND type = 'attempt.launched' ORDER BY run_sequence`, runID, attemptID)
	if err != nil {
		return nil, persistenceError("drain", fmt.Sprintf("reading launch observations of run %s: %v", runID, err))
	}
	defer func() { _ = rows.Close() }()
	var out []launchedObservation
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, persistenceError("drain", fmt.Sprintf("reading launch observations of run %s: %v", runID, err))
		}
		var decoded struct {
			WorkerPID       int       `json:"worker_pid"`
			WorkerStartTime time.Time `json:"worker_start_time"`
		}
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			return nil, persistenceError("drain", fmt.Sprintf("decoding launch observations of run %s: %v", runID, err))
		}
		out = append(out, launchedObservation{pid: decoded.WorkerPID, start: decoded.WorkerStartTime})
	}
	if err := rows.Err(); err != nil {
		return nil, persistenceError("drain", fmt.Sprintf("reading launch observations of run %s: %v", runID, err))
	}
	return out, nil
}

// terminalRunState is the complement of the supervisor's active states: a
// run here has no live owner to drain (pipeline.go activeStates;
// tui.go watchRunTerminal lists the same set).
func terminalRunState(state string) bool {
	switch supervisor.RunState(state) {
	case supervisor.RunReadyForReview, supervisor.RunCompleted, supervisor.RunBlocked,
		supervisor.RunFailed, supervisor.RunCancelled:
		return true
	}
	return false
}

// drainTimeoutError reports a run whose owner never released past the
// deadline: ownership_unresolved naming the run.
func drainTimeoutError(runID string) error {
	return errors.Join(errDrainTimeout, ownershipError("drain", runID,
		fmt.Sprintf("run %s still holds its owner lock; retry the migration after it finishes", runID)))
}

// ownershipError builds the catalogue refusal for a run Drain cannot take
// over: the code drives the retry disposition, the object names the run.
func ownershipError(operation, runID, nextAction string) *v2contract.ControlError {
	err := &v2contract.ControlError{
		Code:        v2contract.CodeOwnershipUnresolved,
		Owner:       "migrate",
		OperationID: ids.New(operation),
		Object:      runID,
		Disposition: v2contract.CodeOwnershipUnresolved.DefaultDisposition(),
		NextAction:  nextAction,
		Detail:      map[string]string{"migrate/run_id": runID},
	}
	return err
}

// persistenceError builds the catalogue error for ledger or lock I/O Drain
// cannot complete.
func persistenceError(operation, cause string) *v2contract.ControlError {
	return &v2contract.ControlError{
		Code:        v2contract.CodePersistenceUnavailable,
		Owner:       "migrate",
		OperationID: ids.New(operation),
		Disposition: v2contract.CodePersistenceUnavailable.DefaultDisposition(),
		NextAction:  "retry the migration; see detail migrate/cause",
		Detail:      map[string]string{"migrate/cause": cause},
	}
}

// lockError maps an acquire failure to its catalogue code: a held lock is
// unresolved ownership, an I/O failure is unavailable persistence.
func lockError(operation string, err error, heldNext string) *v2contract.ControlError {
	if errors.Is(err, control.ErrInstanceHeld) || errors.Is(err, control.ErrRootConflict) ||
		errors.Is(err, supervisor.ErrOwnerHeld) {
		out := ownershipError(operation, "", heldNext)
		out.Detail = map[string]string{"migrate/cause": err.Error()}
		return out
	}
	return persistenceError(operation, err.Error())
}
