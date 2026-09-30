package supervisor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/security"
	"github.com/turbokast/mythhelm/internal/workers"
	"github.com/turbokast/mythhelm/internal/workspace"
)

// RecoveryOutcome describes reattachment, continued post-native work, or a
// lost attempt quarantined without creating a replacement native writer.
type RecoveryOutcome struct {
	Outcome
	Mode           string
	UnresolvedPIDs []int
}

// Recover acquires the abandoned run's owner lock and reconciles its durable
// observations. It never calls an adapter's Prepare/Start or workers.Spawn.
func Recover(ctx context.Context, j *journal.Journal, runID string) (RecoveryOutcome, error) {
	return RecoverWithHooks(ctx, j, runID, Hooks{})
}

// RecoverWithHooks renders journal events and supports foreground interrupts
// while reattaching to the original worker.
func RecoverWithHooks(ctx context.Context, j *journal.Journal, runID string, h Hooks) (RecoveryOutcome, error) {
	row, err := j.Run(ctx, runID)
	if err != nil {
		return RecoveryOutcome{}, err
	}
	out := RecoveryOutcome{Outcome: Outcome{RunID: runID, State: RunState(row.State), Reason: row.Reason}}
	dir := filepath.Join(j.StateDir(), "runs", runID)
	release, err := AcquireOwner(dir)
	if err != nil {
		return out, err
	}
	defer release()
	// The previous owner may have finished between our lookup and lock
	// acquisition. All decisions use a fresh projection under this lock.
	row, err = j.Run(ctx, runID)
	if err != nil {
		return out, err
	}
	out.State, out.Reason = RunState(row.State), row.Reason
	if h.Event == nil {
		h.Event = func(journal.Event) {}
	}
	if h.Notice == nil {
		h.Notice = func(string) {}
	}
	p := &pipeline{j: j, h: h, prod: NewProducer(ids.New("sup"), 1), out: out.Outcome, d: admission.Decision{RunID: runID, StateDir: j.StateDir(), RunDir: dir}}
	// Named terminal states need no worker access. Receipt persistence may
	// have been interrupted after their state transition committed.
	switch out.State {
	case RunReadyForReview, RunBlocked, RunFailed, RunCancelled:
		out.Mode = "continued"
		err = p.repairReceipt(ctx)
		return out, err
	case RunApplying, RunCompleted:
		return out, fmt.Errorf("%w: apply receipt reconciliation requires recorded apply intent", ErrOwnership)
	}
	a, err := j.LatestAttempt(ctx, runID)
	if err != nil {
		return out, errors.Join(ErrOwnership, err)
	}
	p.out.AttemptID, p.out.AttemptState, p.out.AttemptReason = a.AttemptID, AttemptState(a.State), a.Reason
	out.Outcome = p.out
	if out.State == RunVerifying {
		// An unfinished check may still be running under its own process group.
		// Never repeat its external effects just because the CLI disappeared.
		v, verr := j.LatestVerification(ctx, runID)
		if verr != nil {
			return out, errors.Join(ErrOwnership, fmt.Errorf("verification outcome unavailable: %w", verr))
		}
		target, reason := RunReadyForReview, ""
		if v.Result == "not_run" {
			reason = "unverified"
		} else if v.Result != "passed" {
			target, reason = RunFailed, "verification_failed"
		}
		err = p.runTo(ctx, target, reason)
		out.Outcome, out.Mode = p.out, "continued"
		return out, err
	}
	d, err := recoveryDecision(ctx, j, row, a)
	if err != nil {
		return out, errors.Join(ErrOwnership, err)
	}
	p.d = d
	if p.out.State == RunInterrupted {
		if err := p.runTo(ctx, RunRecovering, ""); err != nil {
			return out, err
		}
	}
	id, alive, iderr := recoveryIdentity(ctx, j, a)
	if iderr != nil {
		// A missing identity can be the small pre-acknowledgement launch window.
		// Poll it without spawning anything; bound the wait by the normal timeout.
		deadline := time.Now().Add(identityTimeout)
		for errors.Is(iderr, os.ErrNotExist) && time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return out, ctx.Err()
			case <-time.After(identityPollInterval):
			}
			id, alive, iderr = recoveryIdentity(ctx, j, a)
		}
	}
	if iderr != nil || p.out.AttemptState == AttemptQuarantined {
		if iderr == nil && !alive && p.out.AttemptState == AttemptQuarantined {
			resolved, resolveErr := p.descendantsResolved(ctx)
			if resolveErr != nil {
				return out, errors.Join(ErrOwnership, resolveErr)
			}
			if resolved {
				if err := p.append(ctx, "recovery.ownership_resolved", map[string]any{"attempt_id": a.AttemptID}); err != nil {
					return out, err
				}
				if _, err := j.Candidate(ctx, a.AttemptID); errors.Is(err, journal.ErrNotFound) {
					if err := p.freezeCandidate(ctx); err != nil {
						return out, err
					}
				} else if err != nil {
					return out, err
				}
				err := p.runTo(ctx, RunFailed, "recovered_partial")
				out.Outcome, out.Mode = p.out, "continued"
				return out, err
			}
		}
		err = p.quarantineRecovery(ctx)
		out.Outcome, out.Mode = p.out, "interrupted_and_quarantined"
		out.UnresolvedPIDs, _ = workers.OrphanPIDs(a.AttemptID)
		h.Notice(fmt.Sprintf("ownership unresolved; inspect %s; marked PIDs %v; no process was signalled", workers.AttemptMarker(a.AttemptID), out.UnresolvedPIDs))
		return out, errors.Join(err, ErrOwnership, iderr)
	}
	if p.out.State == RunRecovering {
		if err := p.runTo(ctx, RunExecuting, ""); err != nil {
			return out, err
		}
	}
	out.Mode = "continued"
	if alive {
		out.Mode = "reattached"
		h.Notice(fmt.Sprintf("reattached to worker pid %d", id.PID))
	}
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	exited := make(chan struct{})
	if !alive {
		close(exited)
	} else {
		go func() {
			defer close(exited)
			tick := time.NewTicker(ingestInterval)
			defer tick.Stop()
			for {
				select {
				case <-watchCtx.Done():
					return
				case <-tick.C:
					start, err := workers.ProcessStartTime(id.PID)
					if err != nil || !start.Equal(id.StartTime) {
						return
					}
				}
			}
		}()
	}
	err = p.watch(ctx, AttemptRef{StateDir: d.StateDir, RunID: runID, AttemptID: a.AttemptID}, exited)
	if err == nil && !p.out.Detached && p.out.State != RunInterrupted {
		// The candidate row and its journal event commit together. A freeze that
		// already committed is not repeated on recovery.
		if _, cerr := j.Candidate(ctx, a.AttemptID); errors.Is(cerr, journal.ErrNotFound) {
			err = p.freezeAfterStop(ctx)
		} else if cerr != nil {
			err = cerr
		}
		if err == nil && p.out.State != RunInterrupted && p.out.State != RunFailed {
			err = p.conclude(ctx)
		}
	}
	out.Outcome = p.out
	if p.out.State == RunInterrupted {
		out.Mode = "interrupted_and_quarantined"
		out.UnresolvedPIDs, _ = workers.OrphanPIDs(a.AttemptID)
		h.Notice(fmt.Sprintf("ownership unresolved; inspect %s; marked PIDs %v; no process was signalled", workers.AttemptMarker(a.AttemptID), out.UnresolvedPIDs))
		err = errors.Join(err, ErrOwnership)
	}
	return out, err
}

// descendantsResolved trusts a confirmed group stop and the worker's
// recorded descendant start times. Missing legacy identities stay unresolved;
// a reused PID is no longer this attempt's process and is never signalled.
func (p *pipeline) descendantsResolved(ctx context.Context) (bool, error) {
	events, err := p.j.Events(ctx, p.d.RunID, 0)
	if err != nil {
		return false, err
	}
	var stop struct {
		Confirmed  bool                      `json:"confirmed"`
		Scan       string                    `json:"descendant_scan"`
		PIDs       []int                     `json:"unresolved_pids"`
		Identities []workers.ProcessIdentity `json:"unresolved_identities"`
	}
	for _, ev := range events {
		if ev.Type == "attempt.stopped" && ev.AttemptID == p.d.AttemptID {
			if err := json.Unmarshal(ev.Payload, &stop); err != nil {
				return false, err
			}
		}
	}
	if !stop.Confirmed || stop.Scan == "failed" || len(stop.PIDs) == 0 {
		return false, nil
	}
	for _, pid := range stop.PIDs {
		var identity *workers.ProcessIdentity
		for i := range stop.Identities {
			if stop.Identities[i].PID == pid {
				identity = &stop.Identities[i]
				break
			}
		}
		start, err := workers.ProcessStartTime(pid)
		if errors.Is(err, workers.ErrNoProcess) {
			continue
		}
		if err != nil {
			return false, err
		}
		if identity == nil || identity.StartTime.IsZero() || start.Equal(identity.StartTime) {
			return false, nil
		}
	}
	pids, err := workers.OrphanPIDs(p.d.AttemptID)
	return len(pids) == 0, err
}

// recoveryDecision reconstructs only post-native work from admission's
// recorded choices. Mutable native configuration is never re-admitted.
func recoveryDecision(ctx context.Context, j *journal.Journal, r journal.RunRow, a journal.AttemptRow) (admission.Decision, error) {
	events, err := j.Events(ctx, r.RunID, 0)
	if err != nil {
		return admission.Decision{}, err
	}
	var rec admission.Record
	found := false
	for _, ev := range events {
		if ev.Type == "admission.decided" {
			if err := json.Unmarshal(ev.Payload, &rec); err != nil {
				return admission.Decision{}, err
			}
			found = true
		}
	}
	if !found {
		return admission.Decision{}, errors.New("admission decision missing")
	}
	dir := filepath.Join(j.StateDir(), "runs", r.RunID)
	f, err := os.Open(filepath.Join(dir, taskFile)) // #nosec G304 -- generated run ID under the private state directory
	if err != nil {
		return admission.Decision{}, err
	}
	task, readErr := io.ReadAll(io.LimitReader(f, admission.MaxTaskBytes+1))
	if err := errors.Join(readErr, f.Close()); err != nil {
		return admission.Decision{}, err
	}
	digest := sha256.Sum256(task)
	if len(task) > admission.MaxTaskBytes || hex.EncodeToString(digest[:]) != r.TaskSHA256 {
		return admission.Decision{}, errors.New("admitted task digest mismatch")
	}
	d := admission.Decision{RunID: r.RunID, TaskID: a.TaskID, AttemptID: a.AttemptID, StateDir: j.StateDir(), RunDir: dir, Workdir: a.WorkspacePath,
		Task: admission.Task{SHA256: r.TaskSHA256, Title: admission.TaskTitle(task)}, Snapshot: rec.Snapshot,
		GitName: rec.GitIdentity.Name, GitEmail: rec.GitIdentity.Email, Adapter: rec.Adapter, Probe: rec.Native, Profile: rec.ExecutionProfile,
		NoChecks: rec.NoChecks, KeepGoing: rec.KeepGoing, ConfigDigest: rec.ProjectConfigDigest}
	if !d.NoChecks {
		blob, err := workspace.Git(ctx, d.Workdir, false, "show", r.BaseRev+":"+admission.ProjectConfigFile)
		if err != nil {
			return d, err
		}
		cfg, digest, err := admission.ParseProjectConfig(blob)
		if err != nil {
			return d, err
		}
		if digest != d.ConfigDigest {
			return d, errors.New("admitted check configuration digest mismatch")
		}
		d.ProjectConfig = cfg
		d.Proposal.Spec.Env, err = security.BuildEnv(os.Environ(), cfg.Environment.Passthrough, nil)
		if err != nil {
			return d, err
		}
	}
	return d, nil
}

// quarantineRecovery leaves lost execution active for manual reconciliation.
// No terminal attempt is rewritten and no spool from an unverified worker
// can promote a candidate to ready_for_review.
func (p *pipeline) quarantineRecovery(ctx context.Context) error {
	if !attemptEnded(p.out.AttemptState) {
		if err := p.attemptTo(ctx, AttemptInterrupted, "worker_lost"); err != nil {
			return err
		}
	}
	if p.out.AttemptState == AttemptInterrupted {
		if err := p.attemptTo(ctx, AttemptQuarantined, p.out.AttemptReason); err != nil {
			return err
		}
	}
	if p.out.State != RunInterrupted {
		return p.runTo(ctx, RunInterrupted, "worker_lost")
	}
	return nil
}

// repairReceipt deterministically rebuilds a version 1 terminal receipt
// from committed journal projections, then publishes the exact byte digest.
func (p *pipeline) repairReceipt(ctx context.Context) error {
	r, err := BuildReceipt(ctx, p.j, p.d.RunID)
	if err != nil {
		return err
	}
	sha, err := WriteReceipt(p.d.RunDir, r)
	if err != nil {
		return err
	}
	return p.append(ctx, "receipt.written", map[string]any{"schema_version": 1, "sha256": sha, "path": "receipt.json"})
}
