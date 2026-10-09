package v2contract_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

// allRunStates pins the v2 §6.1 run words and their count (15).
var allRunStates = []v2contract.RunState{
	v2contract.RunCreated,
	v2contract.RunAdmission,
	v2contract.RunPlanning,
	v2contract.RunExecuting,
	v2contract.RunIntegrating,
	v2contract.RunVerifying,
	v2contract.RunReadyForReview,
	v2contract.RunApplying,
	v2contract.RunBlocked,
	v2contract.RunStopping,
	v2contract.RunInterrupted,
	v2contract.RunRecovering,
	v2contract.RunCompleted,
	v2contract.RunCancelled,
	v2contract.RunFailed,
}

// allTaskStates pins the v2 §6.2 task words and their count (10).
var allTaskStates = []v2contract.TaskState{
	v2contract.TaskPending,
	v2contract.TaskReady,
	v2contract.TaskRunning,
	v2contract.TaskCandidate,
	v2contract.TaskVerifying,
	v2contract.TaskBlocked,
	v2contract.TaskAccepted,
	v2contract.TaskFailed,
	v2contract.TaskCancelled,
	v2contract.TaskSuperseded,
}

// allAttemptStates pins the v2 §6.2 attempt words and their count (12).
var allAttemptStates = []v2contract.AttemptState{
	v2contract.AttemptReserved,
	v2contract.AttemptLaunchIntentRecorded,
	v2contract.AttemptLaunching,
	v2contract.AttemptRunning,
	v2contract.AttemptWaitingNative,
	v2contract.AttemptWaitingApproval,
	v2contract.AttemptStopRequested,
	v2contract.AttemptInterrupted,
	v2contract.AttemptQuarantined,
	v2contract.AttemptStopped,
	v2contract.AttemptSucceededNative,
	v2contract.AttemptFailedNative,
}

// checkTable asserts every allowed pair passes check and every forbidden pair
// fails with ErrIllegalTransition naming the from→to pair (AC-2.1).
func checkTable[T ~string](t *testing.T, check func(from, to T) error, allowed map[T][]T, forbidden [][2]T) {
	t.Helper()
	for from, tos := range allowed {
		for _, to := range tos {
			if err := check(from, to); err != nil {
				t.Errorf("allowed %s -> %s: unexpected error: %v", from, to, err)
			}
		}
	}
	for _, p := range forbidden {
		err := check(p[0], p[1])
		if err == nil {
			t.Errorf("forbidden %s -> %s: got nil, want ErrIllegalTransition", p[0], p[1])
			continue
		}
		if !errors.Is(err, v2contract.ErrIllegalTransition) {
			t.Errorf("forbidden %s -> %s: error %v does not wrap ErrIllegalTransition", p[0], p[1], err)
		}
		if pair := string(p[0]) + " -> " + string(p[1]); !strings.Contains(err.Error(), pair) {
			t.Errorf("forbidden %s -> %s: error %q does not name the pair", p[0], p[1], err)
		}
	}
}

func TestLifecycleStateWordsMatchSpec(t *testing.T) {
	t.Parallel()
	wantRun := []string{"created", "admission", "planning", "executing", "integrating", "verifying",
		"ready_for_review", "applying", "blocked", "stopping", "interrupted", "recovering",
		"completed", "cancelled", "failed"}
	if len(allRunStates) != len(wantRun) {
		t.Fatalf("run states: got %d, want %d", len(allRunStates), len(wantRun))
	}
	for i, s := range allRunStates {
		if string(s) != wantRun[i] {
			t.Errorf("run state %d: got %q, want %q", i, s, wantRun[i])
		}
	}
	wantTask := []string{"pending", "ready", "running", "candidate", "verifying", "blocked",
		"accepted", "failed", "cancelled", "superseded"}
	if len(allTaskStates) != len(wantTask) {
		t.Fatalf("task states: got %d, want %d", len(allTaskStates), len(wantTask))
	}
	for i, s := range allTaskStates {
		if string(s) != wantTask[i] {
			t.Errorf("task state %d: got %q, want %q", i, s, wantTask[i])
		}
	}
	wantAttempt := []string{"reserved", "launch_intent_recorded", "launching", "running",
		"waiting_native", "waiting_approval", "stop_requested", "interrupted", "quarantined",
		"stopped", "succeeded_native", "failed_native"}
	if len(allAttemptStates) != len(wantAttempt) {
		t.Fatalf("attempt states: got %d, want %d", len(allAttemptStates), len(wantAttempt))
	}
	for i, s := range allAttemptStates {
		if string(s) != wantAttempt[i] {
			t.Errorf("attempt state %d: got %q, want %q", i, s, wantAttempt[i])
		}
	}
}

func TestRunTableMatchesSpec(t *testing.T) {
	t.Parallel()
	check := func(from, to v2contract.RunState) error {
		return v2contract.CheckRunTransition(from, to, "")
	}
	allowed := map[v2contract.RunState][]v2contract.RunState{
		v2contract.RunCreated:        {v2contract.RunAdmission},
		v2contract.RunAdmission:      {v2contract.RunPlanning, v2contract.RunExecuting, v2contract.RunBlocked, v2contract.RunFailed, v2contract.RunStopping, v2contract.RunInterrupted},
		v2contract.RunPlanning:       {v2contract.RunExecuting, v2contract.RunBlocked, v2contract.RunFailed, v2contract.RunStopping, v2contract.RunInterrupted},
		v2contract.RunExecuting:      {v2contract.RunIntegrating, v2contract.RunVerifying, v2contract.RunPlanning, v2contract.RunBlocked, v2contract.RunFailed, v2contract.RunStopping, v2contract.RunInterrupted},
		v2contract.RunIntegrating:    {v2contract.RunVerifying, v2contract.RunExecuting, v2contract.RunBlocked, v2contract.RunFailed, v2contract.RunStopping, v2contract.RunInterrupted},
		v2contract.RunVerifying:      {v2contract.RunReadyForReview, v2contract.RunExecuting, v2contract.RunBlocked, v2contract.RunFailed, v2contract.RunStopping, v2contract.RunInterrupted},
		v2contract.RunReadyForReview: {v2contract.RunApplying, v2contract.RunBlocked, v2contract.RunCompleted, v2contract.RunCancelled, v2contract.RunStopping, v2contract.RunInterrupted},
		v2contract.RunApplying:       {v2contract.RunCompleted, v2contract.RunBlocked, v2contract.RunFailed, v2contract.RunInterrupted, v2contract.RunStopping},
		v2contract.RunBlocked:        {v2contract.RunStopping, v2contract.RunCancelled, v2contract.RunFailed},
		v2contract.RunStopping:       {v2contract.RunCancelled, v2contract.RunInterrupted},
		v2contract.RunInterrupted:    {v2contract.RunRecovering},
		v2contract.RunRecovering:     {v2contract.RunAdmission, v2contract.RunPlanning, v2contract.RunExecuting, v2contract.RunIntegrating, v2contract.RunVerifying, v2contract.RunReadyForReview, v2contract.RunApplying, v2contract.RunBlocked, v2contract.RunCancelled, v2contract.RunFailed, v2contract.RunInterrupted},
	}
	forbidden := [][2]v2contract.RunState{
		{v2contract.RunCreated, v2contract.RunVerifying},
		{v2contract.RunCreated, v2contract.RunExecuting},
		{v2contract.RunCreated, v2contract.RunCompleted},
		{v2contract.RunCreated, v2contract.RunCreated},
		{v2contract.RunAdmission, v2contract.RunVerifying},
		{v2contract.RunAdmission, v2contract.RunReadyForReview},
		{v2contract.RunPlanning, v2contract.RunVerifying},
		{v2contract.RunPlanning, v2contract.RunPlanning},
		{v2contract.RunExecuting, v2contract.RunCompleted},
		{v2contract.RunExecuting, v2contract.RunCancelled},
		{v2contract.RunExecuting, v2contract.RunReadyForReview},
		{v2contract.RunExecuting, v2contract.RunExecuting},
		{v2contract.RunIntegrating, v2contract.RunCompleted},
		{v2contract.RunIntegrating, v2contract.RunReadyForReview},
		{v2contract.RunVerifying, v2contract.RunCompleted},
		{v2contract.RunVerifying, v2contract.RunApplying},
		{v2contract.RunReadyForReview, v2contract.RunExecuting},
		{v2contract.RunReadyForReview, v2contract.RunFailed},
		{v2contract.RunReadyForReview, v2contract.RunVerifying},
		{v2contract.RunApplying, v2contract.RunExecuting},
		{v2contract.RunApplying, v2contract.RunCancelled},
		{v2contract.RunApplying, v2contract.RunApplying},
		{v2contract.RunBlocked, v2contract.RunExecuting},
		{v2contract.RunBlocked, v2contract.RunBlocked},
		{v2contract.RunBlocked, v2contract.RunAdmission},
		{v2contract.RunBlocked, v2contract.RunCompleted},
		{v2contract.RunStopping, v2contract.RunBlocked},
		{v2contract.RunStopping, v2contract.RunExecuting},
		{v2contract.RunStopping, v2contract.RunStopping},
		{v2contract.RunInterrupted, v2contract.RunExecuting},
		{v2contract.RunInterrupted, v2contract.RunBlocked},
		{v2contract.RunInterrupted, v2contract.RunInterrupted},
		{v2contract.RunRecovering, v2contract.RunStopping},
		{v2contract.RunRecovering, v2contract.RunCreated},
		{v2contract.RunRecovering, v2contract.RunRecovering},
		{v2contract.RunCompleted, v2contract.RunExecuting},
		{v2contract.RunCompleted, v2contract.RunCompleted},
		{v2contract.RunCancelled, v2contract.RunBlocked},
		{v2contract.RunFailed, v2contract.RunFailed},
		{"bogus", v2contract.RunExecuting},
		{v2contract.RunExecuting, "bogus"},
		{"bogus", "bogus"},
	}
	checkTable(t, check, allowed, forbidden)
}

func TestTaskTableMatchesSpec(t *testing.T) {
	t.Parallel()
	check := func(from, to v2contract.TaskState) error {
		return v2contract.CheckTaskTransition(from, to, "")
	}
	allowed := map[v2contract.TaskState][]v2contract.TaskState{
		v2contract.TaskPending:   {v2contract.TaskReady, v2contract.TaskBlocked, v2contract.TaskCancelled, v2contract.TaskSuperseded},
		v2contract.TaskReady:     {v2contract.TaskRunning, v2contract.TaskBlocked, v2contract.TaskCancelled, v2contract.TaskSuperseded},
		v2contract.TaskRunning:   {v2contract.TaskCandidate, v2contract.TaskReady, v2contract.TaskBlocked, v2contract.TaskFailed, v2contract.TaskCancelled, v2contract.TaskSuperseded},
		v2contract.TaskCandidate: {v2contract.TaskVerifying, v2contract.TaskReady, v2contract.TaskBlocked, v2contract.TaskCancelled, v2contract.TaskSuperseded},
		v2contract.TaskVerifying: {v2contract.TaskAccepted, v2contract.TaskReady, v2contract.TaskBlocked, v2contract.TaskFailed, v2contract.TaskCancelled, v2contract.TaskSuperseded},
		v2contract.TaskBlocked:   {v2contract.TaskFailed, v2contract.TaskCancelled, v2contract.TaskSuperseded},
		v2contract.TaskAccepted:  {v2contract.TaskSuperseded},
	}
	forbidden := [][2]v2contract.TaskState{
		{v2contract.TaskAccepted, v2contract.TaskReady},
		{v2contract.TaskPending, v2contract.TaskAccepted},
		{v2contract.TaskPending, v2contract.TaskRunning},
		{v2contract.TaskReady, v2contract.TaskCandidate},
		{v2contract.TaskRunning, v2contract.TaskAccepted},
		{v2contract.TaskRunning, v2contract.TaskVerifying},
		{v2contract.TaskCandidate, v2contract.TaskAccepted},
		{v2contract.TaskCandidate, v2contract.TaskCandidate},
		{v2contract.TaskVerifying, v2contract.TaskCandidate},
		{v2contract.TaskVerifying, v2contract.TaskVerifying},
		{v2contract.TaskAccepted, v2contract.TaskAccepted},
		{v2contract.TaskAccepted, v2contract.TaskFailed},
		{v2contract.TaskBlocked, v2contract.TaskRunning},
		{v2contract.TaskBlocked, v2contract.TaskBlocked},
		{v2contract.TaskBlocked, v2contract.TaskAccepted},
		{v2contract.TaskFailed, v2contract.TaskPending},
		{v2contract.TaskFailed, v2contract.TaskFailed},
		{v2contract.TaskCancelled, v2contract.TaskCancelled},
		{v2contract.TaskSuperseded, v2contract.TaskReady},
		{v2contract.TaskSuperseded, v2contract.TaskSuperseded},
		{"bogus", v2contract.TaskReady},
		{v2contract.TaskReady, "bogus"},
	}
	checkTable(t, check, allowed, forbidden)
}

func TestAttemptTableMatchesSpec(t *testing.T) {
	t.Parallel()
	check := v2contract.CheckAttemptTransition
	runningGroup := func(self v2contract.AttemptState) []v2contract.AttemptState {
		others := []v2contract.AttemptState{}
		for _, s := range []v2contract.AttemptState{v2contract.AttemptRunning, v2contract.AttemptWaitingNative, v2contract.AttemptWaitingApproval} {
			if s != self {
				others = append(others, s)
			}
		}
		return append(others, v2contract.AttemptStopRequested, v2contract.AttemptSucceededNative, v2contract.AttemptFailedNative, v2contract.AttemptInterrupted)
	}
	reconciled := []v2contract.AttemptState{v2contract.AttemptRunning, v2contract.AttemptWaitingNative, v2contract.AttemptWaitingApproval, v2contract.AttemptStopRequested, v2contract.AttemptStopped, v2contract.AttemptSucceededNative, v2contract.AttemptFailedNative, v2contract.AttemptQuarantined}
	allowed := map[v2contract.AttemptState][]v2contract.AttemptState{
		v2contract.AttemptReserved:             {v2contract.AttemptLaunchIntentRecorded, v2contract.AttemptStopped},
		v2contract.AttemptLaunchIntentRecorded: {v2contract.AttemptLaunching, v2contract.AttemptStopped, v2contract.AttemptInterrupted},
		v2contract.AttemptLaunching:            {v2contract.AttemptRunning, v2contract.AttemptSucceededNative, v2contract.AttemptFailedNative, v2contract.AttemptStopRequested, v2contract.AttemptInterrupted},
		v2contract.AttemptRunning:              runningGroup(v2contract.AttemptRunning),
		v2contract.AttemptWaitingNative:        runningGroup(v2contract.AttemptWaitingNative),
		v2contract.AttemptWaitingApproval:      runningGroup(v2contract.AttemptWaitingApproval),
		v2contract.AttemptStopRequested:        {v2contract.AttemptStopped, v2contract.AttemptSucceededNative, v2contract.AttemptFailedNative, v2contract.AttemptInterrupted, v2contract.AttemptQuarantined},
		v2contract.AttemptInterrupted:          reconciled,
		v2contract.AttemptQuarantined:          reconciled,
	}
	forbidden := [][2]v2contract.AttemptState{
		{v2contract.AttemptRunning, v2contract.AttemptReserved},
		{v2contract.AttemptReserved, v2contract.AttemptRunning},
		{v2contract.AttemptReserved, v2contract.AttemptReserved},
		{v2contract.AttemptReserved, v2contract.AttemptLaunching},
		{v2contract.AttemptLaunchIntentRecorded, v2contract.AttemptRunning},
		{v2contract.AttemptLaunching, v2contract.AttemptWaitingNative},
		{v2contract.AttemptLaunching, v2contract.AttemptLaunching},
		{v2contract.AttemptRunning, v2contract.AttemptRunning},
		{v2contract.AttemptWaitingNative, v2contract.AttemptWaitingNative},
		{v2contract.AttemptWaitingApproval, v2contract.AttemptWaitingApproval},
		{v2contract.AttemptRunning, v2contract.AttemptStopped},
		{v2contract.AttemptWaitingNative, v2contract.AttemptStopped},
		{v2contract.AttemptStopRequested, v2contract.AttemptRunning},
		{v2contract.AttemptStopRequested, v2contract.AttemptStopRequested},
		{v2contract.AttemptStopRequested, v2contract.AttemptLaunching},
		{v2contract.AttemptInterrupted, v2contract.AttemptInterrupted},
		{v2contract.AttemptInterrupted, v2contract.AttemptLaunching},
		{v2contract.AttemptInterrupted, v2contract.AttemptReserved},
		{v2contract.AttemptQuarantined, v2contract.AttemptLaunching},
		{v2contract.AttemptQuarantined, v2contract.AttemptInterrupted},
		{v2contract.AttemptStopped, v2contract.AttemptRunning},
		{v2contract.AttemptStopped, v2contract.AttemptStopped},
		{v2contract.AttemptSucceededNative, v2contract.AttemptSucceededNative},
		{v2contract.AttemptFailedNative, v2contract.AttemptQuarantined},
		{"bogus", v2contract.AttemptRunning},
		{v2contract.AttemptRunning, "bogus"},
	}
	checkTable(t, check, allowed, forbidden)
}

func TestBlockedResumeRequiresSaved(t *testing.T) {
	t.Parallel()
	runCases := []struct {
		name  string
		to    v2contract.RunState
		saved v2contract.RunState
		want  bool
	}{
		{"resume to saved phase passes", v2contract.RunExecuting, v2contract.RunExecuting, true},
		{"resume to other phase fails", v2contract.RunExecuting, v2contract.RunVerifying, false},
		{"stop needs no saved phase", v2contract.RunStopping, "", true},
		{"stop ignores unrelated saved", v2contract.RunStopping, v2contract.RunExecuting, true},
		{"stop claimed as resume fails", v2contract.RunStopping, v2contract.RunStopping, false},
		{"completed claimed as resume fails", v2contract.RunCompleted, v2contract.RunCompleted, false},
		{"terminal saved fails even when to == saved", v2contract.RunFailed, v2contract.RunFailed, false},
		{"cancel needs no saved phase", v2contract.RunCancelled, "", true},
		{"cancel claimed as resume fails", v2contract.RunCancelled, v2contract.RunCancelled, false},
		{"unknown saved fails even when to == saved", "bogus", "bogus", false},
		{"resume to recovering passes", v2contract.RunRecovering, v2contract.RunRecovering, true},
		{"resume to ready_for_review passes", v2contract.RunReadyForReview, v2contract.RunReadyForReview, true},
		{"created was never a saved phase", v2contract.RunCreated, v2contract.RunCreated, false},
		{"blocked was never a saved phase", v2contract.RunBlocked, v2contract.RunBlocked, false},
	}
	for _, tc := range runCases {
		err := v2contract.CheckRunTransition(v2contract.RunBlocked, tc.to, tc.saved)
		if tc.want && err != nil {
			t.Errorf("%s: blocked -> %s (saved %s): unexpected error: %v", tc.name, tc.to, tc.saved, err)
		}
		if !tc.want {
			if err == nil {
				t.Errorf("%s: blocked -> %s (saved %s): got nil, want ErrIllegalTransition", tc.name, tc.to, tc.saved)
				continue
			}
			if !errors.Is(err, v2contract.ErrIllegalTransition) {
				t.Errorf("%s: error %v does not wrap ErrIllegalTransition", tc.name, err)
			}
		}
	}
	// Eligible saved run phases are exactly the nonterminal phases with a
	// →blocked edge (design §4.1); resuming to each passes iff eligible.
	eligibleRun := map[v2contract.RunState]bool{
		v2contract.RunAdmission: true, v2contract.RunPlanning: true, v2contract.RunExecuting: true,
		v2contract.RunIntegrating: true, v2contract.RunVerifying: true, v2contract.RunReadyForReview: true,
		v2contract.RunApplying: true, v2contract.RunRecovering: true,
	}
	for _, s := range allRunStates {
		err := v2contract.CheckRunTransition(v2contract.RunBlocked, s, s)
		if eligibleRun[s] && err != nil {
			t.Errorf("blocked -> %s (saved %s): eligible phase refused: %v", s, s, err)
		}
		if !eligibleRun[s] && err == nil {
			t.Errorf("blocked -> %s (saved %s): ineligible phase accepted", s, s)
		}
	}
	taskCases := []struct {
		name  string
		to    v2contract.TaskState
		saved v2contract.TaskState
		want  bool
	}{
		{"resume to saved phase passes", v2contract.TaskRunning, v2contract.TaskRunning, true},
		{"resume to other phase fails", v2contract.TaskRunning, v2contract.TaskReady, false},
		{"fail needs no saved phase", v2contract.TaskFailed, "", true},
		{"fail claimed as resume fails", v2contract.TaskFailed, v2contract.TaskFailed, false},
		{"supersede ignores unrelated saved", v2contract.TaskSuperseded, v2contract.TaskAccepted, true},
		{"accepted was never a saved phase", v2contract.TaskAccepted, v2contract.TaskAccepted, false},
		{"resume to candidate passes", v2contract.TaskCandidate, v2contract.TaskCandidate, true},
		{"unknown saved fails even when to == saved", "bogus", "bogus", false},
	}
	for _, tc := range taskCases {
		err := v2contract.CheckTaskTransition(v2contract.TaskBlocked, tc.to, tc.saved)
		if tc.want && err != nil {
			t.Errorf("%s: blocked -> %s (saved %s): unexpected error: %v", tc.name, tc.to, tc.saved, err)
		}
		if !tc.want && err == nil {
			t.Errorf("%s: blocked -> %s (saved %s): got nil, want ErrIllegalTransition", tc.name, tc.to, tc.saved)
		}
	}
	eligibleTask := map[v2contract.TaskState]bool{
		v2contract.TaskPending: true, v2contract.TaskReady: true, v2contract.TaskRunning: true,
		v2contract.TaskCandidate: true, v2contract.TaskVerifying: true,
	}
	for _, s := range allTaskStates {
		err := v2contract.CheckTaskTransition(v2contract.TaskBlocked, s, s)
		if eligibleTask[s] && err != nil {
			t.Errorf("blocked -> %s (saved %s): eligible phase refused: %v", s, s, err)
		}
		if !eligibleTask[s] && err == nil {
			t.Errorf("blocked -> %s (saved %s): ineligible phase accepted", s, s)
		}
	}
	// Away from blocked, saved is not consulted.
	if err := v2contract.CheckRunTransition(v2contract.RunExecuting, v2contract.RunVerifying, "bogus"); err != nil {
		t.Errorf("non-blocked run transition consults saved: %v", err)
	}
	if err := v2contract.CheckTaskTransition(v2contract.TaskRunning, v2contract.TaskCandidate, "bogus"); err != nil {
		t.Errorf("non-blocked task transition consults saved: %v", err)
	}
}

func TestTerminalEntryRefusesUnresolved(t *testing.T) {
	t.Parallel()
	terminal := []v2contract.RunState{v2contract.RunCompleted, v2contract.RunCancelled, v2contract.RunFailed}
	for _, s := range terminal {
		if err := v2contract.CheckTerminalEntry(s, false); err == nil {
			t.Errorf("terminal %s with unresolved ownership: got nil, want refusal", s)
		} else if !strings.Contains(err.Error(), string(s)) {
			t.Errorf("terminal %s refusal %q does not name the state", s, err)
		}
		if err := v2contract.CheckTerminalEntry(s, true); err != nil {
			t.Errorf("terminal %s with resolved ownership: unexpected error: %v", s, err)
		}
	}
	for _, s := range allRunStates {
		terminal := s == v2contract.RunCompleted || s == v2contract.RunCancelled || s == v2contract.RunFailed
		if v2contract.IsTerminalRunState(s) != terminal {
			t.Errorf("IsTerminalRunState(%s) = %v, want %v", s, !terminal, terminal)
		}
		if terminal {
			continue
		}
		// AC-2.2: nonterminal targets ignore the ownership flag.
		if err := v2contract.CheckTerminalEntry(s, false); err != nil {
			t.Errorf("nonterminal %s with unresolved ownership: unexpected error: %v", s, err)
		}
		if err := v2contract.CheckTerminalEntry(s, true); err != nil {
			t.Errorf("nonterminal %s with resolved ownership: unexpected error: %v", s, err)
		}
	}
}

func TestReconcileRequiresSameLaunch(t *testing.T) {
	t.Parallel()
	if err := v2contract.CheckReconcileIdentity("lch_old", "lch_old"); err != nil {
		t.Errorf("same launch identity: unexpected error: %v", err)
	}
	if err := v2contract.CheckReconcileIdentity("lch_old", "lch_new"); err == nil {
		t.Error("mismatched launch identities: got nil, want refusal")
	} else if !strings.Contains(err.Error(), "lch_old") || !strings.Contains(err.Error(), "lch_new") {
		t.Errorf("launch mismatch %q does not name both identities", err)
	}
	// Emptiness is owned by Attempt validation (Task 2); this guard checks
	// sameness only.
	if err := v2contract.CheckReconcileIdentity("", ""); err != nil {
		t.Errorf("equal empty identities: unexpected error: %v", err)
	}
}
