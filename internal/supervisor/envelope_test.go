package supervisor

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
)

type envelopeWorld struct {
	j     *journal.Journal
	runID string
	prod  *Producer
}

// blockedRun is a run journaled through admission to blocked with reason,
// carrying a stored envelope of 30 minutes, 3 repairs, 2 replans and 5
// transport retries.
func blockedRun(t *testing.T, reason string) envelopeWorld {
	t.Helper()
	j, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	w := envelopeWorld{j: j, runID: ids.New("run"), prod: NewProducer(ids.New("sup"), 1)}
	run := journal.RunRow{RunID: w.runID, AdapterID: "builtin/fake", SourceRepo: "/tmp/repo", TaskSHA256: "t",
		BillingPosture: "local-scripted", ExecutionProfile: "trusted-host"}
	if err := CreateRun(t.Context(), j, run, w.prod); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		to     RunState
		reason string
	}{{RunAdmission, ""}, {RunBlocked, reason}} {
		if err := TransitionRun(t.Context(), j, w.runID, step.to, step.reason, w.prod); err != nil {
			t.Fatal(err)
		}
	}
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.UpsertRunEnvelope(t.Context(), tx, journal.EnvelopeRow{RunID: w.runID, ExecutionSeconds: 1800,
			Repairs: 3, Replans: 2, TransportRetries: 5, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	}); err != nil {
		t.Fatal(err)
	}
	return w
}

func (w envelopeWorld) extensions(t *testing.T) []journal.Event {
	t.Helper()
	evs, err := w.j.Events(t.Context(), w.runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []journal.Event
	for _, ev := range evs {
		if ev.Type == EventExtensionGranted {
			out = append(out, ev)
		}
	}
	return out
}

func TestExtensionGrantsRecordedDecision(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		kind, reason string
		raisedTo     int64
		check        func(journal.EnvelopeRow) int64
	}{
		{"execution", "envelope_deadline_exceeded", 3600, func(e journal.EnvelopeRow) int64 { return e.ExecutionSeconds }},
		{"repairs", "envelope_repairs_exhausted", 6, func(e journal.EnvelopeRow) int64 { return e.Repairs }},
		{"replans", "envelope_replans_exhausted", 4, func(e journal.EnvelopeRow) int64 { return e.Replans }},
		{"transport_retries", "envelope_transport_retries_exhausted", 9, func(e journal.EnvelopeRow) int64 { return e.TransportRetries }},
	} {
		t.Run(c.kind, func(t *testing.T) {
			t.Parallel()
			w := blockedRun(t, c.reason)
			before, err := w.j.RunEnvelope(t.Context(), w.runID)
			if err != nil {
				t.Fatal(err)
			}
			if err := RequestExtension(t.Context(), w.j, w.runID, c.kind, c.raisedTo, "operator"); err != nil {
				t.Fatal(err)
			}
			evs := w.extensions(t)
			if len(evs) != 1 {
				t.Fatalf("run.extension_granted events = %d, want 1", len(evs))
			}
			var p struct {
				Kind      string `json:"kind"`
				RaisedTo  int64  `json:"raised_to"`
				DecidedBy string `json:"decided_by"`
			}
			if err := json.Unmarshal(evs[0].Payload, &p); err != nil {
				t.Fatal(err)
			}
			if p.Kind != c.kind || p.RaisedTo != c.raisedTo || p.DecidedBy != "operator" {
				t.Fatalf("payload = %+v, want kind %s raised_to %d decided_by operator", p, c.kind, c.raisedTo)
			}
			after, err := w.j.RunEnvelope(t.Context(), w.runID)
			if err != nil {
				t.Fatal(err)
			}
			if got := c.check(after); got != c.raisedTo {
				t.Fatalf("envelope %s = %d, want %d", c.kind, got, c.raisedTo)
			}
			// Only the extended ceiling moves.
			if c.check(before) == c.check(after) {
				t.Fatalf("envelope %s did not change", c.kind)
			}
			after.UpdatedAt, before.UpdatedAt = "", ""
			switch c.kind {
			case "execution":
				after.ExecutionSeconds = before.ExecutionSeconds
			case "repairs":
				after.Repairs = before.Repairs
			case "replans":
				after.Replans = before.Replans
			case "transport_retries":
				after.TransportRetries = before.TransportRetries
			}
			if after != before {
				t.Fatalf("envelope changed beyond %s: %+v -> %+v", c.kind, before, after)
			}
			ok, err := CheckExtension(t.Context(), w.j, w.runID, c.kind)
			if err != nil || !ok {
				t.Fatalf("CheckExtension after grant = %v, %v; want true", ok, err)
			}
		})
	}
}

func TestCheckExtensionFailsClosed(t *testing.T) {
	t.Parallel()
	w := blockedRun(t, "envelope_repairs_exhausted")
	if ok, err := CheckExtension(t.Context(), w.j, w.runID, "repairs"); err != nil || ok {
		t.Fatalf("no event: CheckExtension = %v, %v; want false, nil", ok, err)
	}
	if err := RequestExtension(t.Context(), w.j, w.runID, "repairs", 4, "operator"); err != nil {
		t.Fatal(err)
	}
	// A grant for one kind never covers another.
	if ok, err := CheckExtension(t.Context(), w.j, w.runID, "replans"); err != nil || ok {
		t.Fatalf("other kind: CheckExtension = %v, %v; want false, nil", ok, err)
	}
	// A later block for the same kind needs a fresh decision: the grant
	// binds the block it answered.
	ev, err := newEvent(w.runID, "", "", "run.state_changed", stateChange(string(RunBlocked), "envelope_repairs_exhausted"), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := w.prod.append(t.Context(), w.j, ev, nil); err != nil {
		t.Fatal(err)
	}
	if ok, err := CheckExtension(t.Context(), w.j, w.runID, "repairs"); err != nil || ok {
		t.Fatalf("after a second block: CheckExtension = %v, %v; want false, nil", ok, err)
	}
	// A read failure is an error, never a grant.
	if err := w.j.Close(); err != nil {
		t.Fatal(err)
	}
	if ok, err := CheckExtension(t.Context(), w.j, w.runID, "repairs"); err == nil || ok {
		t.Fatalf("closed journal: CheckExtension = %v, %v; want an error and false", ok, err)
	}
}

func TestExtensionRejectsUnknownKind(t *testing.T) {
	t.Parallel()
	w := blockedRun(t, "envelope_repairs_exhausted")
	for name, c := range map[string]struct {
		kind      string
		raisedTo  int64
		decidedBy string
		want      string
	}{
		"unknown kind":           {"budget", 9, "operator", "unknown extension kind"},
		"empty kind":             {"", 9, "operator", "unknown extension kind"},
		"other kind not blocked": {"replans", 9, "operator", "not blocked for replans"},
		"not above the ceiling":  {"repairs", 3, "operator", "must exceed"},
		"lower than the ceiling": {"repairs", 1, "operator", "must exceed"},
		"no decider":             {"repairs", 9, "", "decided_by"},
	} {
		err := RequestExtension(t.Context(), w.j, w.runID, c.kind, c.raisedTo, c.decidedBy)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, c.want)
		}
	}
	if got := len(w.extensions(t)); got != 0 {
		t.Fatalf("rejected requests journaled %d events", got)
	}
	if env, err := w.j.RunEnvelope(t.Context(), w.runID); err != nil || env.Repairs != 3 {
		t.Fatalf("envelope after rejections = %+v, %v; want repairs still 3", env, err)
	}

	// A run that is not blocked at all cannot be extended.
	j, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	prod := NewProducer(ids.New("sup"), 1)
	runID := ids.New("run")
	if err := CreateRun(t.Context(), j, journal.RunRow{RunID: runID, AdapterID: "builtin/fake", SourceRepo: "/tmp/repo", TaskSHA256: "t",
		BillingPosture: "local-scripted", ExecutionProfile: "trusted-host"}, prod); err != nil {
		t.Fatal(err)
	}
	if err := RequestExtension(t.Context(), j, runID, "repairs", 9, "operator"); err == nil || !strings.Contains(err.Error(), "not blocked") {
		t.Fatalf("unblocked run: err = %v, want a not blocked error", err)
	}
}

// gateWorld is an executing run with a stored envelope, ready to be gated.
type gateWorld struct {
	envelopeWorld
	attempts int64
	sent     map[string]int64
}

func newGateWorld(t *testing.T, env journal.EnvelopeRow, attempts int64) *gateWorld {
	t.Helper()
	j, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	w := &gateWorld{sent: map[string]int64{}}
	w.j, w.runID, w.prod = j, ids.New("run"), NewProducer(ids.New("sup"), 1)
	run := journal.RunRow{RunID: w.runID, AdapterID: "builtin/fake", SourceRepo: "/tmp/repo", TaskSHA256: "t",
		BillingPosture: "local-scripted", ExecutionProfile: "trusted-host"}
	if err := CreateRun(t.Context(), j, run, w.prod); err != nil {
		t.Fatal(err)
	}
	for _, to := range []RunState{RunAdmission, RunExecuting} {
		if err := TransitionRun(t.Context(), j, w.runID, to, "", w.prod); err != nil {
			t.Fatal(err)
		}
	}
	env.RunID, env.UpdatedAt = w.runID, time.Now().UTC().Format(time.RFC3339Nano)
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error { return journal.UpsertRunEnvelope(t.Context(), tx, env) }); err != nil {
		t.Fatal(err)
	}
	for range attempts {
		w.addAttempt(t)
	}
	return w
}

func (w *gateWorld) addAttempt(t *testing.T) string {
	t.Helper()
	w.attempts++
	id := ids.New("att")
	if err := RecordLaunchIntent(t.Context(), w.j, journal.AttemptRow{AttemptID: id, RunID: w.runID, TaskID: "task", AttemptNumber: w.attempts,
		LaunchTokenSHA256: strings.Repeat("a", 64), WorkspacePath: "/tmp/ws"}, w.prod); err != nil {
		t.Fatal(err)
	}
	return id
}

// launch plans and records the next attempt as the pipeline does: the check,
// then the counted intent.
func (w *gateWorld) launch(t *testing.T, c billing.Ceilings, now time.Time) error {
	t.Helper()
	plan, err := planLaunchAt(t.Context(), w.j, w.runID, c, now)
	if err != nil {
		return err
	}
	w.attempts++
	err = recordLaunchIntent(t.Context(), w.j, journal.AttemptRow{AttemptID: ids.New("att"), RunID: w.runID, TaskID: "task", AttemptNumber: w.attempts,
		LaunchTokenSHA256: strings.Repeat("a", 64), WorkspacePath: "/tmp/ws"}, w.prod, plan.commit(t.Context(), w.runID))
	if err != nil {
		w.attempts--
	}
	return err
}

func (w *gateWorld) addVerification(t *testing.T) {
	t.Helper()
	now := time.Now().UTC()
	if err := w.j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.InsertVerification(t.Context(), tx, journal.VerificationRow{ID: ids.New("ver"), RunID: w.runID,
			CandidateCommit: strings.Repeat("c", 40), ConfigSHA256: strings.Repeat("d", 64), Result: "passed", StartedAt: now, FinishedAt: now})
	}); err != nil {
		t.Fatal(err)
	}
}

func (w *gateWorld) row(t *testing.T) journal.EnvelopeRow {
	t.Helper()
	row, err := w.j.RunEnvelope(t.Context(), w.runID)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

var gateCeilings = billing.Ceilings{Execution: 10 * time.Minute, Repairs: 3, Replans: 2, TransportRetries: 5}

func gateEnvelope() journal.EnvelopeRow {
	return journal.EnvelopeRow{ExecutionSeconds: 600, Repairs: 3, Replans: 2, TransportRetries: 5}
}

func requireGateRefusal(t *testing.T, err error, reason string) {
	t.Helper()
	var gate *GateError
	if !errors.As(err, &gate) || gate.Reason != reason || !errors.Is(err, billing.ErrBudgetExhausted) {
		t.Fatalf("gate err = %v, want a %s refusal wrapping ErrBudgetExhausted", err, reason)
	}
}

// blockedByGate runs the pipeline's reaction to a refused launch and
// returns the run as projected afterwards.
func (w *gateWorld) blockedByGate(t *testing.T, gateErr error) journal.RunRow {
	t.Helper()
	p := &pipeline{d: admission.Decision{RunID: w.runID, RunDir: t.TempDir()}, j: w.j, prod: w.prod, h: Hooks{Event: func(journal.Event) {}, Notice: func(string) {}}, out: Outcome{RunID: w.runID, State: RunExecuting}}
	if err := p.blockLaunch(t.Context(), gateErr); err == nil {
		t.Fatal("blockLaunch returned nil for a refused launch")
	}
	run, err := w.j.Run(t.Context(), w.runID)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func (w *gateWorld) launchIntents(t *testing.T) int {
	t.Helper()
	evs, err := w.j.Events(t.Context(), w.runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, ev := range evs {
		if ev.Type == "attempt.launch_intent_recorded" {
			n++
		}
	}
	return n
}

func TestGateRefusesOverCeilingRepairs(t *testing.T) {
	t.Parallel()
	env := gateEnvelope()
	env.RepairsUsed = 3 // at the ceiling: the next repair is one over
	w := newGateWorld(t, env, 1)
	w.addVerification(t)
	intents := w.launchIntents(t)
	err := GateLaunch(t.Context(), w.j, w.runID, gateCeilings)
	requireGateRefusal(t, err, "envelope_repairs_exhausted")
	if got := w.row(t).RepairsUsed; got != 3 {
		t.Fatalf("a refused launch counted: repairs used = %d, want 3", got)
	}
	run := w.blockedByGate(t, err)
	if run.State != string(RunBlocked) || run.Reason != "envelope_repairs_exhausted" {
		t.Fatalf("run = %s (%s), want blocked (envelope_repairs_exhausted)", run.State, run.Reason)
	}
	if got := w.launchIntents(t); got != intents {
		t.Fatalf("launch intents %d -> %d: a refused launch journaled an intent", intents, got)
	}

	// One repair under the ceiling is allowed, and counted.
	env.RepairsUsed = 2
	ok := newGateWorld(t, env, 1)
	ok.addVerification(t)
	if err := ok.launch(t, gateCeilings, time.Now()); err != nil {
		t.Fatalf("repair under the ceiling refused: %v", err)
	}
	if got := ok.row(t).RepairsUsed; got != 3 {
		t.Fatalf("repairs used after an allowed repair = %d, want 3", got)
	}
}

func TestGateRefusesOverCeilingReplans(t *testing.T) {
	t.Parallel()
	env := gateEnvelope()
	env.ReplansUsed = 2
	w := newGateWorld(t, env, 1) // no verification: the next attempt is a replan
	intents := w.launchIntents(t)
	err := GateLaunch(t.Context(), w.j, w.runID, gateCeilings)
	requireGateRefusal(t, err, "envelope_replans_exhausted")
	run := w.blockedByGate(t, err)
	if run.State != string(RunBlocked) || run.Reason != "envelope_replans_exhausted" {
		t.Fatalf("run = %s (%s), want blocked (envelope_replans_exhausted)", run.State, run.Reason)
	}
	if got := w.launchIntents(t); got != intents {
		t.Fatalf("launch intents %d -> %d: a refused replan journaled an intent", intents, got)
	}
}

func TestGateHonoursGrantedExtension(t *testing.T) {
	t.Parallel()
	env := gateEnvelope()
	env.RepairsUsed = 3
	w := newGateWorld(t, env, 1)
	w.addVerification(t)
	requireGateRefusal(t, GateLaunch(t.Context(), w.j, w.runID, gateCeilings), "envelope_repairs_exhausted")

	// The refusal blocks the run; the operator's recorded decision answers it.
	w.blockedByGate(t, GateLaunch(t.Context(), w.j, w.runID, gateCeilings))
	if err := RequestExtension(t.Context(), w.j, w.runID, "repairs", 6, "operator"); err != nil {
		t.Fatal(err)
	}
	if err := GateLaunch(t.Context(), w.j, w.runID, gateCeilings); err != nil {
		t.Fatalf("launch with a granted repairs extension refused: %v", err)
	}
	// The grant covers repairs only.
	other := gateEnvelope()
	other.ReplansUsed = 2
	o := newGateWorld(t, other, 1)
	o.blockedByGate(t, GateLaunch(t.Context(), o.j, o.runID, gateCeilings))
	if err := RequestExtension(t.Context(), o.j, o.runID, "replans", 4, "operator"); err != nil {
		t.Fatal(err)
	}
	o.addVerification(t) // the next attempt is now a repair, with 0 used
	if err := GateLaunch(t.Context(), o.j, o.runID, gateCeilings); err != nil {
		t.Fatalf("unrelated repair refused: %v", err)
	}
}

func TestRepairReplanClassification(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name         string
		attempts     int64
		verification bool
		repairs      int64
		replans      int64
	}{
		{"first launch counts neither", 0, false, 0, 0},
		{"first launch with a stray verification counts neither", 0, true, 0, 0},
		{"attempt 2 after a check suite is a repair", 1, true, 1, 0},
		{"attempt 2 with no check suite is a replan", 1, false, 0, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := newGateWorld(t, gateEnvelope(), c.attempts)
			if c.verification {
				w.addVerification(t)
			}
			if err := w.launch(t, gateCeilings, time.Now()); err != nil {
				t.Fatal(err)
			}
			if row := w.row(t); row.RepairsUsed != c.repairs || row.ReplansUsed != c.replans {
				t.Fatalf("repairs/replans used = %d/%d, want %d/%d", row.RepairsUsed, row.ReplansUsed, c.repairs, c.replans)
			}
		})
	}
}

func TestIsReplanReadsVerifications(t *testing.T) {
	t.Parallel()
	w := newGateWorld(t, gateEnvelope(), 1)
	if got, err := IsReplan(t.Context(), w.j, w.runID); err != nil || !got {
		t.Fatalf("IsReplan without a verification = %v, %v; want true", got, err)
	}
	w.addVerification(t)
	if got, err := IsReplan(t.Context(), w.j, w.runID); err != nil || got {
		t.Fatalf("IsReplan with a verification = %v, %v; want false", got, err)
	}
	if err := w.j.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := IsReplan(t.Context(), w.j, w.runID); err == nil || got {
		t.Fatalf("IsReplan on a broken journal = %v, %v; want an error, never a default", got, err)
	}
}

func (w *gateWorld) progress(t *testing.T, attemptID string, retries int) {
	t.Helper()
	ev, err := newEvent(w.runID, "task", attemptID, "attempt.progress", map[string]any{"assistant_turns": 1, "tool_uses": map[string]int{}, "retries": retries}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	ev.ProducerID, ev.ProducerSequence, ev.Generation = "worker-"+attemptID, 1+w.progressCount(attemptID), 1
	if err := w.j.Append(t.Context(), ev, func(tx *sql.Tx) error { return projectWorkerEvent(t.Context(), tx, ev) }); err != nil {
		t.Fatal(err)
	}
}

func (w *gateWorld) progressCount(attemptID string) int64 {
	w.sent[attemptID]++
	return w.sent[attemptID] - 1
}

func TestTransportCeilingCountsProgressRetries(t *testing.T) {
	t.Parallel()
	w := newGateWorld(t, gateEnvelope(), 0)
	first := w.addAttempt(t)
	// The worker reports a cumulative count per attempt.
	w.progress(t, first, 1)
	w.progress(t, first, 3)
	w.progress(t, first, 3)
	if got := w.row(t).TransportRetriesSeen; got != 3 {
		t.Fatalf("transport retries seen = %d, want 3 (cumulative per attempt, never summed)", got)
	}
	second := w.addAttempt(t)
	w.progress(t, second, 2)
	if got := w.row(t).TransportRetriesSeen; got != 5 {
		t.Fatalf("transport retries seen over two attempts = %d, want 5", got)
	}
	w.addVerification(t)
	if err := GateLaunch(t.Context(), w.j, w.runID, gateCeilings); err != nil {
		t.Fatalf("5 of 5 transport retries refused: %v", err)
	}
	w.progress(t, second, 3)
	requireGateRefusal(t, GateLaunch(t.Context(), w.j, w.runID, gateCeilings), "envelope_transport_retries_exhausted")
}

func TestGateRefusesPastDeadline(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	env := gateEnvelope()
	env.FirstStartAt = start.Format(time.RFC3339Nano)
	w := newGateWorld(t, env, 1)
	w.addVerification(t)
	for _, c := range []struct {
		name string
		now  time.Time
		want string
	}{
		{"one second before the deadline", start.Add(10*time.Minute - time.Second), ""},
		{"at the deadline", start.Add(10 * time.Minute), "envelope_deadline_exceeded"},
		{"past the deadline", start.Add(time.Hour), "envelope_deadline_exceeded"},
	} {
		err := gateLaunchAt(t.Context(), w.j, w.runID, gateCeilings, c.now)
		if c.want == "" {
			if err != nil {
				t.Errorf("%s: refused: %v", c.name, err)
			}
			continue
		}
		var gate *GateError
		if !errors.As(err, &gate) || gate.Reason != c.want {
			t.Errorf("%s: err = %v, want %s", c.name, err, c.want)
		}
	}
	// A quiesced pause stops the clock; a still-running turn does not.
	env.PauseSpans = fmt.Sprintf(`[{"requested_at":%q,"quiesced_at":%q,"resumed_at":%q}]`,
		start.Add(time.Minute).Format(time.RFC3339Nano), start.Add(2*time.Minute).Format(time.RFC3339Nano), start.Add(7*time.Minute).Format(time.RFC3339Nano))
	p := newGateWorld(t, env, 1)
	p.addVerification(t)
	if err := gateLaunchAt(t.Context(), p.j, p.runID, gateCeilings, start.Add(14*time.Minute)); err != nil {
		t.Fatalf("a 5m quiesced pause did not extend the deadline: %v", err)
	}
	if err := gateLaunchAt(t.Context(), p.j, p.runID, gateCeilings, start.Add(15*time.Minute)); err == nil {
		t.Fatal("deadline 15m (10m + 5m pause) not enforced")
	}
}

func TestFirstLaunchStartsClockOnce(t *testing.T) {
	t.Parallel()
	w := newGateWorld(t, gateEnvelope(), 0)
	t0 := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	if err := w.launch(t, gateCeilings, t0); err != nil {
		t.Fatal(err)
	}
	if got := w.row(t).FirstStartAt; got != t0.Format(time.RFC3339Nano) {
		t.Fatalf("first start = %q, want %q", got, t0.Format(time.RFC3339Nano))
	}
	plan, err := planLaunchAt(t.Context(), w.j, w.runID, gateCeilings, t0.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if want := t0.Add(gateCeilings.Execution); !plan.deadline.Equal(want) {
		t.Fatalf("deadline of a later launch = %v, want %v (from the first start)", plan.deadline, want)
	}
	if err := w.launch(t, gateCeilings, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := w.row(t).FirstStartAt; got != t0.Format(time.RFC3339Nano) {
		t.Fatalf("a later launch moved the first start to %q", got)
	}
}

func TestPlannedLaunchesCannotBothPass(t *testing.T) {
	t.Parallel()
	env := gateEnvelope()
	env.RepairsUsed = 2 // one repair left
	w := newGateWorld(t, env, 1)
	w.addVerification(t)
	// Two launches planned from the same state both pass the check.
	a, errA := planLaunchAt(t.Context(), w.j, w.runID, gateCeilings, time.Now())
	b, errB := planLaunchAt(t.Context(), w.j, w.runID, gateCeilings, time.Now())
	if errA != nil || errB != nil {
		t.Fatalf("plans refused: %v, %v", errA, errB)
	}
	intent := func(n int64, plan launchPlan) error {
		return recordLaunchIntent(t.Context(), w.j, journal.AttemptRow{AttemptID: ids.New("att"), RunID: w.runID, TaskID: "task", AttemptNumber: n,
			LaunchTokenSHA256: strings.Repeat("a", 64), WorkspacePath: "/tmp/ws"}, w.prod, plan.commit(t.Context(), w.runID))
	}
	if err := intent(2, a); err != nil {
		t.Fatalf("first launch: %v", err)
	}
	requireGateRefusal(t, intent(3, b), "envelope_repairs_exhausted")
	if got := w.row(t).RepairsUsed; got != 3 {
		t.Fatalf("repairs used = %d, want 3: the second launch must not count", got)
	}
	if got := w.launchIntents(t); got != 2 {
		t.Fatalf("launch intents = %d, want 2 (the first attempt and one repair)", got)
	}
}

func TestConcurrentLaunchesStayWithinCeiling(t *testing.T) {
	t.Parallel()
	env := gateEnvelope()
	env.RepairsUsed = 1 // two repairs left
	w := newGateWorld(t, env, 1)
	w.addVerification(t)
	const launchers = 6
	var wg sync.WaitGroup
	var mu sync.Mutex
	passed := 0
	for n := range launchers {
		wg.Go(func() {
			plan, err := planLaunchAt(t.Context(), w.j, w.runID, gateCeilings, time.Now())
			if err != nil {
				return
			}
			err = recordLaunchIntent(t.Context(), w.j, journal.AttemptRow{AttemptID: ids.New("att"), RunID: w.runID, TaskID: "task", AttemptNumber: int64(2 + n),
				LaunchTokenSHA256: strings.Repeat("a", 64), WorkspacePath: "/tmp/ws"}, w.prod, plan.commit(t.Context(), w.runID))
			if err == nil {
				mu.Lock()
				passed++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if passed != 2 {
		t.Fatalf("%d concurrent launches passed with 2 repairs left, want exactly 2", passed)
	}
	if got := w.row(t).RepairsUsed; got != 3 {
		t.Fatalf("repairs used = %d, want 3", got)
	}
}

func TestFailedLaunchDoesNotConsumeCounter(t *testing.T) {
	t.Parallel()
	env := gateEnvelope()
	env.RepairsUsed = 1
	w := newGateWorld(t, env, 1)
	w.addVerification(t)
	db := rawOpen(t, filepath.Join(w.j.StateDir(), journal.DBName))
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER no_attempts BEFORE INSERT ON attempts BEGIN SELECT RAISE(ABORT, 'injected intent failure'); END`); err != nil {
		t.Fatal(err)
	}
	before := w.row(t)
	if err := w.launch(t, gateCeilings, time.Now()); err == nil || !strings.Contains(err.Error(), "injected intent failure") {
		t.Fatalf("launch err = %v, want the injected intent failure", err)
	}
	after := w.row(t)
	if after.RepairsUsed != before.RepairsUsed || after.ReplansUsed != before.ReplansUsed || after.FirstStartAt != before.FirstStartAt {
		t.Fatalf("a launch whose intent failed changed the envelope: %+v -> %+v", before, after)
	}
}

// rawProgress appends an attempt.progress event with the given payload
// text through the ingest projection (validate=true) or straight into the
// journal (validate=false), as a journal from before the check would hold.
func (w *gateWorld) rawProgress(t *testing.T, attemptID, payload string, validate bool) error {
	t.Helper()
	ev := journal.Event{SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"), RunID: w.runID, TaskID: "task", AttemptID: attemptID,
		ProducerID: "worker-" + attemptID, ProducerSequence: 1 + w.sent[attemptID], Generation: 1, ObservedAt: time.Now().UTC(),
		Type: "attempt.progress", Payload: json.RawMessage(payload)}
	var project func(*sql.Tx) error
	if validate {
		project = func(tx *sql.Tx) error { return projectWorkerEvent(t.Context(), tx, ev) }
	}
	err := w.j.Append(t.Context(), ev, project)
	if err == nil {
		w.sent[attemptID]++ // a rejected event does not advance the producer
	}
	return err
}

func TestMalformedProgressRetriesFailClosed(t *testing.T) {
	t.Parallel()
	w := newGateWorld(t, gateEnvelope(), 0)
	att := w.addAttempt(t)
	if err := w.rawProgress(t, att, `{"retries":3}`, true); err != nil {
		t.Fatal(err)
	}
	for name, payload := range map[string]string{
		"negative":    `{"retries":-4}`,
		"non-numeric": `{"retries":"many"}`,
		"fractional":  `{"retries":1.5}`,
		"null":        `{"retries":null}`,
		"missing":     `{"assistant_turns":1}`,
	} {
		err := w.rawProgress(t, att, payload, true)
		if !errors.Is(err, ErrCorruptSpool) {
			t.Errorf("%s: err = %v, want ErrCorruptSpool", name, err)
		}
		if got := w.row(t).TransportRetriesSeen; got != 3 {
			t.Errorf("%s: transport retries seen = %d, want 3 (malformed output cannot change it)", name, got)
		}
	}
	// The ceiling still bites: 3 seen, 3 more is over the ceiling of 5.
	if err := w.rawProgress(t, att, `{"retries":6}`, true); err != nil {
		t.Fatal(err)
	}
	requireGateRefusal(t, GateLaunch(t.Context(), w.j, w.runID, gateCeilings), "envelope_transport_retries_exhausted")
}

func TestJournaledMalformedRetriesNeverReduceCount(t *testing.T) {
	t.Parallel()
	w := newGateWorld(t, gateEnvelope(), 0)
	good, negative, text := w.addAttempt(t), w.addAttempt(t), w.addAttempt(t)
	// Events journaled before the validation existed: a negative and a
	// non-numeric report, each alone for its attempt, with no projection run.
	if err := w.rawProgress(t, negative, `{"retries":-9}`, false); err != nil {
		t.Fatal(err)
	}
	if err := w.rawProgress(t, text, `{"retries":"7"}`, false); err != nil {
		t.Fatal(err)
	}
	if err := w.rawProgress(t, good, `{"retries":4}`, true); err != nil {
		t.Fatal(err)
	}
	if got := w.row(t).TransportRetriesSeen; got != 4 {
		t.Fatalf("transport retries seen = %d, want 4: malformed journaled reports must count for nothing", got)
	}
}

func TestLaunchRefusedWhenDeadlineExpiresBeforeCommit(t *testing.T) {
	t.Parallel()
	// Planned long ago with a 1ms ceiling: the plan passes at its own time,
	// and by the time the intent commits the deadline is long past.
	planned := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	short := billing.Ceilings{Execution: time.Millisecond, Repairs: 3, Replans: 2, TransportRetries: 5}
	for _, c := range []struct {
		name     string
		attempts int64
		verify   bool
	}{
		{"first launch", 0, false},
		{"repair", 1, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := newGateWorld(t, gateEnvelope(), c.attempts)
			if c.verify {
				w.addVerification(t)
			}
			before := w.row(t)
			plan, err := planLaunchAt(t.Context(), w.j, w.runID, short, planned)
			if err != nil {
				t.Fatalf("plan at its own time refused: %v", err)
			}
			intents := w.launchIntents(t)
			err = recordLaunchIntent(t.Context(), w.j, journal.AttemptRow{AttemptID: ids.New("att"), RunID: w.runID, TaskID: "task", AttemptNumber: c.attempts + 1,
				LaunchTokenSHA256: strings.Repeat("a", 64), WorkspacePath: "/tmp/ws"}, w.prod, plan.commit(t.Context(), w.runID))
			requireGateRefusal(t, err, "envelope_deadline_exceeded")
			if got := w.launchIntents(t); got != intents {
				t.Fatalf("launch intents %d -> %d: an expired launch was admitted", intents, got)
			}
			after := w.row(t)
			if after.RepairsUsed != before.RepairsUsed || after.ReplansUsed != before.ReplansUsed || after.FirstStartAt != before.FirstStartAt {
				t.Fatalf("a refused launch changed the envelope: %+v -> %+v", before, after)
			}
		})
	}
}

// reserveWorld is a run whose execution clock started at reserveStart under a
// 10-minute ceiling, gated by a pipeline that carries the given admitted
// checks. The start floats with the test clock: the tests commit with the
// real clock, so a fixed date would expire and fail every run past it.
var reserveStart = time.Now().UTC().Truncate(time.Second)

func newReserveWorld(t *testing.T, env journal.EnvelopeRow, attempts int64, verified bool, checks ...admission.CheckConfig) (*gateWorld, *pipeline) {
	t.Helper()
	env.FirstStartAt = reserveStart.Format(time.RFC3339Nano)
	w := newGateWorld(t, env, attempts)
	if verified {
		w.addVerification(t)
	}
	p := &pipeline{d: admission.Decision{RunID: w.runID, ProjectConfig: admission.ProjectConfig{Checks: checks}},
		j: w.j, prod: w.prod, ceilings: gateCeilings}
	return w, p
}

func reserveChecks(timeouts ...string) []admission.CheckConfig {
	checks := make([]admission.CheckConfig, len(timeouts))
	for i, d := range timeouts {
		checks[i] = admission.CheckConfig{Name: fmt.Sprintf("check-%d", i), Argv: []string{"true"}, Timeout: d}
	}
	return checks
}

// commitPlan records the planned launch's intent, as the pipeline does.
func (w *gateWorld) commitPlan(t *testing.T, plan launchPlan) {
	t.Helper()
	w.attempts++
	if err := recordLaunchIntent(t.Context(), w.j, journal.AttemptRow{AttemptID: ids.New("att"), RunID: w.runID, TaskID: "task", AttemptNumber: w.attempts,
		LaunchTokenSHA256: strings.Repeat("a", 64), WorkspacePath: "/tmp/ws"}, w.prod, plan.commit(t.Context(), w.runID)); err != nil {
		t.Fatal(err)
	}
}

// I21 (v2 §10.3): completion is budgeted before optional work.
func TestShortfallBlocksOptionalWork(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		replansUsed int64
		now         time.Time
		wantReason  string
	}{
		{"remainder one second short blocks the replan", 0, reserveStart.Add(10*time.Minute - 2*time.Minute + time.Second), "completion_reserve_shortfall"},
		{"remainder exactly covering dispatches", 0, reserveStart.Add(10*time.Minute - 2*time.Minute), ""},
		{"reserve is checked before the replan count", 2, reserveStart.Add(9 * time.Minute), "completion_reserve_shortfall"},
		{"a covered replan over the count reports the count", 2, reserveStart.Add(time.Minute), "envelope_replans_exhausted"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := gateEnvelope()
			env.ReplansUsed = tc.replansUsed
			w, p := newReserveWorld(t, env, 1, false, reserveChecks("1m", "1m")...) // no verification: a replan
			intents := w.launchIntents(t)
			_, _, err := p.planAttempt(t.Context(), tc.now)
			if tc.wantReason == "" {
				if err != nil {
					t.Fatalf("planAttempt refused a covered replan: %v", err)
				}
				return
			}
			requireGateRefusal(t, err, tc.wantReason)
			run := w.blockedByGate(t, err)
			if run.State != string(RunBlocked) || run.Reason != tc.wantReason {
				t.Fatalf("run = %s (%s), want blocked (%s)", run.State, run.Reason, tc.wantReason)
			}
			if got := w.launchIntents(t); got != intents {
				t.Fatalf("launch intents %d -> %d: a blocked replan journaled an intent", intents, got)
			}
		})
	}
}

func TestCheckBoundConversionBlocksOnFailure(t *testing.T) {
	t.Parallel()
	now := reserveStart.Add(time.Minute)
	for _, timeout := range []string{"soon", "", "0s", "-1m"} {
		t.Run("timeout "+timeout, func(t *testing.T) {
			t.Parallel()
			w, p := newReserveWorld(t, gateEnvelope(), 1, false, reserveChecks("1m", timeout)...)
			intents := w.launchIntents(t)
			_, _, err := p.planAttempt(t.Context(), now)
			if err == nil {
				t.Fatal("an unconvertible check timeout did not block the launch")
			}
			if _, ok := errors.AsType[*GateError](err); ok {
				t.Fatalf("err = %v: a conversion failure is not an envelope refusal", err)
			}
			run := w.blockedByGate(t, err)
			if run.State != string(RunBlocked) || run.Reason != "envelope_unavailable" {
				t.Fatalf("run = %s (%s), want blocked (envelope_unavailable)", run.State, run.Reason)
			}
			if got := w.launchIntents(t); got != intents {
				t.Fatalf("launch intents %d -> %d", intents, got)
			}
		})
	}
	// Control: the same list with parseable timeouts is estimated and passes.
	_, p := newReserveWorld(t, gateEnvelope(), 1, false, reserveChecks("1m", "2m")...)
	if _, _, err := p.planAttempt(t.Context(), now); err != nil {
		t.Fatalf("parseable checks refused: %v", err)
	}
}

func TestReplanDeadlineLeavesReserve(t *testing.T) {
	t.Parallel()
	_, p := newReserveWorld(t, gateEnvelope(), 1, false, reserveChecks("1m", "2m")...)
	plan, deadline, err := p.planAttempt(t.Context(), reserveStart.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	full := reserveStart.Add(10 * time.Minute)
	if !plan.deadline.Equal(full) {
		t.Fatalf("plan deadline = %v, want the full execution deadline %v", plan.deadline, full)
	}
	if want := full.Add(-3 * time.Minute); !deadline.Equal(want) {
		t.Fatalf("replan deadline = %v, want execution deadline minus the 3m verify sum = %v", deadline, want)
	}
}

func TestCommitRefusesPastReplanDeadline(t *testing.T) {
	t.Parallel()
	w, p := newReserveWorld(t, gateEnvelope(), 1, false, reserveChecks("1m", "2m")...)
	plan, deadline, err := p.planAttempt(t.Context(), reserveStart.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !plan.replanDeadline.Equal(deadline) {
		t.Fatalf("plan replan deadline = %v, want the returned replan deadline %v", plan.replanDeadline, deadline)
	}
	intents := w.launchIntents(t)
	before := w.row(t)
	plan.replanDeadline = time.Now().Add(-time.Minute)
	err = recordLaunchIntent(t.Context(), w.j, journal.AttemptRow{AttemptID: ids.New("att"), RunID: w.runID, TaskID: "task", AttemptNumber: 2,
		LaunchTokenSHA256: strings.Repeat("a", 64), WorkspacePath: "/tmp/ws"}, w.prod, plan.commit(t.Context(), w.runID))
	requireGateRefusal(t, err, "envelope_deadline_exceeded")
	if got := w.launchIntents(t); got != intents {
		t.Fatalf("launch intents %d -> %d: a refused replan journaled an intent", intents, got)
	}
	if after := w.row(t); after != before {
		t.Fatalf("a refused launch changed the envelope: %+v -> %+v", before, after)
	}
}

func TestReplanPreservesRepairSlots(t *testing.T) {
	t.Parallel()
	env := gateEnvelope()
	env.RepairsUsed = 1
	w, p := newReserveWorld(t, env, 1, false, reserveChecks("1m")...)
	plan, deadline, err := p.planAttempt(t.Context(), reserveStart.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	w.commitPlan(t, plan)
	row := w.row(t)
	if row.RepairsUsed != 1 || row.ReplansUsed != 1 {
		t.Fatalf("after a dispatched replan repairs_used = %d, replans_used = %d, want 1 and 1", row.RepairsUsed, row.ReplansUsed)
	}
	if !deadline.Before(plan.deadline) {
		t.Fatalf("replan deadline %v is not derived (execution deadline %v)", deadline, plan.deadline)
	}
}

// Repairs are completion work: drawn from the reserve, never gated on it.
func TestRepairsNotReserveGated(t *testing.T) {
	t.Parallel()
	env := gateEnvelope()
	env.RepairsUsed = 1 // an earlier repair consumed part of the ceiling of 3
	for _, c := range []struct {
		name string
		now  time.Time
	}{
		{"time covers the verify pass", reserveStart.Add(time.Minute)},
		{"time short of the verify pass", reserveStart.Add(9*time.Minute + 30*time.Second)},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w, p := newReserveWorld(t, env, 2, true, reserveChecks("1m", "1m")...)
			plan, deadline, err := p.planAttempt(t.Context(), c.now)
			if err != nil {
				t.Fatalf("repair blocked: %v", err)
			}
			if !deadline.Equal(plan.deadline) {
				t.Fatalf("repair deadline = %v, want the shared execution deadline %v", deadline, plan.deadline)
			}
			w.commitPlan(t, plan)
			if got := w.row(t).RepairsUsed; got != 2 {
				t.Fatalf("repairs used = %d, want 2", got)
			}
		})
	}
}

func TestFirstAttemptUnaffected(t *testing.T) {
	t.Parallel()
	// The first attempt has nothing optional yet: even checks that could never
	// fit, and a timeout the estimator could not convert, do not gate it.
	for _, timeouts := range [][]string{{"1h", "1h"}, {"soon"}} {
		env := gateEnvelope()
		w, p := newReserveWorld(t, env, 0, false, reserveChecks(timeouts...)...)
		plan, deadline, err := p.planAttempt(t.Context(), reserveStart)
		if err != nil {
			t.Fatalf("first attempt with checks %v blocked: %v", timeouts, err)
		}
		if !deadline.Equal(plan.deadline) {
			t.Fatalf("first attempt deadline = %v, want the full %v", deadline, plan.deadline)
		}
		_ = w
	}
}

// Under --no-checks no verification pass runs, so there is nothing to reserve.
func TestNoChecksReservesNothing(t *testing.T) {
	t.Parallel()
	_, p := newReserveWorld(t, gateEnvelope(), 1, false, reserveChecks("1h")...)
	now := reserveStart.Add(9 * time.Minute)
	if _, _, err := p.planAttempt(t.Context(), now); err == nil {
		t.Fatal("control: a 1h check on 1m left did not block the replan")
	}
	p.d.NoChecks = true
	plan, deadline, err := p.planAttempt(t.Context(), now)
	if err != nil {
		t.Fatalf("replan under --no-checks blocked: %v", err)
	}
	if !deadline.Equal(plan.deadline) {
		t.Fatalf("deadline = %v, want the full %v (empty reserve)", deadline, plan.deadline)
	}
}
