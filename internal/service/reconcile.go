package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
)

// reconcileRun is the actor producer consumed by Task9 startup wiring. It does
// not register a public method or enumerate/start runs during package init.
func (r *runtime) reconcileRun(ctx context.Context, runID string) (*actor, error) {
	return r.reconcileRunObserved(ctx, runID, nil)
}

// The optional private observation checkpoint surrounds actual committed-event
// delivery. It supplies no identity, authority, predicate or successful result.
func (r *runtime) reconcileRunObserved(ctx context.Context, runID string, onCommitted func(journal.Event)) (*actor, error) {
	return r.reconcileRunCheckpoint(ctx, runID, onCommitted, nil)
}

// beforeCommit is an observational barrier after the real event, epoch and
// completed operation writes but before transaction commit. It cannot supply
// an identity, authority decision, SQL handle or successful result.
func (r *runtime) reconcileRunCheckpoint(ctx context.Context, runID string, onCommitted func(journal.Event), beforeCommit func()) (*actor, error) {
	owner, err := supervisor.AcquireServiceOwner(ctx, r.journal, r.lease, runID)
	if err != nil {
		return nil, err
	}
	ready := make(chan error, 1)
	a, err := r.startEntry(ctx, owner, func(ctx context.Context, h supervisor.Hooks) error {
		adoption, err := r.verifyAdoption(ctx, runID)
		if err != nil {
			ready <- err
			return err
		}
		if err := r.journal.Transact(ctx, func(tx *sql.Tx) error {
			// Re-read the exact attempt/offset under the transaction before publishing.
			var token string
			var offset int64
			if err := tx.QueryRowContext(ctx, `SELECT launch_token_sha256,spool_offset FROM attempts WHERE attempt_id=? AND run_id=?`, adoption.AttemptID, runID).Scan(&token, &offset); err != nil {
				return err
			}
			if token != adoption.LaunchSHA256 || offset != adoption.CommittedSpoolOffset {
				return supervisor.ErrOwnership
			}
			if _, _, err := journal.ReadAssignmentEpoch(ctx, tx, runID); err != nil {
				return err
			}
			if err := control.RecordServiceAdoption(ctx, tx, adoption); err != nil {
				return err
			}
			if beforeCommit != nil {
				beforeCommit()
			}
			return nil
		}); err != nil {
			ready <- err
			return err
		}
		ready <- nil // Durable adoption precedes readiness and recovery presence.
		if onCommitted != nil {
			deliver := h.Event
			h.Event = func(e journal.Event) { onCommitted(e); deliver(e) }
		}
		_, err = supervisor.RecoverOwned(ctx, r.journal, owner, runID, h)
		return err
	})
	if err != nil {
		_ = owner.Close()
		return nil, err
	}
	select {
	case err := <-ready:
		if err != nil {
			return nil, err
		}
		return a, nil
	case <-ctx.Done():
		// The admitted actor remains owned until coordinated shutdown joins it.
		return nil, ctx.Err()
	}
}

// verifyAdoption is read-only. In particular it never invokes recovery/ingest
// or writes presence before the adoption transaction commits.
func (r *runtime) verifyAdoption(ctx context.Context, runID string) (control.ServiceAdoption, error) {
	var out control.ServiceAdoption
	refuse := func() (control.ServiceAdoption, error) { return out, supervisor.ErrOwnership }
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if err := r.lease.CheckRootIdentity(ctx, r.journal.StateDir()); err != nil {
		return refuse()
	}
	if err := r.journal.CheckStateIdentity(ctx); err != nil {
		return refuse()
	}
	row, err := r.journal.LatestAttempt(ctx, runID)
	if err != nil {
		return refuse()
	}
	pin, err := control.ReadLaunchProvenance(ctx, r.journal, runID, row.AttemptID)
	if err != nil {
		return out, err
	}
	if pin.Generation == nil || pin.ExecutionDeadline.IsZero() {
		return refuse()
	}
	dir := workers.AttemptDir(r.journal.StateDir(), runID, row.AttemptID)
	id, err := workers.ReadIdentity(dir)
	if err != nil {
		return refuse()
	}
	sum := sha256.Sum256([]byte(id.LaunchToken))
	if id.RunID != runID || id.AttemptID != row.AttemptID || id.PID <= 0 || id.StartTime.IsZero() || hex.EncodeToString(sum[:]) != row.LaunchTokenSHA256 || workers.NonceDigest(id.Nonce) != pin.NonceSHA256 {
		return refuse()
	}
	start, err := workers.ProcessStartTime(id.PID)
	if err != nil || !start.Equal(id.StartTime) {
		return refuse()
	}
	events, err := r.journal.Events(ctx, runID, 0)
	if err != nil {
		return out, err
	}
	for _, ev := range events {
		if ev.Type != "attempt.launched" || ev.AttemptID != row.AttemptID {
			continue
		}
		if !adoptionLaunchMatches(ev.Payload, id) {
			return refuse()
		}
	}
	// Explicit adopted phase is also mandatory for the known-zero legacy case.
	if err := r.journal.Transact(ctx, func(tx *sql.Tx) error {
		var phase string
		if err := tx.QueryRowContext(ctx, `SELECT phase FROM migration_state WHERE id=1`).Scan(&phase); err != nil {
			return err
		}
		if phase != "adopted" {
			return supervisor.ErrOwnership
		}
		return nil
	}); err != nil {
		return out, err
	}
	if err := checkAdoptionSpool(ctx, filepath.Join(dir, supervisor.SpoolFile), row, id); err != nil {
		return out, err
	}
	return control.ServiceAdoption{SchemaVersion: 2, RunID: runID, AttemptID: row.AttemptID, LaunchSHA256: row.LaunchTokenSHA256, OperationID: ids.New("op"), WorkerPID: id.PID, WorkerStartedAt: id.StartTime, LaunchGeneration: *pin.Generation, ControlGeneration: r.lease.Generation(), CommittedSpoolOffset: row.SpoolOffset, ReconciliationEventID: ids.New("evt")}, nil
}

func adoptionLaunchMatches(payload []byte, id workers.Identity) bool {
	var launched struct {
		WorkerPID       int       `json:"worker_pid"`
		WorkerStartedAt time.Time `json:"worker_start_time"`
	}
	return json.Unmarshal(payload, &launched) == nil && launched.WorkerPID == id.PID && launched.WorkerStartedAt.Equal(id.StartTime)
}

func checkAdoptionSpool(ctx context.Context, path string, a journal.AttemptRow, id workers.Identity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	before, err := os.Lstat(path) // #nosec G703 -- fixed attempt spool under held root/run ownership; refuse nonregular before open and bind the opened inode below.
	if err != nil {
		return errors.Join(supervisor.ErrOwnership, err)
	}
	if !before.Mode().IsRegular() {
		return supervisor.ErrCorruptSpool
	}
	// Reject named nonregular files before open and bind the opened inode. As in
	// migration drain, hostile same-UID swaps between Lstat/open or unresponsive
	// filesystems can still block OS I/O; this is no arbitrary-I/O cancellation
	// guarantee or containment boundary.
	f, err := os.Open(path) // #nosec G304 -- fixed attempt spool under held root/run ownership; pre-open regular-file check and opened-inode comparison bind this read.
	if err != nil {
		return errors.Join(supervisor.ErrOwnership, err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || !os.SameFile(before, info) || a.SpoolOffset < 0 || a.SpoolOffset > info.Size() {
		return supervisor.ErrCorruptSpool
	}
	// Inspect a finite byte snapshot; a live appending worker cannot extend it.
	reader := bufio.NewReaderSize(io.LimitReader(f, info.Size()), (1<<20)+1)
	var offset int64
	boundary := a.SpoolOffset == 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := reader.ReadSlice('\n')
		if len(line) > 1<<20 {
			return supervisor.ErrCorruptSpool
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if errors.Is(err, io.EOF) {
			if !boundary {
				return supervisor.ErrCorruptSpool
			}
			return nil
		}
		offset += int64(len(line))
		if offset == a.SpoolOffset {
			boundary = true
		}
		var ev journal.Event
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		var trailing any
		if dec.Decode(&ev) != nil || dec.Decode(&trailing) != io.EOF || ev.EventID == "" || ev.ObservedAt.IsZero() || ev.ProducerSequence <= 0 || ev.Generation < 0 || !json.Valid(ev.Payload) || !bytes.HasPrefix(bytes.TrimSpace(ev.Payload), []byte("{")) || ev.SchemaVersion != 1 || ev.RunID != a.RunID || ev.AttemptID != a.AttemptID || ev.ProducerID != "wrk_"+a.AttemptID || !slices.Contains([]string{"attempt.state_changed", "attempt.launched", "attempt.native_session", "attempt.progress", "attempt.permission_denied", "attempt.protocol_counters", "attempt.native_result", "attempt.stop_requested", "attempt.stopped", "attempt.native_error"}, ev.Type) {
			return supervisor.ErrCorruptSpool
		}
		if ev.Type == "attempt.launched" && !adoptionLaunchMatches(ev.Payload, id) {
			return supervisor.ErrOwnership
		}
	}
}
