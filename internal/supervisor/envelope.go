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

// GateLaunch refuses a launch that would exceed the run's finite envelope:
// a past deadline, or one more repair, replan or transport retry than its
// ceiling allows. A refusal is a *GateError naming the blocked reason and
// counts nothing. An allowed attempt after the first is counted as a repair
// or a replan (D8). A ceiling raised by a recorded run.extension_granted
// takes the envelope row's raised value; with no grant, c stays in force
// (I21).
func GateLaunch(ctx context.Context, j *journal.Journal, runID string, c billing.Ceilings) error {
	return gateLaunchAt(ctx, j, runID, c, time.Now())
}

// gateLaunchAt is GateLaunch at an explicit time, so tests need no clock.
func gateLaunchAt(ctx context.Context, j *journal.Journal, runID string, c billing.Ceilings, now time.Time) error {
	env, eff, err := effectiveCeilings(ctx, j, runID, c)
	if err != nil {
		return err
	}
	if deadline, expired, err := envelopeDeadline(env, eff, now); err != nil {
		return err
	} else if expired {
		return &GateError{Reason: "envelope_deadline_exceeded",
			Err: fmt.Errorf("%w: execution deadline %s reached", billing.ErrBudgetExhausted, deadline.Format(time.RFC3339))}
	}
	counts := billing.AttemptCounts{Repairs: int(env.RepairsUsed), Replans: int(env.ReplansUsed), TransportRetries: int(env.TransportRetriesSeen)}
	counted := false
	if _, err := j.LatestAttempt(ctx, runID); err == nil {
		replan, err := IsReplan(ctx, j, runID)
		if err != nil {
			return err
		}
		if replan {
			counts.Replans++
			env.ReplansUsed++
		} else {
			counts.Repairs++
			env.RepairsUsed++
		}
		counted = true
	} else if !errors.Is(err, journal.ErrNotFound) {
		return err
	}
	if err := eff.Check(counts); err != nil {
		return &GateError{Reason: exhaustedReason(eff, counts), Err: err}
	}
	if !counted {
		return nil
	}
	env.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	return j.Transact(ctx, func(tx *sql.Tx) error { return journal.UpsertRunEnvelope(ctx, tx, env) })
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

// executionDeadline is the run's deadline at now, or the zero time before
// the first attempt has started.
func executionDeadline(ctx context.Context, j *journal.Journal, runID string, c billing.Ceilings, now time.Time) (time.Time, error) {
	env, eff, err := effectiveCeilings(ctx, j, runID, c)
	if err != nil {
		return time.Time{}, err
	}
	deadline, _, err := envelopeDeadline(env, eff, now)
	return deadline, err
}

// recordFirstStart stamps the run's first attempt launch, once; the
// execution deadline runs from it.
func recordFirstStart(ctx context.Context, j *journal.Journal, runID string, at time.Time) error {
	env, err := j.RunEnvelope(ctx, runID)
	if err != nil {
		return err
	}
	if env.FirstStartAt != "" {
		return nil
	}
	env.FirstStartAt = at.UTC().Format(time.RFC3339Nano)
	env.UpdatedAt = env.FirstStartAt
	return j.Transact(ctx, func(tx *sql.Tx) error { return journal.UpsertRunEnvelope(ctx, tx, env) })
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
