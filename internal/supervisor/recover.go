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
	"reflect"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/billing"
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
	out := RecoveryOutcome{RunID: runID, State: RunState(row.State), Reason: row.Reason}
	dir := filepath.Join(j.StateDir(), "runs", runID)
	// Recovery drains after the other owner sites: it is refused while
	// migration owns the state directory or the ledger has moved past
	// previewed, and reconciles migrated runs only once migration ends.
	// The guard returns the state-directory lock held; recovery keeps it
	// until the run lock is held, matching Run's lock order (migration
	// lock first, run lock second) so neither site can deadlock Drain.
	releaseMigration, err := checkMigrationClear(ctx, j, "recover the run")
	if err != nil {
		return out, err
	}
	release, err := AcquireOwner(dir)
	releaseMigration()
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
	// An operator's extension is journaled before anything else, so the
	// decision is durable whatever recovery does next (AC-4.2).
	if g := h.Extension; g != nil {
		if err := RequestExtension(ctx, j, runID, g.Kind, g.RaisedTo, g.DecidedBy); err != nil {
			return out, err
		}
	}
	// Named terminal states need no worker access. Receipt persistence may
	// have been interrupted after their state transition committed.
	switch out.State {
	case RunReadyForReview, RunBlocked, RunFailed, RunCancelled:
		out.Mode = "continued"
		err = p.repairReceipt(ctx)
		return out, err
	case RunApplying, RunCompleted:
		out.Mode = "continued"
		err = p.recoverApply(ctx, row)
		// Delegation may have transitioned the run (completed or blocked);
		// report the reconciled state either way.
		if fresh, rerr := j.Run(ctx, runID); rerr == nil {
			out.State, out.Reason = RunState(fresh.State), fresh.Reason
		} else if err == nil {
			err = errors.Join(ErrOwnership, rerr)
		}
		return out, err
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
					if p.out.State == RunFailed || p.out.State == RunInterrupted {
						// The freeze classified the run itself; recovered_partial
						// must neither overwrite that reason nor attempt an
						// illegal failed→failed transition.
						out.Outcome, out.Mode = p.out, "continued"
						return out, nil
					}
				} else if err != nil {
					return out, err
				}
				// The attempt was lost, not concluded: its claim is orphaned
				// after reconciliation (AC-3.4).
				if err := orphanHeldReservations(ctx, j, runID); err != nil {
					return out, err
				}
				err := p.runTo(ctx, RunFailed, "recovered_partial")
				out.Outcome, out.Mode = p.out, "continued"
				return out, err
			}
		}
		err = p.quarantineRecovery(ctx)
		if oerr := orphanHeldReservations(ctx, j, runID); oerr != nil {
			err = errors.Join(err, oerr)
		}
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
	// The execution clock lives in the journaled envelope, so the recovering
	// supervisor enforces the same deadline the launch did (I21, AC-4.1).
	if err := p.recoverDeadline(ctx); err != nil {
		return out, err
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
		if oerr := orphanHeldReservations(ctx, j, runID); oerr != nil {
			err = errors.Join(err, oerr)
		}
	}
	return out, err
}

// recoverDeadline re-derives the execution deadline from the run's journaled
// envelope, with any extension granted since launch (AC-4.2). A run whose
// envelope cannot be read is not watched without a deadline (I02).
func (p *pipeline) recoverDeadline(ctx context.Context) error {
	row, err := p.j.RunEnvelope(ctx, p.d.RunID)
	if err != nil {
		return err
	}
	base := billing.Ceilings{Execution: time.Duration(row.ExecutionSeconds) * time.Second, Repairs: int(row.Repairs),
		Replans: int(row.Replans), TransportRetries: int(row.TransportRetries)}
	env, eff, err := effectiveCeilings(ctx, p.j, p.d.RunID, base)
	if err != nil {
		return err
	}
	deadline, _, err := envelopeDeadline(env, eff, time.Now())
	if err != nil {
		return err
	}
	p.ceilings, p.deadline = eff, deadline
	return nil
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

// journaledApplyRecords reads the apply intent and completion records. Any
// corrupt apply payload fails closed: recovery never interprets around it.
func journaledApplyRecords(events []journal.Event) (intent, completed *applyRecord, err error) {
	for _, ev := range events {
		if ev.Type != "apply.intent_recorded" && ev.Type != "apply.completed" {
			continue
		}
		var m applyRecord
		if err := json.Unmarshal(ev.Payload, &m); err != nil {
			return nil, nil, errors.Join(ErrOwnership, err)
		}
		if ev.Type == "apply.intent_recorded" {
			intent = &m
		} else {
			completed = &m
		}
	}
	return intent, completed, nil
}

// recoverApply reconciles an apply interrupted by a crash. The journaled
// intent binds the exact branch and candidate; the branch effect is
// inspected before any durable state is repaired. States ApplyRun can
// resume are delegated to it under this owner lock; the one it cannot —
// a v2 receipt file whose digest was never journaled — is verified field
// by field against the preserved v1 bytes and adopted only when genuine.
// Anything else leaves ownership unresolved without touching the journal,
// the receipts or the source checkout.
func (p *pipeline) recoverApply(ctx context.Context, row journal.RunRow) error {
	runID := row.RunID
	events, err := p.j.Events(ctx, runID, 0)
	if err != nil {
		return errors.Join(ErrOwnership, err)
	}
	intent, completed, err := journaledApplyRecords(events)
	if err != nil {
		return err
	}
	if intent == nil {
		return fmt.Errorf("%w: run is %s without a journaled apply intent", ErrOwnership, row.State)
	}
	if completed != nil && *completed != *intent {
		return fmt.Errorf("%w: journaled apply completion does not match its intent", ErrOwnership)
	}
	attempt, err := p.j.LatestAttempt(ctx, runID)
	if err != nil {
		return errors.Join(ErrOwnership, err)
	}
	candidate, err := p.j.Candidate(ctx, attempt.AttemptID)
	if err != nil {
		return errors.Join(ErrOwnership, err)
	}
	if intent.TargetRepo != row.SourceRepo || intent.CandidateCommit != candidate.Commit {
		return fmt.Errorf("%w: journaled apply intent no longer matches the run", ErrOwnership)
	}
	if err := workspace.CheckBranch(ctx, row.SourceRepo, intent.Branch); err != nil {
		return errors.Join(ErrOwnership, err)
	}
	current, err := workspace.BranchCommit(ctx, row.SourceRepo, intent.Branch)
	if err != nil && !errors.Is(err, workspace.ErrBranchExists) {
		return errors.Join(ErrOwnership, err)
	}
	if row.State == string(RunCompleted) {
		// A completed run's effect is history: a disturbed destination is
		// manual reconciliation, never a silent repair or re-creation.
		switch {
		case err != nil:
			return fmt.Errorf("%w: apply destination is now a symbolic ref", ErrOwnership)
		case current == "":
			return fmt.Errorf("%w: apply branch is absent after completion", ErrOwnership)
		case current != candidate.Commit:
			return fmt.Errorf("%w: apply branch no longer matches the candidate", ErrOwnership)
		}
		r, _, rerr := ReadReceipt(ctx, p.j, runID)
		if rerr != nil {
			return p.adoptUnjournaledV2(ctx, events, row, intent, completed, candidate.Commit)
		}
		if r["state"] == string(RunCompleted) && r["schema_version"] == float64(2) {
			return nil
		}
		if r["schema_version"] != float64(1) {
			return fmt.Errorf("%w: verified receipt is neither v1 nor completed v2", ErrOwnership)
		}
	}
	// The journaled intent is the standing instruction to complete this
	// explicitly requested apply, so an absent branch is created, an
	// existing one reconciled, and a conflicting destination or missing
	// base terminalizes the run as blocked with a fresh v1 receipt.
	_, derr := ApplyRun(ctx, p.j, runID, intent.Branch, false, false)
	if derr == nil {
		return nil
	}
	if fresh, rerr := p.j.Run(ctx, runID); rerr == nil && fresh.State == string(RunBlocked) {
		return nil
	} else if rerr != nil {
		return errors.Join(ErrOwnership, derr, rerr)
	}
	return errors.Join(ErrOwnership, derr)
}

// adoptUnjournaledV2 repairs a crash between the v2 receipt file write and
// its digest journal append. The preserved v1 bytes must match the latest
// journaled digest and the run's ready receipt; the unjournaled v2 bytes
// must equal that v1 outside exactly the apply fields. Only then is the
// digest of the exact v2 bytes journaled. No file is rewritten.
func (p *pipeline) adoptUnjournaledV2(ctx context.Context, events []journal.Event, row journal.RunRow, intent, completed *applyRecord, candidateCommit string) error {
	if completed == nil {
		return fmt.Errorf("%w: completed run has no journaled apply completion", ErrOwnership)
	}
	var latest string
	for _, ev := range events {
		if ev.Type != "receipt.written" {
			continue
		}
		var m struct {
			SHA256 string `json:"sha256"`
		}
		if err := json.Unmarshal(ev.Payload, &m); err != nil {
			return errors.Join(ErrOwnership, err)
		}
		latest = m.SHA256
	}
	if latest == "" {
		return fmt.Errorf("%w: no journaled receipt to verify against", ErrOwnership)
	}
	dir := filepath.Join(p.j.StateDir(), "runs", row.RunID)
	preserved, err := os.ReadFile(filepath.Join(dir, "receipt.v1.json")) // #nosec G304 -- fixed name under the owner-locked run directory
	if err != nil {
		return fmt.Errorf("%w: preserved v1 receipt unavailable: %w", ErrOwnership, err)
	}
	sum := sha256.Sum256(preserved)
	if hex.EncodeToString(sum[:]) != latest {
		return fmt.Errorf("%w: preserved v1 receipt does not match its journaled digest", ErrOwnership)
	}
	var v1 map[string]any
	if err := json.Unmarshal(preserved, &v1); err != nil {
		return errors.Join(ErrOwnership, err)
	}
	candidate, _ := v1["candidate"].(map[string]any)
	if v1["schema_version"] != float64(1) || v1["run_id"] != row.RunID ||
		v1["state"] != string(RunReadyForReview) || candidate["commit"] != candidateCommit {
		return fmt.Errorf("%w: preserved v1 receipt is not the run's ready receipt", ErrOwnership)
	}
	current, err := os.ReadFile(filepath.Join(dir, "receipt.json")) // #nosec G304 -- fixed name under the owner-locked run directory
	if err != nil {
		return errors.Join(ErrOwnership, err)
	}
	var v2 map[string]any
	if err := json.Unmarshal(current, &v2); err != nil {
		return errors.Join(ErrOwnership, err)
	}
	if err := checkAdoptedV2(v2, v1, row, intent, candidateCommit); err != nil {
		return err
	}
	sum = sha256.Sum256(current)
	return p.append(ctx, "receipt.written", map[string]any{"schema_version": 2, "sha256": hex.EncodeToString(sum[:]), "path": "receipt.json"})
}

// checkAdoptedV2 proves the unjournaled bytes are ApplyRun's genuine output:
// the exact deterministic apply fields over the verified v1, nothing else.
func checkAdoptedV2(v2, v1 map[string]any, row journal.RunRow, intent *applyRecord, candidateCommit string) error {
	fail := func(format string, args ...any) error {
		return fmt.Errorf("%w: unjournaled v2 receipt "+format, append([]any{ErrOwnership}, args...)...)
	}
	if v2["schema_version"] != float64(2) {
		return fail("has schema %v, want 2", v2["schema_version"])
	}
	if v2["state"] != string(RunCompleted) {
		return fail("has state %v, want completed", v2["state"])
	}
	if v2["run_id"] != row.RunID {
		return fail("names another run")
	}
	if v2["exit_code"] != float64(0) {
		return fail("has exit code %v, want 0", v2["exit_code"])
	}
	effects, ok := v2["external_effects"].([]any)
	if !ok || len(effects) != 1 {
		return fail("has %v external effects, want one branch effect", v2["external_effects"])
	}
	effect, ok := effects[0].(map[string]any)
	if !ok {
		return fail("has a malformed branch effect")
	}
	if effect["type"] != "branch_created" && effect["type"] != "branch_reconciled" {
		return fail("has effect type %v", effect["type"])
	}
	if effect["target_repo"] != row.SourceRepo || effect["branch"] != intent.Branch || effect["commit"] != candidateCommit {
		return fail("names another branch effect")
	}
	actions, ok := v2["remaining_human_action"].([]any)
	if !ok || len(actions) != 2 || actions[0] != "review the branch" || actions[1] != "sign off (DCO) after review" {
		return fail("has unexpected remaining actions")
	}
	for _, key := range []string{"schema_version", "state", "exit_code", "external_effects", "remaining_human_action"} {
		delete(v1, key)
		delete(v2, key)
	}
	if !reflect.DeepEqual(v1, v2) {
		return fail("differs from the verified v1 outside the apply fields")
	}
	return nil
}
