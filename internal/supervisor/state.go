// Package supervisor owns run and attempt state (design §4, §7.2): it is the
// only code that moves either through its state machine.
package supervisor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
)

// RunState is a run's lifecycle state (design §4).
type RunState string

const (
	RunCreated        RunState = "created"
	RunAdmission      RunState = "admission"
	RunExecuting      RunState = "executing"
	RunVerifying      RunState = "verifying"
	RunReadyForReview RunState = "ready_for_review"
	RunApplying       RunState = "applying"
	RunCompleted      RunState = "completed"
	RunStopping       RunState = "stopping"
	RunCancelled      RunState = "cancelled"
	RunInterrupted    RunState = "interrupted"
	RunRecovering     RunState = "recovering"
	RunBlocked        RunState = "blocked"
	RunFailed         RunState = "failed"
)

// runTransitions is design §4's table. blocked, failed, cancelled and
// completed are terminal. planning and integrating are never entered (N4).
var runTransitions = map[RunState][]RunState{
	RunCreated:        {RunAdmission},
	RunAdmission:      {RunExecuting, RunBlocked, RunFailed},
	RunExecuting:      {RunVerifying, RunFailed, RunBlocked, RunStopping, RunInterrupted},
	RunVerifying:      {RunReadyForReview, RunFailed},
	RunReadyForReview: {RunApplying},
	RunApplying:       {RunCompleted, RunBlocked},
	RunStopping:       {RunCancelled, RunInterrupted},
	RunInterrupted:    {RunRecovering},
	RunRecovering:     {RunExecuting, RunVerifying, RunFailed, RunInterrupted},
}

// AttemptState is an attempt's lifecycle state (design §4). reserved,
// waiting_native and waiting_approval are never entered (N10).
type AttemptState string

const (
	AttemptLaunchIntentRecorded AttemptState = "launch_intent_recorded"
	AttemptLaunching            AttemptState = "launching"
	AttemptRunning              AttemptState = "running"
	AttemptSucceededNative      AttemptState = "succeeded_native"
	AttemptFailedNative         AttemptState = "failed_native"
	AttemptStopRequested        AttemptState = "stop_requested"
	AttemptStopped              AttemptState = "stopped"
	AttemptInterrupted          AttemptState = "interrupted"
	AttemptQuarantined          AttemptState = "quarantined"
)

// attemptTransitions is design §4's attempt chain. An attempt whose worker is
// lost before its native exit is confirmed becomes interrupted from any
// non-terminal state (§7.3), including a crash between launch intent and
// acknowledgement (AC-11.3). A stop request, once recorded, can end only in
// stopped or interrupted, never in a native success or failure (AC-5.7).
var attemptTransitions = map[AttemptState][]AttemptState{
	AttemptLaunchIntentRecorded: {AttemptLaunching, AttemptInterrupted},
	AttemptLaunching:            {AttemptRunning, AttemptInterrupted},
	AttemptRunning:              {AttemptSucceededNative, AttemptFailedNative, AttemptStopRequested, AttemptInterrupted},
	AttemptStopRequested:        {AttemptStopped, AttemptInterrupted},
	AttemptInterrupted:          {AttemptQuarantined},
}

// ErrIllegalTransition reports a transition the state machine does not
// allow, or a reason the target state does not accept. Nothing is journaled.
var ErrIllegalTransition = errors.New("illegal state transition")

// checkReason keeps outcome labels truthful (I06): a state that records a
// failure or an unresolved condition must say why, and ready_for_review is
// either fully verified (no reason) or unverified.
func checkReason(to, reason string) error {
	switch to {
	case string(RunBlocked), string(RunFailed), string(RunInterrupted), string(AttemptFailedNative):
		if reason == "" {
			return fmt.Errorf("%w: %s requires a reason", ErrIllegalTransition, to)
		}
	case string(RunReadyForReview):
		if reason != "" && reason != "unverified" {
			return fmt.Errorf("%w: ready_for_review reason must be empty or unverified, got %q", ErrIllegalTransition, reason)
		}
	}
	return nil
}

// Producer is one event source's identity and sequence counter (§7.6). It
// is safe for concurrent use: its appends are serialised, and a sequence
// number is spent only when its event commits, so a failed or rejected
// transition never leaves a gap.
type Producer struct {
	id         string
	generation int64

	mu   sync.Mutex
	last int64
}

// NewProducer returns a producer whose first event has sequence 1.
func NewProducer(id string, generation int64) *Producer {
	return &Producer{id: id, generation: generation}
}

func (p *Producer) append(ctx context.Context, j *journal.Journal, ev journal.Event, project func(*sql.Tx) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	ev.ProducerID, ev.Generation, ev.ProducerSequence = p.id, p.generation, p.last+1
	if err := j.Append(ctx, ev, project); err != nil {
		return err
	}
	p.last++
	return nil
}

func newEvent(runID, taskID, attemptID, typ string, payload any, at time.Time) (journal.Event, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return journal.Event{}, fmt.Errorf("encoding %s payload: %w", typ, err)
	}
	return journal.Event{
		SchemaVersion: journal.EnvelopeVersion,
		EventID:       ids.New("evt"),
		RunID:         runID,
		TaskID:        taskID,
		AttemptID:     attemptID,
		ObservedAt:    at,
		Type:          typ,
		Payload:       b,
	}, nil
}

// statePayload is the {state, reason} payload; an empty reason is an
// explicit null.
type statePayload struct {
	State  string  `json:"state"`
	Reason *string `json:"reason"`
}

func stateChange(state, reason string) statePayload {
	p := statePayload{State: state}
	if reason != "" {
		p.Reason = &reason
	}
	return p
}

// CreateRun journals run.created and inserts run's projection in state
// created. It sets run's State, Reason, CreatedAt and UpdatedAt.
func CreateRun(ctx context.Context, j *journal.Journal, run journal.RunRow, producer *Producer) error {
	now := time.Now().UTC()
	run.State, run.Reason, run.CreatedAt, run.UpdatedAt = string(RunCreated), "", now, now
	ev, err := newEvent(run.RunID, "", "", "run.created", stateChange(run.State, ""), now)
	if err != nil {
		return err
	}
	return producer.append(ctx, j, ev, func(tx *sql.Tx) error {
		return journal.InsertRun(ctx, tx, run)
	})
}

// TransitionRun moves runID to state to, journaling run.state_changed and
// updating the projection in one transaction (AC-9.3). The current state is
// read inside that transaction, so concurrent transitions serialise and an
// illegal one (ErrIllegalTransition) journals nothing.
func TransitionRun(ctx context.Context, j *journal.Journal, runID string, to RunState, reason string, producer *Producer) error {
	if err := checkReason(string(to), reason); err != nil {
		return err
	}
	now := time.Now().UTC()
	ev, err := newEvent(runID, "", "", "run.state_changed", stateChange(string(to), reason), now)
	if err != nil {
		return err
	}
	var illegal error
	err = producer.append(ctx, j, ev, func(tx *sql.Tx) error {
		from, err := journal.CurrentRunState(ctx, tx, runID)
		if err != nil {
			return err
		}
		if !slices.Contains(runTransitions[RunState(from)], to) {
			illegal = fmt.Errorf("%w: run %s cannot move from %s to %s", ErrIllegalTransition, runID, from, to)
			return illegal
		}
		return journal.SetRunState(ctx, tx, runID, string(to), reason, now)
	})
	if illegal != nil {
		return illegal
	}
	return err
}

// launchIntentPayload is attempt.launch_intent_recorded's payload.
type launchIntentPayload struct {
	LaunchTokenSHA256 string `json:"launch_token_sha256"`
}

// RecordLaunchIntent journals attempt.launch_intent_recorded and inserts the
// attempt's projection in state launch_intent_recorded, before any worker is
// spawned (AC-5.1). It sets attempt's State and Reason.
func RecordLaunchIntent(ctx context.Context, j *journal.Journal, attempt journal.AttemptRow, producer *Producer) error {
	attempt.State, attempt.Reason = string(AttemptLaunchIntentRecorded), ""
	ev, err := newEvent(attempt.RunID, attempt.TaskID, attempt.AttemptID, "attempt.launch_intent_recorded",
		launchIntentPayload{LaunchTokenSHA256: attempt.LaunchTokenSHA256}, time.Now().UTC())
	if err != nil {
		return err
	}
	return producer.append(ctx, j, ev, func(tx *sql.Tx) error {
		return journal.InsertAttempt(ctx, tx, attempt)
	})
}

// TransitionAttempt moves attemptID to state to, journaling
// attempt.state_changed and updating the projection in one transaction, with
// the same guarantees as TransitionRun.
func TransitionAttempt(ctx context.Context, j *journal.Journal, attemptID string, to AttemptState, reason string, producer *Producer) error {
	if err := checkReason(string(to), reason); err != nil {
		return err
	}
	// run_id and task_id never change, so reading them before the
	// transaction is safe; the state is checked inside it.
	a, err := j.Attempt(ctx, attemptID)
	if err != nil {
		return err
	}
	ev, err := newEvent(a.RunID, a.TaskID, attemptID, "attempt.state_changed", stateChange(string(to), reason), time.Now().UTC())
	if err != nil {
		return err
	}
	var illegal error
	err = producer.append(ctx, j, ev, func(tx *sql.Tx) error {
		from, err := journal.CurrentAttemptState(ctx, tx, attemptID)
		if err != nil {
			return err
		}
		if !slices.Contains(attemptTransitions[AttemptState(from)], to) {
			illegal = fmt.Errorf("%w: attempt %s cannot move from %s to %s", ErrIllegalTransition, attemptID, from, to)
			return illegal
		}
		return journal.SetAttemptState(ctx, tx, attemptID, string(to), reason)
	})
	if illegal != nil {
		return illegal
	}
	return err
}
