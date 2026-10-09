package v2contract

import (
	"errors"
	"fmt"
	"slices"
)

// RunState is a run's v2 §6.1 lifecycle state. It is distinct from the v1
// supervisor.RunState, which a later spec migrates onto this vocabulary.
type RunState string

// V2 §6.1 run lifecycle states.
const (
	RunCreated        RunState = "created"
	RunAdmission      RunState = "admission"
	RunPlanning       RunState = "planning"
	RunExecuting      RunState = "executing"
	RunIntegrating    RunState = "integrating"
	RunVerifying      RunState = "verifying"
	RunReadyForReview RunState = "ready_for_review"
	RunApplying       RunState = "applying"
	RunBlocked        RunState = "blocked"
	RunStopping       RunState = "stopping"
	RunInterrupted    RunState = "interrupted"
	RunRecovering     RunState = "recovering"
	RunCompleted      RunState = "completed"
	RunCancelled      RunState = "cancelled"
	RunFailed         RunState = "failed"
)

// TaskState is a task revision's v2 §6.2 lifecycle state.
type TaskState string

// V2 §6.2 task lifecycle states.
const (
	TaskPending    TaskState = "pending"
	TaskReady      TaskState = "ready"
	TaskRunning    TaskState = "running"
	TaskCandidate  TaskState = "candidate"
	TaskVerifying  TaskState = "verifying"
	TaskBlocked    TaskState = "blocked"
	TaskAccepted   TaskState = "accepted"
	TaskFailed     TaskState = "failed"
	TaskCancelled  TaskState = "cancelled"
	TaskSuperseded TaskState = "superseded"
)

// AttemptState is an attempt's v2 §6.2 lifecycle state. It is distinct from
// the v1 supervisor.AttemptState, which a later spec migrates onto this
// vocabulary.
type AttemptState string

// V2 §6.2 attempt lifecycle states.
const (
	AttemptReserved             AttemptState = "reserved"
	AttemptLaunchIntentRecorded AttemptState = "launch_intent_recorded"
	AttemptLaunching            AttemptState = "launching"
	AttemptRunning              AttemptState = "running"
	AttemptWaitingNative        AttemptState = "waiting_native"
	AttemptWaitingApproval      AttemptState = "waiting_approval"
	AttemptStopRequested        AttemptState = "stop_requested"
	AttemptInterrupted          AttemptState = "interrupted"
	AttemptQuarantined          AttemptState = "quarantined"
	AttemptStopped              AttemptState = "stopped"
	AttemptSucceededNative      AttemptState = "succeeded_native"
	AttemptFailedNative         AttemptState = "failed_native"
)

// ErrIllegalTransition reports a lifecycle transition outside the v2
// §6.1–§6.2 tables. Every error wrapping it names the offending from→to
// pair (AC-2.1); unknown states are named as well.
var ErrIllegalTransition = errors.New("v2contract: illegal transition")

// runTransitions is the v2 §6.1 table: explicit from→to edges. The blanket
// "any nonterminal active phase → stopping, interrupted" row and the
// recovering→reconciled-phase rule are applied in CheckRunTransition from
// activeRunPhases. Terminal states map to no targets.
var runTransitions = map[RunState][]RunState{
	RunCreated:        {RunAdmission},
	RunAdmission:      {RunPlanning, RunExecuting, RunBlocked, RunFailed},
	RunPlanning:       {RunExecuting, RunBlocked, RunFailed},
	RunExecuting:      {RunIntegrating, RunVerifying, RunPlanning, RunBlocked, RunFailed},
	RunIntegrating:    {RunVerifying, RunExecuting, RunBlocked, RunFailed},
	RunVerifying:      {RunReadyForReview, RunExecuting, RunBlocked, RunFailed},
	RunReadyForReview: {RunApplying, RunBlocked, RunCompleted, RunCancelled},
	RunApplying:       {RunCompleted, RunBlocked, RunFailed, RunInterrupted},
	RunBlocked:        {RunStopping, RunCancelled, RunFailed},
	RunStopping:       {RunCancelled, RunInterrupted},
	RunInterrupted:    {RunRecovering},
	RunRecovering:     {RunReadyForReview, RunBlocked, RunCancelled, RunFailed, RunInterrupted},
	RunCompleted:      nil,
	RunCancelled:      nil,
	RunFailed:         nil,
}

// activeRunPhases are the v2 §6.1 forward phases: admitted, doing work or
// awaiting review, and outside the stop/recovery protocol. Created is
// pre-admission; blocked, stopping, interrupted and recovering are parking
// or protocol states with their own rows. The blanket stop/interrupt row
// and the recovering→reconciled-phase rule range over exactly this set.
var activeRunPhases = []RunState{
	RunAdmission,
	RunPlanning,
	RunExecuting,
	RunIntegrating,
	RunVerifying,
	RunReadyForReview,
	RunApplying,
}

// taskTransitions is the v2 §6.2 task table. Terminal states map to no
// targets; accepted is nonterminal with a single edge to superseded.
var taskTransitions = map[TaskState][]TaskState{
	TaskPending:    {TaskReady, TaskBlocked, TaskCancelled, TaskSuperseded},
	TaskReady:      {TaskRunning, TaskBlocked, TaskCancelled, TaskSuperseded},
	TaskRunning:    {TaskCandidate, TaskReady, TaskBlocked, TaskFailed, TaskCancelled, TaskSuperseded},
	TaskCandidate:  {TaskVerifying, TaskReady, TaskBlocked, TaskCancelled, TaskSuperseded},
	TaskVerifying:  {TaskAccepted, TaskReady, TaskBlocked, TaskFailed, TaskCancelled, TaskSuperseded},
	TaskBlocked:    {TaskFailed, TaskCancelled, TaskSuperseded},
	TaskAccepted:   {TaskSuperseded},
	TaskFailed:     nil,
	TaskCancelled:  nil,
	TaskSuperseded: nil,
}

// attemptTransitions is the v2 §6.2 attempt table. The running/waiting
// trio moves only to another state in the group (master §6.2: "Another
// state in this group"), never to itself; quarantined→quarantined is an
// explicit row entry, interrupted→interrupted is not.
var attemptTransitions = map[AttemptState][]AttemptState{
	AttemptReserved:             {AttemptLaunchIntentRecorded, AttemptStopped},
	AttemptLaunchIntentRecorded: {AttemptLaunching, AttemptStopped, AttemptInterrupted},
	AttemptLaunching:            {AttemptRunning, AttemptSucceededNative, AttemptFailedNative, AttemptStopRequested, AttemptInterrupted},
	AttemptRunning:              {AttemptWaitingNative, AttemptWaitingApproval, AttemptStopRequested, AttemptSucceededNative, AttemptFailedNative, AttemptInterrupted},
	AttemptWaitingNative:        {AttemptRunning, AttemptWaitingApproval, AttemptStopRequested, AttemptSucceededNative, AttemptFailedNative, AttemptInterrupted},
	AttemptWaitingApproval:      {AttemptRunning, AttemptWaitingNative, AttemptStopRequested, AttemptSucceededNative, AttemptFailedNative, AttemptInterrupted},
	AttemptStopRequested:        {AttemptStopped, AttemptSucceededNative, AttemptFailedNative, AttemptInterrupted, AttemptQuarantined},
	AttemptInterrupted:          {AttemptRunning, AttemptWaitingNative, AttemptWaitingApproval, AttemptStopRequested, AttemptStopped, AttemptSucceededNative, AttemptFailedNative, AttemptQuarantined},
	AttemptQuarantined:          {AttemptRunning, AttemptWaitingNative, AttemptWaitingApproval, AttemptStopRequested, AttemptStopped, AttemptSucceededNative, AttemptFailedNative, AttemptQuarantined},
	AttemptStopped:              nil,
	AttemptSucceededNative:      nil,
	AttemptFailedNative:         nil,
}

func knownRunState(s RunState) bool {
	_, ok := runTransitions[s]
	return ok
}

func knownTaskState(s TaskState) bool {
	_, ok := taskTransitions[s]
	return ok
}

func knownAttemptState(s AttemptState) bool {
	_, ok := attemptTransitions[s]
	return ok
}

func isActiveRunPhase(s RunState) bool {
	return slices.Contains(activeRunPhases, s)
}

// savedRunPhase reports whether s is an eligible blocked-resume phase: a
// nonterminal phase with a →blocked edge in the run table (design §4.1).
func savedRunPhase(s RunState) bool {
	return !IsTerminalRunState(s) && slices.Contains(runTransitions[s], RunBlocked)
}

// savedTaskPhase reports whether s is an eligible blocked-resume phase: a
// nonterminal phase with a →blocked edge in the task table (design §4.1).
func savedTaskPhase(s TaskState) bool {
	return !isTerminalTaskState(s) && slices.Contains(taskTransitions[s], TaskBlocked)
}

func isTerminalTaskState(s TaskState) bool {
	switch s {
	case TaskFailed, TaskCancelled, TaskSuperseded:
		return true
	}
	return false
}

// illegalTransition builds the AC-2.1 error for kind ("run", "task" or
// "attempt"): it always names the offending from→to pair, and names any
// unknown side as well.
func illegalTransition[T ~string](kind string, from, to T, fromKnown, toKnown bool) error {
	pair := kind + " " + string(from) + " -> " + string(to)
	switch {
	case !fromKnown && !toKnown:
		return fmt.Errorf("%w: %s (unknown states %q and %q)", ErrIllegalTransition, pair, from, to)
	case !fromKnown:
		return fmt.Errorf("%w: %s (unknown state %q)", ErrIllegalTransition, pair, from)
	case !toKnown:
		return fmt.Errorf("%w: %s (unknown state %q)", ErrIllegalTransition, pair, to)
	default:
		return fmt.Errorf("%w: %s", ErrIllegalTransition, pair)
	}
}

// CheckRunTransition enforces the v2 §6.1 table. saved is the phase
// recorded when from is blocked: claiming a resume (to == saved) requires
// an eligible saved phase, while the explicit blocked targets pass with
// any other saved value (D8); saved is not consulted away from blocked.
// The caller revalidates the blocker, authority and revisions before
// resuming (master §6.1: "never resume from a remembered enum alone").
func CheckRunTransition(from, to, saved RunState) error {
	fromKnown, toKnown := knownRunState(from), knownRunState(to)
	if !fromKnown || !toKnown {
		return illegalTransition("run", from, to, fromKnown, toKnown)
	}
	if from == RunBlocked {
		if to == saved {
			if !savedRunPhase(saved) {
				return illegalTransition("run", from, to, true, true)
			}
			return nil
		}
		if slices.Contains(runTransitions[from], to) {
			return nil
		}
		return illegalTransition("run", from, to, true, true)
	}
	if slices.Contains(runTransitions[from], to) {
		return nil
	}
	// Master §6.1 blanket row: any nonterminal active phase may stop or
	// be interrupted; recovery reconciles to a runtime-chosen active phase.
	if isActiveRunPhase(from) && (to == RunStopping || to == RunInterrupted) {
		return nil
	}
	if from == RunRecovering && isActiveRunPhase(to) {
		return nil
	}
	return illegalTransition("run", from, to, true, true)
}

// CheckTaskTransition enforces the v2 §6.2 task table; saved behaves as in
// CheckRunTransition, with the task-table eligible set.
func CheckTaskTransition(from, to, saved TaskState) error {
	fromKnown, toKnown := knownTaskState(from), knownTaskState(to)
	if !fromKnown || !toKnown {
		return illegalTransition("task", from, to, fromKnown, toKnown)
	}
	if from == TaskBlocked {
		if to == saved {
			if !savedTaskPhase(saved) {
				return illegalTransition("task", from, to, true, true)
			}
			return nil
		}
		if slices.Contains(taskTransitions[from], to) {
			return nil
		}
		return illegalTransition("task", from, to, true, true)
	}
	if slices.Contains(taskTransitions[from], to) {
		return nil
	}
	return illegalTransition("task", from, to, true, true)
}

// CheckAttemptTransition enforces the v2 §6.2 attempt table; attempts carry
// no saved phase.
func CheckAttemptTransition(from, to AttemptState) error {
	fromKnown, toKnown := knownAttemptState(from), knownAttemptState(to)
	if !fromKnown || !toKnown {
		return illegalTransition("attempt", from, to, fromKnown, toKnown)
	}
	if slices.Contains(attemptTransitions[from], to) {
		return nil
	}
	return illegalTransition("attempt", from, to, true, true)
}

// IsTerminalRunState reports whether s is a terminal v2 §6.1 run state:
// completed, cancelled or failed.
func IsTerminalRunState(s RunState) bool {
	switch s {
	case RunCompleted, RunCancelled, RunFailed:
		return true
	}
	return false
}

// CheckTerminalEntry refuses a terminal run state while ownership is
// unresolved (v2 §6.1; I06). Nonterminal targets ignore ownershipResolved.
// Run the transition check first: it rejects unknown states, which this
// guard passes through as nonterminal. The caller maps a refusal to
// ownership_unresolved.
func CheckTerminalEntry(to RunState, ownershipResolved bool) error {
	if IsTerminalRunState(to) && !ownershipResolved {
		return fmt.Errorf("v2contract: terminal run state %q refused: ownership unresolved", to)
	}
	return nil
}

// CheckReconcileIdentity requires the same launch identity when
// reconciling an interrupted or quarantined attempt (v2 §6.2; I12). It
// checks sameness only; non-empty identities are the Attempt record's
// duty. The caller maps a mismatch to revision_conflict.
func CheckReconcileIdentity(oldLaunchID, newLaunchID string) error {
	if oldLaunchID != newLaunchID {
		return fmt.Errorf("v2contract: reconcile requires launch %q, got %q", oldLaunchID, newLaunchID)
	}
	return nil
}
