package supervisor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
)

// EventExtensionGranted is the control event that records a user's decision
// to raise one envelope ceiling (AC-4.2).
const EventExtensionGranted = "run.extension_granted"

// blockReasons maps each extendable ceiling to the run block it answers.
var blockReasons = map[string]string{
	"execution":         "envelope_deadline_exceeded",
	"repairs":           "envelope_repairs_exhausted",
	"replans":           "envelope_replans_exhausted",
	"transport_retries": "envelope_transport_retries_exhausted",
}

// ExtensionGrant is the operator's decision to raise one ceiling, in the
// envelope row's units: seconds for execution, counts otherwise.
type ExtensionGrant struct {
	Kind      string
	RaisedTo  int64
	DecidedBy string
}

type extensionPayload struct {
	Kind      string `json:"kind"`
	RaisedTo  int64  `json:"raised_to"`
	DecidedBy string `json:"decided_by"`
}

// RequestExtension journals run.extension_granted and raises the run's
// envelope row in one transaction. An unknown kind, a run not blocked for
// that kind, a value that does not exceed the current ceiling or a missing
// decider errors and journals nothing.
func RequestExtension(ctx context.Context, j *journal.Journal, runID, kind string, raisedTo int64, decidedBy string) error {
	reason, ok := blockReasons[kind]
	if !ok {
		return fmt.Errorf("unknown extension kind %q", kind)
	}
	if decidedBy == "" {
		return errors.New("extension needs a decided_by")
	}
	run, err := j.Run(ctx, runID)
	if err != nil {
		return err
	}
	if RunState(run.State) != RunBlocked || run.Reason != reason {
		return fmt.Errorf("run %s is %s (%s), not blocked for %s", runID, run.State, run.Reason, kind)
	}
	env, err := j.RunEnvelope(ctx, runID)
	if err != nil {
		return err
	}
	ceiling := map[string]*int64{"execution": &env.ExecutionSeconds, "repairs": &env.Repairs, "replans": &env.Replans, "transport_retries": &env.TransportRetries}[kind]
	if raisedTo <= *ceiling {
		return fmt.Errorf("extension of %s to %d must exceed the current ceiling %d", kind, raisedTo, *ceiling)
	}
	*ceiling = raisedTo
	now := time.Now().UTC()
	env.UpdatedAt = now.Format(time.RFC3339Nano)
	ev, err := newEvent(runID, "", "", EventExtensionGranted, extensionPayload{Kind: kind, RaisedTo: raisedTo, DecidedBy: decidedBy}, now)
	if err != nil {
		return err
	}
	return NewProducer(ids.New("sup"), 1).append(ctx, j, ev, func(tx *sql.Tx) error {
		return journal.UpsertRunEnvelope(ctx, tx, env)
	})
}

// CheckExtension reports whether a run.extension_granted event for kind
// follows the run's latest block for that kind, so a grant answers one block
// and never a later one. A journal read failure is an error: no grant is
// assumed (I02).
func CheckExtension(ctx context.Context, j *journal.Journal, runID, kind string) (bool, error) {
	reason, ok := blockReasons[kind]
	if !ok {
		return false, fmt.Errorf("unknown extension kind %q", kind)
	}
	events, err := j.Events(ctx, runID, 0)
	if err != nil {
		return false, err
	}
	var lastBlock, lastGrant int64
	for _, ev := range events {
		switch ev.Type {
		case "run.state_changed":
			var p struct {
				State  string  `json:"state"`
				Reason *string `json:"reason"`
			}
			if err := json.Unmarshal(ev.Payload, &p); err != nil {
				return false, fmt.Errorf("decoding %s: %w", ev.EventID, err)
			}
			if p.State == string(RunBlocked) && p.Reason != nil && *p.Reason == reason {
				lastBlock = ev.RunSequence
			}
		case EventExtensionGranted:
			var p extensionPayload
			if err := json.Unmarshal(ev.Payload, &p); err != nil {
				return false, fmt.Errorf("decoding %s: %w", ev.EventID, err)
			}
			if p.Kind == kind {
				lastGrant = ev.RunSequence
			}
		}
	}
	return lastGrant > lastBlock, nil
}

// GateError is a launch the envelope refuses; Reason is the blocked reason.
type GateError struct {
	Reason string
	Err    error
}

func (e *GateError) Error() string { return e.Reason + ": " + e.Err.Error() }
func (e *GateError) Unwrap() error { return e.Err }

// launchPlan is a launch the envelope allows, and what committing it with
// the attempt's launch intent records.
type launchPlan struct {
	kind     string // "repairs", "replans", or "" for the first attempt
	startAt  time.Time
	deadline time.Time
	// replanDeadline is the execution deadline minus the completion reserve,
	// set for replans only; the attempt must still have drafting time when
	// its intent commits.
	replanDeadline time.Time
	ceilings       billing.Ceilings
}

// GateLaunch refuses a launch that would exceed the run's finite envelope: a
// past deadline, or one more repair, replan or transport retry than its
// ceiling allows. A refusal is a *GateError naming the blocked reason. It
// only checks; the launch is counted when its intent commits (planLaunchAt).
// A ceiling raised by a recorded run.extension_granted takes the envelope
// row's raised value; with no grant, c stays in force (I21).
func GateLaunch(ctx context.Context, j *journal.Journal, runID string, c billing.Ceilings) error {
	return gateLaunchAt(ctx, j, runID, c, time.Now())
}

// gateLaunchAt is GateLaunch at an explicit time, so tests need no clock.
func gateLaunchAt(ctx context.Context, j *journal.Journal, runID string, c billing.Ceilings, now time.Time) error {
	_, err := planLaunchAt(ctx, j, runID, c, now)
	return err
}

func deadlineError(deadline time.Time) *GateError {
	return &GateError{Reason: "envelope_deadline_exceeded",
		Err: fmt.Errorf("%w: execution deadline %s reached", billing.ErrBudgetExhausted, deadline.Format(time.RFC3339))}
}

// planLaunchAt checks the launch and returns its plan. An attempt after the
// first is a repair when the run has a verifications row and a replan when
// it has none (D8); the first attempt starts the execution clock.
func planLaunchAt(ctx context.Context, j *journal.Journal, runID string, c billing.Ceilings, now time.Time) (launchPlan, error) {
	env, eff, err := effectiveCeilings(ctx, j, runID, c)
	if err != nil {
		return launchPlan{}, err
	}
	if deadline, expired, err := envelopeDeadline(env, eff, now); err != nil {
		return launchPlan{}, err
	} else if expired {
		return launchPlan{}, deadlineError(deadline)
	}
	plan := launchPlan{startAt: now.UTC(), ceilings: eff}
	counts := billing.AttemptCounts{Repairs: int(env.RepairsUsed), Replans: int(env.ReplansUsed), TransportRetries: int(env.TransportRetriesSeen)}
	if _, err := j.LatestAttempt(ctx, runID); err == nil {
		replan, err := IsReplan(ctx, j, runID)
		if err != nil {
			return launchPlan{}, err
		}
		if replan {
			plan.kind = "replans"
			counts.Replans++
		} else {
			plan.kind = "repairs"
			counts.Repairs++
		}
	} else if !errors.Is(err, journal.ErrNotFound) {
		return launchPlan{}, err
	}
	if err := eff.Check(counts); err != nil {
		return launchPlan{}, &GateError{Reason: exhaustedReason(eff, counts), Err: err}
	}
	// The deadline of a launch that starts the clock runs from now.
	started := env
	if started.FirstStartAt == "" {
		started.FirstStartAt = plan.startAt.Format(time.RFC3339Nano)
	}
	if plan.deadline, _, err = envelopeDeadline(started, eff, now); err != nil {
		return launchPlan{}, err
	}
	return plan, nil
}

// commit counts the planned launch and stamps the first start. It runs in
// the launch intent's transaction, so a launch whose intent fails counts
// nothing, and the update itself enforces the ceiling, so two launches
// planned from the same state cannot both pass (AC-4.2), and a deadline that
// has passed since the plan refuses it. It increments in
// place rather than rewriting the row, leaving concurrent ingest updates of
// the transport count intact.
func (p launchPlan) commit(ctx context.Context, runID string) func(*sql.Tx) error {
	var repairs, replans int
	switch p.kind {
	case "repairs":
		repairs = 1
	case "replans":
		replans = 1
	}
	return func(tx *sql.Tx) error {
		// The plan was checked earlier; a ceiling that ran out since then
		// refuses the launch, rather than admitting a worker the deadline
		// would immediately stop (I21). A replan additionally refuses once
		// its shortened deadline has passed, so a slot is never consumed
		// for an attempt with no drafting time left.
		now := time.Now()
		if !now.Before(p.deadline) {
			return deadlineError(p.deadline)
		}
		if !p.replanDeadline.IsZero() && !now.Before(p.replanDeadline) {
			return deadlineError(p.replanDeadline)
		}
		res, err := tx.ExecContext(ctx, `UPDATE run_envelopes SET repairs_used = repairs_used + ?, replans_used = replans_used + ?,
			first_start_at = COALESCE(first_start_at, ?), updated_at = ?
			WHERE run_id = ? AND repairs_used + ? <= ? AND replans_used + ? <= ?`,
			repairs, replans, p.startAt.Format(time.RFC3339Nano), p.startAt.Format(time.RFC3339Nano),
			runID, repairs, p.ceilings.Repairs, replans, p.ceilings.Replans)
		if err != nil {
			return fmt.Errorf("counting the launch of %s: %w", runID, err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return fmt.Errorf("counting the launch of %s: %w", runID, err)
		} else if n == 0 {
			reason := "envelope_repairs_exhausted"
			if p.kind == "replans" {
				reason = "envelope_replans_exhausted"
			}
			return &GateError{Reason: reason, Err: fmt.Errorf("%w: another launch took the last %s slot of %s", billing.ErrBudgetExhausted, p.kind, runID)}
		}
		return nil
	}
}

// exhaustedReason names the blocked reason of the first kind over its
// ceiling, in the order Ceilings.Check reports them.
func exhaustedReason(c billing.Ceilings, n billing.AttemptCounts) string {
	switch {
	case c.Check(billing.AttemptCounts{Repairs: n.Repairs}) != nil:
		return "envelope_repairs_exhausted"
	case c.Check(billing.AttemptCounts{Replans: n.Replans}) != nil:
		return "envelope_replans_exhausted"
	default:
		return "envelope_transport_retries_exhausted"
	}
}

// effectiveCeilings returns the run's envelope row and the ceilings in force:
// c, with each kind that has a recorded extension taking the row's raised
// value.
func effectiveCeilings(ctx context.Context, j *journal.Journal, runID string, c billing.Ceilings) (journal.EnvelopeRow, billing.Ceilings, error) {
	env, err := j.RunEnvelope(ctx, runID)
	if err != nil {
		return journal.EnvelopeRow{}, c, err
	}
	for kind, apply := range map[string]func(){
		"execution":         func() { c.Execution = time.Duration(env.ExecutionSeconds) * time.Second },
		"repairs":           func() { c.Repairs = int(env.Repairs) },
		"replans":           func() { c.Replans = int(env.Replans) },
		"transport_retries": func() { c.TransportRetries = int(env.TransportRetries) },
	} {
		granted, err := CheckExtension(ctx, j, runID, kind)
		if err != nil {
			return journal.EnvelopeRow{}, c, err
		}
		if granted {
			apply()
		}
	}
	return env, c, nil
}

type pauseSpanJSON struct {
	RequestedAt string `json:"requested_at"`
	QuiescedAt  string `json:"quiesced_at"`
	ResumedAt   string `json:"resumed_at"`
}

// envelopeDeadline returns the execution deadline, once the first attempt
// has started. Before that no clock runs (AC-4.3).
func envelopeDeadline(env journal.EnvelopeRow, c billing.Ceilings, now time.Time) (deadline time.Time, expired bool, err error) {
	if env.FirstStartAt == "" {
		return time.Time{}, false, nil
	}
	first, err := time.Parse(time.RFC3339Nano, env.FirstStartAt)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("envelope first start %q: %w", env.FirstStartAt, err)
	}
	var raw []pauseSpanJSON
	if err := json.Unmarshal([]byte(env.PauseSpans), &raw); err != nil {
		return time.Time{}, false, fmt.Errorf("envelope pause spans: %w", err)
	}
	pauses := make([]billing.PauseSpan, len(raw))
	for i, r := range raw {
		for _, f := range []struct {
			text string
			dst  *time.Time
		}{{r.RequestedAt, &pauses[i].RequestedAt}, {r.QuiescedAt, &pauses[i].QuiescedAt}, {r.ResumedAt, &pauses[i].ResumedAt}} {
			if f.text == "" {
				continue
			}
			if *f.dst, err = time.Parse(time.RFC3339Nano, f.text); err != nil {
				return time.Time{}, false, fmt.Errorf("envelope pause span time %q: %w", f.text, err)
			}
		}
	}
	deadline, expired = billing.Deadline(first, c, pauses, now)
	return deadline, expired, nil
}

// IsReplan reports whether the run's next attempt is a material replan: true
// iff no verifications row exists for the run (D8). A journal read failure
// is an error, never a default; the pipeline blocks the launch (I02).
func IsReplan(ctx context.Context, j *journal.Journal, runID string) (bool, error) {
	_, err := j.LatestVerification(ctx, runID)
	switch {
	case errors.Is(err, journal.ErrNotFound):
		return true, nil
	case err != nil:
		return false, err
	}
	return false, nil
}
