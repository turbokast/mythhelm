package supervisor

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/statedir"
	"github.com/turbokast/mythhelm/internal/workers"
	"github.com/turbokast/mythhelm/internal/workspace"
)

const (
	// ingestInterval is how often the owner ingests the spool while the
	// attempt runs.
	ingestInterval = 100 * time.Millisecond
	// identityTimeout bounds the wait for a new worker's worker.json, and
	// identityPollInterval is how often it is looked for.
	identityTimeout      = 30 * time.Second
	identityPollInterval = 20 * time.Millisecond
	// reapTimeout bounds the wait for a worker to exit once its attempt's
	// terminal state is journaled.
	reapTimeout = 10 * time.Second
	// taskFile is the admitted task inside the run directory; the worker
	// delivers it to the native on stdin.
	taskFile = "task.md"
	// stopRequester is recorded as requested_by for a Ctrl-C stop.
	stopRequester = "user-interrupt"
)

// StopRequestedNotice and DetachedNotice are what the user sees on the first
// and second interrupt (AC-5.5).
const (
	StopRequestedNotice = "stop requested — waiting for worker confirmation"
	DetachedNotice      = "stop not yet confirmed; run 'mythhelm recover %s'"
)

// activeStates are the run states in which a run may still write to its
// workspace, or its owner is unresolved. Admission refuses a new run while
// any other run is in one (I05, N4).
var activeStates = []string{
	string(RunCreated), string(RunAdmission), string(RunExecuting), string(RunVerifying), string(RunApplying),
	string(RunStopping), string(RunInterrupted), string(RunRecovering),
}

// Hooks connect a run to the caller's terminal.
type Hooks struct {
	// Interrupt delivers the user's interrupts. The first requests a stop;
	// the second detaches before the stop is confirmed (AC-5.5).
	Interrupt <-chan os.Signal
	// Event receives every journaled event of the run, in run_sequence
	// order, as soon as it is journaled.
	Event func(journal.Event)
	// Notice receives status the user must see that is not an event.
	Notice func(string)
}

// Outcome is where a run's pipeline stopped.
type Outcome struct {
	RunID         string
	AttemptID     string
	State         RunState
	Reason        string
	AttemptState  AttemptState
	AttemptReason string
	// ActiveRun is the run that blocked this one with active_run_exists.
	ActiveRun string
	// Detached is set when the caller detached before the stop was
	// confirmed: the worker still owns the attempt and the run stays
	// stopping until it is recovered.
	Detached bool
}

// Run records the admitted decision d and carries its one attempt through
// the stages that exist: snapshot, launch intent, worker spawn and identity
// check, and spool ingestion under the run's owner lock until the attempt
// ends or the caller detaches. Verification is not implemented yet, so a
// native success ends failed/verification_unavailable.
//
// A run that could not be recorded at all returns a *admission.BlockedError
// (persistence_unavailable) or journal.ErrSchemaTooNew. Otherwise the
// Outcome says how far the run got, and a non-nil error reports a failure
// the run's recorded state does not explain.
func Run(ctx context.Context, d admission.Decision, h Hooks) (Outcome, error) {
	if h.Event == nil {
		h.Event = func(journal.Event) {}
	}
	if h.Notice == nil {
		h.Notice = func(string) {}
	}
	exe, err := os.Executable()
	if err != nil {
		return Outcome{}, fmt.Errorf("locating mythhelm for the worker: %w", err)
	}
	unavailable := func(err error) error {
		return errors.Join(&admission.BlockedError{Code: "persistence_unavailable", Field: d.StateDir,
			Action: "the state directory cannot be written; no run was admitted"}, err)
	}
	if err := statedir.Ensure(d.StateDir); err != nil {
		return Outcome{}, unavailable(err)
	}
	j, err := journal.Open(ctx, d.StateDir)
	if errors.Is(err, journal.ErrSchemaTooNew) {
		return Outcome{}, err
	}
	if err != nil {
		return Outcome{}, unavailable(err)
	}
	defer func() { _ = j.Close() }()
	if err := os.MkdirAll(d.RunDir, 0o700); err != nil {
		return Outcome{}, unavailable(err)
	}
	release, err := AcquireOwner(d.RunDir)
	if err != nil {
		return Outcome{}, err
	}
	defer release()

	p := &pipeline{d: d, h: h, j: j, exe: exe, prod: NewProducer(ids.New("sup"), 1),
		out: Outcome{RunID: d.RunID, AttemptID: d.AttemptID}}
	err = p.run(ctx)
	return p.out, err
}

type pipeline struct {
	d        admission.Decision
	h        Hooks
	j        *journal.Journal
	exe      string
	prod     *Producer
	out      Outcome
	rendered int64 // run_sequence of the last event passed to h.Event
}

func (p *pipeline) run(ctx context.Context) error {
	d := p.d
	if err := CreateRun(ctx, p.j, journal.RunRow{
		RunID:            d.RunID,
		AdapterID:        d.Adapter.ID,
		SourceRepo:       d.Snapshot.SourceRepo,
		SourceBranch:     d.Snapshot.Branch,
		BaseRev:          d.Snapshot.BaseRev,
		TaskSHA256:       d.Task.SHA256,
		BillingPosture:   d.Proposal.Billing.Mode,
		ExecutionProfile: d.Profile.Name,
	}, p.prod); err != nil {
		return err
	}
	p.out.State = RunCreated
	if err := p.flush(ctx); err != nil {
		return err
	}
	if err := p.runTo(ctx, RunAdmission, ""); err != nil {
		return err
	}
	// Checked after this run is recorded, so of two runs admitted at once
	// at least one sees the other: both may block, never both proceed.
	active, err := p.j.RunsInStates(ctx, activeStates...)
	if err != nil {
		return err
	}
	if i := slices.IndexFunc(active, func(r journal.RunRow) bool { return r.RunID != d.RunID }); i >= 0 {
		p.out.ActiveRun = active[i].RunID
		return p.runTo(ctx, RunBlocked, "active_run_exists")
	}
	if err := p.append(ctx, "admission.decided", d.Record()); err != nil {
		return err
	}
	if err := p.snapshot(ctx); err != nil {
		return errors.Join(err, p.runTo(ctx, RunFailed, "preflight_snapshot_failed"))
	}
	if err := p.runTo(ctx, RunExecuting, ""); err != nil {
		return err
	}
	return p.attempt(ctx)
}

// snapshot writes the task and clones the admitted revision (AC-3.3).
func (p *pipeline) snapshot(ctx context.Context) error {
	d := p.d
	if err := os.WriteFile(filepath.Join(d.RunDir, taskFile), d.Task.Content, 0o600); err != nil {
		return fmt.Errorf("writing the task: %w", err)
	}
	if err := workspace.Snapshot(ctx, d.Snapshot.SourceRepo, d.Snapshot.BaseRev, d.Workdir); err != nil {
		return fmt.Errorf("snapshotting %s: %w", d.Snapshot.BaseRev, err)
	}
	return p.append(ctx, "workspace.snapshot_created", map[string]any{
		"base_rev": d.Snapshot.BaseRev, "branch": nullIfEmpty(d.Snapshot.Branch), "clone_path": d.Workdir,
	})
}

// attempt records the launch intent, spawns and identifies the worker, and
// ingests its spool until the attempt ends.
func (p *pipeline) attempt(ctx context.Context) error {
	d := p.d
	if stopped, err := p.stopBeforeSpawn(ctx); stopped || err != nil {
		return err
	}
	token, err := launchToken()
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(token))
	if err := RecordLaunchIntent(ctx, p.j, journal.AttemptRow{
		AttemptID:         d.AttemptID,
		RunID:             d.RunID,
		TaskID:            d.TaskID,
		AttemptNumber:     1,
		LaunchTokenSHA256: hex.EncodeToString(sum[:]),
		WorkspacePath:     d.Workdir,
	}, p.prod); err != nil {
		return err
	}
	p.out.AttemptState = AttemptLaunchIntentRecorded
	if err := p.flush(ctx); err != nil {
		return err
	}
	if stopped, err := p.stopBeforeSpawn(ctx); stopped || err != nil {
		return err
	}

	lp := d.Proposal
	proc, err := workers.Spawn(p.exe, d.StateDir, d.RunID, d.AttemptID, workers.Launch{
		LaunchToken: token,
		TaskID:      d.TaskID,
		AdapterID:   d.Adapter.ID,
		Path:        lp.Spec.Path,
		Args:        lp.Spec.Args,
		Dir:         lp.Spec.Dir,
		Env:         lp.Spec.Env,
		PromptPath:  filepath.Join(d.RunDir, taskFile),
		StopLadder:  lp.StopLadder,
	})
	if err != nil {
		// Spawn kills a worker it could not hand the launch to, and a
		// worker without its launch starts nothing.
		return errors.Join(err,
			p.attemptTo(ctx, AttemptInterrupted, "worker_spawn_failed"),
			p.runTo(ctx, RunFailed, "worker_spawn_failed"))
	}
	exited := make(chan struct{})
	go func() {
		_, _ = proc.Wait()
		close(exited)
	}()

	ref := AttemptRef{StateDir: d.StateDir, RunID: d.RunID, AttemptID: d.AttemptID}
	if err := acceptWorker(ctx, ref.Dir(), proc.Pid, sum, exited); err != nil {
		// A worker that cannot be identified is lost and never signalled
		// (AC-10.3).
		p.h.Notice(fmt.Sprintf("worker not accepted: %v", err))
		return p.lose(ctx)
	}
	// A detached run stays stopping; a lost worker's run, or one whose
	// spool could not be ingested, is already interrupted.
	if err := p.watch(ctx, ref, exited); err != nil || p.out.Detached || p.out.State == RunInterrupted {
		return err
	}
	if err := p.conclude(ctx); err != nil {
		return err
	}
	select {
	case <-exited:
	case <-time.After(reapTimeout):
		p.h.Notice(fmt.Sprintf("worker pid %d has not exited %s after its attempt ended", proc.Pid, reapTimeout))
	}
	return nil
}

// stopBeforeSpawn consumes a pending interrupt at each side of the launch
// intent. Once a worker exists, watch owns the stop and its confirmation.
func (p *pipeline) stopBeforeSpawn(ctx context.Context) (bool, error) {
	select {
	case <-p.h.Interrupt:
		p.h.Notice("stop requested before worker launch; no native started")
		if p.out.AttemptState == AttemptLaunchIntentRecorded {
			if err := p.attemptTo(ctx, AttemptInterrupted, "cancelled_before_spawn"); err != nil {
				return true, err
			}
		}
		if err := p.runTo(ctx, RunStopping, ""); err != nil {
			return true, err
		}
		return true, p.runTo(ctx, RunCancelled, "")
	default:
		return false, nil
	}
}

func launchToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// errWorkerUnverified reports a worker whose identity does not match the
// launch (§7.3).
var errWorkerUnverified = errors.New("worker identity not verified")

// acceptWorker waits for the worker's identity and accepts it only if its
// PID, its process start time and its launch token all match (AC-5.1).
func acceptWorker(ctx context.Context, dir string, pid int, tokenSHA256 [32]byte, exited <-chan struct{}) error {
	deadline := time.Now().Add(identityTimeout)
	for {
		gone := closed(exited)
		id, err := workers.ReadIdentity(dir)
		switch {
		case err == nil:
			return verifyIdentity(id, pid, tokenSHA256)
		case !errors.Is(err, os.ErrNotExist):
			return fmt.Errorf("%w: %w", errWorkerUnverified, err)
		case gone:
			return fmt.Errorf("%w: the worker exited without writing its identity", errWorkerUnverified)
		case time.Now().After(deadline):
			return fmt.Errorf("%w: no identity after %s", errWorkerUnverified, identityTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-exited:
		case <-time.After(identityPollInterval):
		}
	}
}

func closed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// verifyIdentity checks worker.json against the spawned worker. A worker
// that has already exited and been reaped has no start time to compare:
// its PID is still the one this process spawned, the token authenticates
// its files, and there is nothing left to signal, so it is accepted. A live
// PID with another start time is a reused PID and is refused (AC-10.3).
func verifyIdentity(id workers.Identity, pid int, tokenSHA256 [32]byte) error {
	if id.PID != pid {
		return fmt.Errorf("%w: worker.json names pid %d, the spawned worker is %d", errWorkerUnverified, id.PID, pid)
	}
	if sha256.Sum256([]byte(id.LaunchToken)) != tokenSHA256 {
		return fmt.Errorf("%w: launch token does not match the journaled one", errWorkerUnverified)
	}
	start, err := workers.ProcessStartTime(pid)
	switch {
	case errors.Is(err, workers.ErrNoProcess):
		return nil
	case err != nil:
		return fmt.Errorf("%w: %w", errWorkerUnverified, err)
	case !start.Equal(id.StartTime):
		return fmt.Errorf("%w: pid %d started at %s, worker.json says %s", errWorkerUnverified, pid, start, id.StartTime)
	}
	return nil
}

// watch ingests the spool until the attempt reaches a terminal state, the
// worker is lost, or the caller detaches.
func (p *pipeline) watch(ctx context.Context, ref AttemptRef, exited <-chan struct{}) error {
	tick := time.NewTicker(ingestInterval)
	defer tick.Stop()
	interrupts := 0
	workerGone := false
	for {
		// A worker seen gone before this ingest has spooled everything it
		// ever will.
		gone := workerGone
		if _, err := Ingest(ctx, p.j, ref); err != nil {
			// The worker still owns the attempt; only this supervisor's view
			// of it is broken. The run is left for recover, not active.
			p.h.Notice(fmt.Sprintf("spool ingestion failed: %v; run 'mythhelm recover %s'", err, p.d.RunID))
			// Lines before the failing one were journaled; report the attempt
			// as recorded.
			a, aerr := p.j.Attempt(ctx, ref.AttemptID)
			if aerr != nil {
				return errors.Join(err, aerr)
			}
			p.out.AttemptState, p.out.AttemptReason = AttemptState(a.State), a.Reason
			if terr := p.runTo(ctx, RunInterrupted, "ingest_failed"); terr != nil {
				return errors.Join(err, terr)
			}
			return nil
		}
		if err := p.flush(ctx); err != nil {
			return err
		}
		a, err := p.j.Attempt(ctx, ref.AttemptID)
		if err != nil {
			return err
		}
		p.out.AttemptState, p.out.AttemptReason = AttemptState(a.State), a.Reason
		if attemptEnded(p.out.AttemptState) {
			return nil
		}
		if gone {
			p.h.Notice("the worker exited before the attempt ended")
			return p.lose(ctx)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-exited:
			workerGone, exited = true, nil
		case <-p.h.Interrupt:
			interrupts++
			if interrupts > 1 {
				p.out.Detached = true
				p.h.Notice(fmt.Sprintf(DetachedNotice, p.d.RunID))
				return nil
			}
			if err := workers.RequestStop(ref.Dir(), stopRequester); err != nil {
				return fmt.Errorf("requesting a stop: %w", err)
			}
			if err := p.runTo(ctx, RunStopping, ""); err != nil {
				return err
			}
			p.h.Notice(StopRequestedNotice)
		case <-tick.C:
		}
	}
}

// attemptEnded reports the attempt states a worker ends in (ADR 0004).
func attemptEnded(s AttemptState) bool {
	switch s {
	case AttemptSucceededNative, AttemptFailedNative, AttemptStopped, AttemptInterrupted, AttemptQuarantined:
		return true
	}
	return false
}

// conclude moves the run to what its attempt's terminal state means (design
// §4, §6.4). A run that is stopping was cancelled by the user whatever the
// native did, as long as the worker confirmed its process group gone.
func (p *pipeline) conclude(ctx context.Context) error {
	stopping := p.out.State == RunStopping
	switch p.out.AttemptState {
	case AttemptInterrupted:
		reason := p.out.AttemptReason
		if err := p.attemptTo(ctx, AttemptQuarantined, reason); err != nil {
			return err
		}
		return p.runTo(ctx, RunInterrupted, reason)
	case AttemptStopped:
		if !stopping {
			if err := p.runTo(ctx, RunStopping, ""); err != nil {
				return err
			}
		}
		return p.runTo(ctx, RunCancelled, "")
	case AttemptSucceededNative, AttemptFailedNative:
		if stopping {
			return p.runTo(ctx, RunCancelled, "")
		}
		if p.out.AttemptState == AttemptFailedNative {
			return p.runTo(ctx, RunFailed, "native_failed")
		}
		// Freeze (Task 11) and verification (Task 12) are not in this
		// build: a native success is never reported as verified.
		if err := p.runTo(ctx, RunVerifying, ""); err != nil {
			return err
		}
		return p.runTo(ctx, RunFailed, "verification_unavailable")
	}
	return fmt.Errorf("attempt %s ended in unexpected state %s", p.d.AttemptID, p.out.AttemptState)
}

// lose records a worker that is gone, or was never identified, before the
// attempt's end was spooled: the attempt is interrupted and quarantined and
// the run interrupted (§7.3).
func (p *pipeline) lose(ctx context.Context) error {
	const reason = "worker_lost"
	if err := p.attemptTo(ctx, AttemptInterrupted, reason); err != nil {
		return err
	}
	if err := p.attemptTo(ctx, AttemptQuarantined, reason); err != nil {
		return err
	}
	return p.runTo(ctx, RunInterrupted, reason)
}

func (p *pipeline) runTo(ctx context.Context, to RunState, reason string) error {
	if err := TransitionRun(ctx, p.j, p.d.RunID, to, reason, p.prod); err != nil {
		return err
	}
	p.out.State, p.out.Reason = to, reason
	return p.flush(ctx)
}

func (p *pipeline) attemptTo(ctx context.Context, to AttemptState, reason string) error {
	if err := TransitionAttempt(ctx, p.j, p.d.AttemptID, to, reason, p.prod); err != nil {
		return err
	}
	p.out.AttemptState, p.out.AttemptReason = to, reason
	return p.flush(ctx)
}

// append journals a supervisor event that changes no projection.
func (p *pipeline) append(ctx context.Context, typ string, payload any) error {
	ev, err := newEvent(p.d.RunID, "", "", typ, payload, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := p.prod.append(ctx, p.j, ev, nil); err != nil {
		return err
	}
	return p.flush(ctx)
}

// flush passes the run's newly journaled events to the Event hook.
func (p *pipeline) flush(ctx context.Context) error {
	evs, err := p.j.Events(ctx, p.d.RunID, p.rendered)
	if err != nil {
		return err
	}
	for _, ev := range evs {
		p.h.Event(ev)
		p.rendered = ev.RunSequence
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
