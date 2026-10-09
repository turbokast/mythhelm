package supervisor

import (
	"context"
	"crypto/rand"
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

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/integration"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/qualify"
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
	// Extension is the operator's decision, on recovering a run blocked at
	// an envelope ceiling, to raise that ceiling (AC-4.2).
	Extension *ExtensionGrant
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

// Run records the admitted decision and carries its attempt through snapshot,
// native execution, candidate freeze, and verification under one owner lock.
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
	// The first admitted run seeds the seven v2 §7.2 records; refused
	// runs never reach Run and seed nothing. A seed failure fails the
	// run closed: no admission without its honest labels (I14).
	if err := qualify.EnsureSeeded(ctx, j); err != nil {
		return Outcome{}, unavailable(err)
	}
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
	if err != nil && p.out.State == "" {
		// A failed initial journal append admits nothing and starts no
		// worker. Report the persistence admission refusal (exit 3).
		err = unavailable(err)
	}
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

	// ceilings are the run's resolved envelope, set at admission; deadline is
	// when its execution time ends (zero until the first attempt starts), and
	// deadlineHit records that the stop in progress is the deadline's.
	ceilings    billing.Ceilings
	deadline    time.Time
	deadlineHit bool
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
	if err := p.appendAdmission(ctx); err != nil {
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

// appendAdmission journals the decision with its native side effects: a
// fresh entitlement declaration, an explicit native-config trust grant and
// the run's resolved envelope commit in the same transaction as the
// admission event, so a crash between them cannot admit a run whose
// declaration never persisted. The run's one reservation commits there too;
// a failed hold rolls the admission back and blocks the run.
func (p *pipeline) appendAdmission(ctx context.Context) error {
	d := p.d
	file, err := d.ProjectConfig.Envelopes.ToCeilings()
	if err != nil {
		return errors.Join(err, p.runTo(ctx, RunBlocked, "envelope_config_invalid"))
	}
	ceilings := billing.ResolveCeilings(d.EnvelopeFlags, file)
	p.ceilings = ceilings
	at := time.Now().UTC()
	ev, err := newEvent(d.RunID, "", "", "admission.decided", d.Record(), at)
	if err != nil {
		return err
	}
	decl, trust, digest, repo := d.Declaration, d.RecordNativeTrust, d.NativeConfigDigest, d.RepoIdentity
	rec, identityRef := quotaBucketInputs(d)
	var holdErr error
	// The envelope and the reservation commit with the admission, so no
	// admitted run exists without either, and a failed hold leaves nothing
	// behind (AC-3.2, I21).
	project := func(tx *sql.Tx) error {
		if decl != nil {
			if err := journal.InsertDeclaration(ctx, tx, *decl); err != nil {
				return err
			}
		}
		if trust {
			if err := journal.InsertTrustGrant(ctx, tx, admission.NativeConfigTrustKind, repo, digest, at); err != nil {
				return err
			}
		}
		if err := journal.UpsertRunEnvelope(ctx, tx, journal.EnvelopeRow{RunID: d.RunID,
			ExecutionSeconds: int64(ceilings.Execution / time.Second), Repairs: int64(ceilings.Repairs),
			Replans: int64(ceilings.Replans), TransportRetries: int64(ceilings.TransportRetries),
			UpdatedAt: at.Format(time.RFC3339Nano)}); err != nil {
			return err
		}
		if _, holdErr = admission.HoldQuotaReservation(ctx, tx, admission.NewJournalReserver(ceilings, EvaluateBucket), d.RunID, rec, identityRef); holdErr != nil {
			return holdErr
		}
		return nil
	}
	if err := p.prod.append(ctx, p.j, ev, project); err != nil {
		if holdErr != nil {
			// The admission rolled back with the failed hold: block the
			// run before any worker exists (I02). A bucket still waiting
			// on its retry schedule is the allowance, not a storage fault.
			reason := "quota_reservation_failed"
			if errors.Is(holdErr, billing.ErrAllowanceExhausted) {
				reason = "allowance_exhausted"
			}
			return errors.Join(err, p.runTo(ctx, RunBlocked, reason))
		}
		return err
	}
	p.h.Notice(admission.ReservationText(admission.QuotaBucket(rec, identityRef)))
	return p.flush(ctx)
}

// exhausted ends a run whose native reported an exhausted allowance, after
// the candidate froze: the bucket is recorded, the run blocked and the
// reservation released together. The native reports no reset time, so it
// stays unknown (I09). MYTHHELM starts nothing more; retries are the
// operator's, on the recorded schedule (D13).
func (p *pipeline) exhausted(ctx context.Context) error {
	held, err := heldReservations(ctx, p.j, p.d.RunID)
	if err != nil {
		return err
	}
	if len(held) == 0 {
		p.h.Notice("allowance exhausted with no held reservation; the bucket could not be recorded")
		return p.runTo(ctx, RunBlocked, "allowance_exhausted")
	}
	if err := HandleExhaustion(ctx, p.j, p.prod, p.d.RunID, held[0], nil); err != nil {
		return err
	}
	return p.settleRun(ctx, RunBlocked, "allowance_exhausted")
}

// endStop finishes a stopping run: cancelled when the user asked for the
// stop, blocked when the execution deadline did. Either way the stop ladder
// already ran and the candidate is preserved (I06).
func (p *pipeline) endStop(ctx context.Context) error {
	if p.deadlineHit {
		return p.runTo(ctx, RunBlocked, "envelope_deadline_exceeded")
	}
	return p.runTo(ctx, RunCancelled, "")
}

// blockLaunch turns a refused launch into a blocked run before any attempt
// is journaled. A gate that could not be evaluated blocks too (I02).
func (p *pipeline) blockLaunch(ctx context.Context, err error) error {
	reason := "envelope_unavailable"
	if gate, ok := errors.AsType[*GateError](err); ok {
		reason = gate.Reason
	}
	return errors.Join(err, p.runTo(ctx, RunBlocked, reason))
}

// quotaBucketInputs builds the bucket's record from the decision: its
// harness and surface come from the adapter descriptor, its entitlement
// class from the admitted billing posture and its identity from the native
// auth evidence, which the fake adapter has none of (unknown).
func quotaBucketInputs(d admission.Decision) (qualify.Record, string) {
	rec := qualify.Record{Key: qualify.Key{Harness: d.Adapter.Harness, Surface: d.Adapter.Surface, EntitlementClass: d.Proposal.Billing.EntitlementClass}}
	if d.NativeAuth == nil {
		return rec, ""
	}
	return rec, d.NativeAuth.IdentityRef
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
	if !d.NoChecks {
		_, digest, err := admission.LoadProjectConfig(d.Workdir)
		if err != nil || digest != d.ConfigDigest {
			return fmt.Errorf("snapshot project config digest %s does not match admitted digest %s: %w", digest, d.ConfigDigest, errors.Join(err, admission.ErrProjectConfig))
		}
	}
	at := time.Now().UTC()
	ev, err := newEvent(d.RunID, d.TaskID, "", "workspace.snapshot_created", map[string]any{
		"base_rev": d.Snapshot.BaseRev, "branch": nullIfEmpty(d.Snapshot.Branch), "clone_path": d.Workdir,
		"project_config_trust_granted": d.RecordTrust,
	}, at)
	if err != nil {
		return err
	}
	var project func(*sql.Tx) error
	if d.RecordTrust {
		project = func(tx *sql.Tx) error {
			return journal.InsertTrustGrant(ctx, tx, admission.ProjectConfigTrustKind, d.RepoIdentity, d.ConfigDigest, at)
		}
	}
	if err := p.prod.append(ctx, p.j, ev, project); err != nil {
		return err
	}
	return p.flush(ctx)
}

// attempt records the launch intent, spawns and identifies the worker, and
// ingests its spool until the attempt ends.
func (p *pipeline) attempt(ctx context.Context) error {
	d := p.d
	if stopped, err := p.stopBeforeSpawn(ctx); stopped || err != nil {
		return err
	}
	// The envelope gates the launch before any intent is journaled; the
	// launch is counted, and the first one starts the execution clock, in
	// the intent's own transaction (I21, I02).
	plan, deadline, err := p.planAttempt(ctx, time.Now().UTC())
	if err != nil {
		return p.blockLaunch(ctx, err)
	}
	p.deadline = deadline
	token, err := launchToken()
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(token))
	if err := recordLaunchIntent(ctx, p.j, journal.AttemptRow{
		AttemptID:         d.AttemptID,
		RunID:             d.RunID,
		TaskID:            d.TaskID,
		AttemptNumber:     1,
		LaunchTokenSHA256: hex.EncodeToString(sum[:]),
		WorkspacePath:     d.Workdir,
	}, p.prod, plan.commit(ctx, d.RunID)); err != nil {
		// The update refused the launch the check allowed: the envelope
		// changed in between.
		if _, ok := errors.AsType[*GateError](err); ok {
			return p.blockLaunch(ctx, err)
		}
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
		LaunchToken:  token,
		TaskID:       d.TaskID,
		AdapterID:    d.Adapter.ID,
		Path:         lp.Spec.Path,
		NativeSHA256: d.Probe.SHA256,
		Args:         lp.Spec.Args,
		Dir:          lp.Spec.Dir,
		Env:          lp.Spec.Env,
		PromptPath:   filepath.Join(d.RunDir, taskFile),
		StopLadder:   lp.StopLadder,
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
	if err := p.freezeAfterStop(ctx); err != nil {
		return err
	}
	if p.out.State == RunInterrupted || p.out.State == RunFailed {
		return nil
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

// planAttempt gates the next launch and returns its plan with the deadline
// the attempt runs under. A material replan after the first verification is
// optional work: it dispatches only when the remaining execution time covers
// one verification pass, and then runs under a deadline that leaves that
// pass unconsumed (AC-5.1, I21). The reserve is checked before the envelope
// so a short replan reports the shortfall. The first attempt and repairs are
// completion work and keep the execution deadline.
func (p *pipeline) planAttempt(ctx context.Context, now time.Time) (launchPlan, time.Time, error) {
	reserve, err := p.replanReserve(ctx, now)
	if err != nil {
		return launchPlan{}, time.Time{}, err
	}
	plan, err := planLaunchAt(ctx, p.j, p.d.RunID, p.ceilings, now)
	if err != nil {
		return launchPlan{}, time.Time{}, err
	}
	if reserve > 0 {
		plan.replanDeadline = plan.deadline.Add(-reserve)
	}
	return plan, plan.deadline.Add(-reserve), nil
}

// replanReserve returns the verification time a launch at now must leave
// unconsumed: zero unless the launch is a replan, and a *GateError with
// reason completion_reserve_shortfall when the remaining time cannot cover
// it. A check timeout or journal read that cannot be evaluated is an error
// that blocks the launch (I02).
func (p *pipeline) replanReserve(ctx context.Context, now time.Time) (time.Duration, error) {
	if _, err := p.j.LatestAttempt(ctx, p.d.RunID); errors.Is(err, journal.ErrNotFound) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	replan, err := IsReplan(ctx, p.j, p.d.RunID)
	if err != nil || !replan {
		return 0, err
	}
	var checks []billing.CheckBound
	if !p.d.NoChecks {
		for _, c := range p.d.ProjectConfig.Checks {
			timeout := c.Duration()
			if timeout <= 0 {
				return 0, fmt.Errorf("completion reserve: check %q has timeout %q, want a positive duration", c.Name, c.Timeout)
			}
			checks = append(checks, billing.CheckBound{Name: c.Name, Timeout: timeout})
		}
	}
	env, eff, err := effectiveCeilings(ctx, p.j, p.d.RunID, p.ceilings)
	if err != nil {
		return 0, err
	}
	deadline, _, err := envelopeDeadline(env, eff, now)
	if err != nil {
		return 0, err
	}
	left := eff.Execution
	if !deadline.IsZero() {
		left = max(deadline.Sub(now), 0)
	}
	est, err := billing.EstimateReserve(checks, eff)
	if err != nil {
		return 0, err
	}
	if !billing.RemainderCoversReserve(billing.ExecutionRemainder{TimeLeft: left}, est) {
		return 0, &GateError{Reason: "completion_reserve_shortfall", Err: fmt.Errorf("%w: %s left, a verification pass of %d checks needs %s (%s)",
			billing.ErrBudgetExhausted, left, est.VerifyPassChecks, est.VerifyTimeoutSum, est.Note)}
	}
	return est.VerifyTimeoutSum, nil
}

// freezeAfterStop trusts the worker's confirmed stop event, never just its
// terminal state. An unresolved descendant remains owned and cannot yield a
// candidate. Failed and cancelled native attempts produce partial candidates.
func (p *pipeline) freezeAfterStop(ctx context.Context) error {
	if p.out.Detached || p.out.State == RunInterrupted || p.out.AttemptState == AttemptInterrupted ||
		p.out.AttemptState == AttemptQuarantined {
		return nil
	}
	events, err := p.j.Events(ctx, p.d.RunID, 0)
	if err != nil {
		return err
	}
	confirmed := false
	unresolved := false
	for _, ev := range events {
		if ev.Type != "attempt.stopped" || ev.AttemptID != p.d.AttemptID {
			continue
		}
		var stop struct {
			Confirmed      bool  `json:"confirmed"`
			UnresolvedPIDs []int `json:"unresolved_pids"`
		}
		if err := json.Unmarshal(ev.Payload, &stop); err != nil {
			return err
		}
		confirmed, unresolved = stop.Confirmed, len(stop.UnresolvedPIDs) > 0
	}
	if !confirmed || unresolved {
		reason := "stop_unconfirmed"
		if unresolved {
			reason = "unresolved_descendants"
		}
		return p.runTo(ctx, RunInterrupted, reason)
	}
	return p.freezeCandidate(ctx)
}

// freezeCandidate is called only after ownership has been confirmed gone,
// either by the worker or by recovery's recorded PID/start-time checks.
func (p *pipeline) freezeCandidate(ctx context.Context) error {
	c, err := integration.Freeze(ctx, p.d.Workdir, p.d.Snapshot.BaseRev, integration.CommitMeta{
		RunID: p.d.RunID, AttemptID: p.d.AttemptID, Title: p.d.Task.Title,
		Name: p.d.GitName, Email: p.d.GitEmail,
		Partial: p.out.AttemptState != AttemptSucceededNative || p.out.State == RunStopping,
	})
	if err != nil {
		p.h.Notice(fmt.Sprintf("candidate freeze failed: %v", err))
		if p.out.State == RunStopping {
			return p.runTo(ctx, RunInterrupted, "freeze_failed")
		}
		return p.runTo(ctx, RunFailed, "freeze_failed")
	}
	changed, err := json.Marshal(c.Changed)
	if err != nil {
		return err
	}
	flags, err := json.Marshal(c.Flags)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"base_rev": c.BaseRev, "candidate_commit": c.Commit, "tree_id": c.Tree,
		"patch_sha256": c.PatchSHA256, "changed_count": len(c.Changed), "flags": c.Flags, "partial": c.Partial,
	}
	ev, err := newEvent(p.d.RunID, p.d.TaskID, p.d.AttemptID, "candidate.frozen", payload, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := p.prod.append(ctx, p.j, ev, func(tx *sql.Tx) error {
		return journal.InsertCandidate(ctx, tx, journal.CandidateRow{
			AttemptID: p.d.AttemptID, BaseRev: c.BaseRev, Commit: c.Commit, Tree: c.Tree,
			PatchSHA256: c.PatchSHA256, ChangedPaths: changed, Flags: flags, Partial: c.Partial,
		})
	}); err != nil {
		return err
	}
	return p.flush(ctx)
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
		case !errors.Is(err, os.ErrNotExist) && !retryIdentityRead(err):
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
	var expiry <-chan time.Time
	if !p.deadline.IsZero() {
		timer := time.NewTimer(time.Until(p.deadline))
		defer timer.Stop()
		expiry = timer.C
	}
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
		case <-expiry:
			// The execution deadline ends the attempt through the same stop
			// ladder as a user's stop; conclude reports it as blocked.
			expiry = nil
			// A stop the user already asked for stays the user's.
			p.deadlineHit = p.out.State != RunStopping
			if err := workers.RequestStop(ref.Dir(), stopRequester); err != nil {
				return fmt.Errorf("requesting a stop at the deadline: %w", err)
			}
			if p.out.State != RunStopping {
				if err := p.runTo(ctx, RunStopping, ""); err != nil {
					return err
				}
			}
			p.h.Notice("execution deadline reached; stopping the attempt")
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
			if p.out.State != RunStopping {
				if err := p.runTo(ctx, RunStopping, ""); err != nil {
					return err
				}
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
	// A terminal attempt ends its quota claim, with evidence; an interrupted
	// one stays held until recovery reconciles it. Exhaustion releases with
	// the bucket record in HandleExhaustion.
	switch p.out.AttemptState {
	case AttemptSucceededNative, AttemptFailedNative, AttemptStopped:
		if p.out.AttemptReason != "allowance_exhausted" || stopping {
			evidence := "attempt terminal: " + string(p.out.AttemptState)
			if err := releaseHeldReservations(ctx, p.j, p.d.RunID, evidence); err != nil {
				return err
			}
		}
	}
	switch p.out.AttemptState {
	case AttemptInterrupted:
		reason := p.out.AttemptReason
		if err := p.attemptTo(ctx, AttemptQuarantined, reason); err != nil {
			return err
		}
		return p.runTo(ctx, RunInterrupted, reason)
	case AttemptStopped:
		// A worker-initiated billing stop is a policy block, not a
		// cancellation, even when the run is already stopping because the
		// user also asked: the route violation is the significant fact
		// (AC-4.4), and stopping exits to blocked for exactly this reason.
		if p.out.AttemptReason == "billing_route_mismatch" {
			return p.runTo(ctx, RunBlocked, "billing_route_mismatch")
		}
		if !stopping {
			if err := p.runTo(ctx, RunStopping, ""); err != nil {
				return err
			}
		}
		return p.endStop(ctx)
	case AttemptSucceededNative, AttemptFailedNative:
		if stopping {
			return p.endStop(ctx)
		}
		if p.out.AttemptState == AttemptFailedNative {
			// Classified native failures keep their reason at run level
			// (design §6.4). Auth and billing classes block the run;
			// protocol and provider failures fail it.
			switch p.out.AttemptReason {
			case "authentication_failed", "oauth_org_not_allowed", "account_on_hold", "billing_error":
				return p.runTo(ctx, RunBlocked, "native_auth_or_billing")
			case "protocol_error":
				return p.runTo(ctx, RunFailed, "protocol_error")
			case "provider_limit":
				return p.runTo(ctx, RunFailed, "provider_limit")
			case "allowance_exhausted":
				return p.exhausted(ctx)
			default:
				return p.runTo(ctx, RunFailed, "native_failed")
			}
		}
		if err := p.runTo(ctx, RunVerifying, ""); err != nil {
			return err
		}
		if err := p.append(ctx, "verification.started", map[string]any{
			"candidate_attempt_id": p.d.AttemptID, "config_sha256": p.d.ConfigDigest,
			"no_checks": p.d.NoChecks,
		}); err != nil {
			return err
		}
		// Checks are waived, or refused under inspect: no per-check scope
		// exists yet, so none runs and none can be faked (I07).
		skipReason := ""
		switch {
		case p.d.NoChecks:
			skipReason = "waived by --no-checks"
		case p.d.Profile.Name == admission.ProfileInspect:
			skipReason = "checks_refused_under_inspect"
		}
		if skipReason != "" {
			candidate, err := p.j.Candidate(ctx, p.d.AttemptID)
			if err != nil {
				return errors.Join(err, p.runTo(ctx, RunFailed, "verification_unavailable"))
			}
			at, id := time.Now().UTC(), ids.New("ver")
			ev, err := newEvent(p.d.RunID, p.d.TaskID, p.d.AttemptID, "verification.completed", map[string]any{
				"verification_id": id, "candidate_commit": candidate.Commit, "result": "NOT RUN",
				"reason": skipReason, "baseline": "not-run",
			}, at)
			if err != nil {
				return err
			}
			if err := p.prod.append(ctx, p.j, ev, func(tx *sql.Tx) error {
				return journal.InsertVerification(ctx, tx, journal.VerificationRow{
					ID: id, RunID: p.d.RunID, CandidateCommit: candidate.Commit,
					Result: "not_run", StartedAt: at, FinishedAt: at,
				})
			}); err != nil {
				return err
			}
			if err := p.flush(ctx); err != nil {
				return err
			}
			return p.runTo(ctx, RunReadyForReview, "unverified")
		}
		candidate, err := p.j.Candidate(ctx, p.d.AttemptID)
		if err != nil {
			return errors.Join(err, p.runTo(ctx, RunFailed, "verification_unavailable"))
		}
		v, err := integration.RunChecksWithOptions(ctx, integration.Candidate{
			Commit: candidate.Commit, Workspace: p.d.Workdir,
		}, p.d.ProjectConfig, p.d.Proposal.Spec.Env, p.d.KeepGoing)
		if err != nil {
			p.h.Notice(fmt.Sprintf("verification could not complete: %v", err))
			return p.runTo(ctx, RunFailed, "verification_unavailable")
		}
		v.ConfigSHA256 = p.d.ConfigDigest
		for _, check := range v.Checks {
			if err := p.append(ctx, "check.completed", check); err != nil {
				return err
			}
		}
		id := ids.New("ver")
		ev, err := newEvent(p.d.RunID, p.d.TaskID, p.d.AttemptID, "verification.completed",
			map[string]any{"verification_id": id, "candidate_commit": v.CandidateCommit,
				"config_sha256": v.ConfigSHA256, "result": v.Result,
				"checks": v.Checks, "baseline": "not-run"}, v.FinishedAt)
		if err != nil {
			return err
		}
		if err := p.prod.append(ctx, p.j, ev, func(tx *sql.Tx) error {
			row := journal.VerificationRow{ID: id, RunID: p.d.RunID, CandidateCommit: v.CandidateCommit,
				ConfigSHA256: v.ConfigSHA256, Result: v.Result, StartedAt: v.StartedAt, FinishedAt: v.FinishedAt}
			for _, c := range v.Checks {
				row.Checks = append(row.Checks, journal.CheckRow{Name: c.Name, Argv: c.Argv, Status: c.Status,
					ExitCode: c.ExitCode, DurationMS: c.DurationMS, EvidencePath: c.EvidencePath,
					EvidenceSHA256: c.EvidenceSHA256})
			}
			return journal.InsertVerification(ctx, tx, row)
		}); err != nil {
			return err
		}
		if err := p.flush(ctx); err != nil {
			return err
		}
		if v.Result != "passed" {
			return p.runTo(ctx, RunFailed, "verification_failed")
		}
		return p.runTo(ctx, RunReadyForReview, "")
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
	return p.settleRun(ctx, to, reason)
}

// settleRun reports a transition that has committed: the outcome, the
// rendered events and, for a run that ended, its receipt.
func (p *pipeline) settleRun(ctx context.Context, to RunState, reason string) error {
	p.out.State, p.out.Reason = to, reason
	if err := p.flush(ctx); err != nil {
		return err
	}
	if to == RunReadyForReview || to == RunBlocked || to == RunFailed || to == RunCancelled {
		r, err := BuildReceipt(ctx, p.j, p.d.RunID)
		if err != nil {
			return fmt.Errorf("building receipt: %w", err)
		}
		sha, err := WriteReceipt(p.d.RunDir, r)
		if err != nil {
			return fmt.Errorf("writing receipt: %w", err)
		}
		if err := p.append(ctx, "receipt.written", map[string]any{"schema_version": 1, "sha256": sha, "path": "receipt.json"}); err != nil {
			return fmt.Errorf("journaling receipt: %w", err)
		}
	}
	return nil
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
