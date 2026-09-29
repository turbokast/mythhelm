package supervisor

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
)

func openTemp(t *testing.T) (*journal.Journal, string) {
	t.Helper()
	dir := t.TempDir()
	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return j, filepath.Join(dir, journal.DBName)
}

// rawOpen is a second connection for fixtures and fault injection that
// bypass the supervisor.
func rawOpen(t *testing.T, path string) *sql.DB {
	t.Helper()
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	u := url.URL{Scheme: "file", Path: p, RawQuery: "_pragma=busy_timeout(5000)"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func newRun(t *testing.T, j *journal.Journal, p *Producer) string {
	t.Helper()
	runID := ids.New("run")
	err := CreateRun(t.Context(), j, journal.RunRow{
		RunID:            runID,
		AdapterID:        "fake",
		SourceRepo:       "/tmp/repo",
		TaskSHA256:       strings.Repeat("a", 64),
		BillingPosture:   "local-scripted",
		ExecutionProfile: "trusted-host",
	}, p)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	return runID
}

// attemptNumbers keeps (run, task, attempt number) unique across fixtures.
var attemptNumbers atomic.Int64

func newAttempt(t *testing.T, j *journal.Journal, p *Producer, runID string) string {
	t.Helper()
	attemptID := ids.New("att")
	err := RecordLaunchIntent(t.Context(), j, journal.AttemptRow{
		AttemptID:         attemptID,
		RunID:             runID,
		TaskID:            "task_1",
		AttemptNumber:     attemptNumbers.Add(1),
		LaunchTokenSHA256: strings.Repeat("b", 64),
		WorkspacePath:     "/tmp/workspace",
	}, p)
	if err != nil {
		t.Fatalf("RecordLaunchIntent: %v", err)
	}
	return attemptID
}

func journalCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM journal`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func projected(t *testing.T, db *sql.DB, table, id string) (state, reason string) {
	t.Helper()
	q := `SELECT state, reason FROM runs WHERE run_id = ?`
	if table == "attempts" {
		q = `SELECT state, reason FROM attempts WHERE attempt_id = ?`
	}
	var r sql.NullString
	if err := db.QueryRowContext(t.Context(), q, id).Scan(&state, &r); err != nil {
		t.Fatal(err)
	}
	return state, r.String
}

// legalRun is design §4's run transition table, restated independently of
// the implementation.
var legalRun = map[RunState][]RunState{
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

var allRunStates = []RunState{
	RunCreated, RunAdmission, RunExecuting, RunVerifying, RunReadyForReview, RunApplying, RunCompleted,
	RunStopping, RunCancelled, RunInterrupted, RunRecovering, RunBlocked, RunFailed,
}

// reasonFor returns a reason the target state accepts.
func reasonFor[S ~string](to S) string {
	switch string(to) {
	case "blocked", "failed", "interrupted", "failed_native":
		return "test_reason"
	}
	return ""
}

func TestIllegalRunTransitionRejected(t *testing.T) {
	j, path := openTemp(t)
	db := rawOpen(t, path)
	p := NewProducer(ids.New("sup"), 1)

	for _, from := range allRunStates {
		for _, to := range allRunStates {
			legal := slices.Contains(legalRun[from], to)
			t.Run(fmt.Sprintf("%s to %s", from, to), func(t *testing.T) {
				runID := newRun(t, j, p)
				mustExec(t, db, `UPDATE runs SET state = ? WHERE run_id = ?`, string(from), runID)
				before := journalCount(t, db)

				err := TransitionRun(t.Context(), j, runID, to, reasonFor(to), p)

				state, _ := projected(t, db, "runs", runID)
				after := journalCount(t, db)
				if legal {
					if err != nil {
						t.Fatalf("legal transition rejected: %v", err)
					}
					if state != string(to) || after != before+1 {
						t.Fatalf("state = %s, journal rows %d -> %d; want %s and one new row", state, before, after, to)
					}
					return
				}
				if !errors.Is(err, ErrIllegalTransition) {
					t.Fatalf("err = %v, want ErrIllegalTransition", err)
				}
				if state != string(from) || after != before {
					t.Fatalf("state = %s, journal rows %d -> %d; want %s and no new row", state, before, after, from)
				}
			})
		}
	}
}

func TestRunTransitionReasons(t *testing.T) {
	j, path := openTemp(t)
	db := rawOpen(t, path)
	p := NewProducer(ids.New("sup"), 1)

	tests := []struct {
		from   RunState
		to     RunState
		reason string
		ok     bool
	}{
		{RunAdmission, RunBlocked, "", false},
		{RunAdmission, RunBlocked, "trust_required", true},
		{RunAdmission, RunFailed, "", false},
		{RunExecuting, RunInterrupted, "", false},
		{RunExecuting, RunInterrupted, "worker_lost", true},
		{RunVerifying, RunReadyForReview, "", true},
		{RunVerifying, RunReadyForReview, "unverified", true},
		{RunVerifying, RunReadyForReview, "looks_fine", false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s to %s reason %q", tt.from, tt.to, tt.reason), func(t *testing.T) {
			runID := newRun(t, j, p)
			mustExec(t, db, `UPDATE runs SET state = ? WHERE run_id = ?`, string(tt.from), runID)
			before := journalCount(t, db)

			err := TransitionRun(t.Context(), j, runID, tt.to, tt.reason, p)

			if tt.ok != (err == nil) {
				t.Fatalf("err = %v, want ok = %v", err, tt.ok)
			}
			if !tt.ok && !errors.Is(err, ErrIllegalTransition) {
				t.Fatalf("err = %v, want ErrIllegalTransition", err)
			}
			state, reason := projected(t, db, "runs", runID)
			if tt.ok && (state != string(tt.to) || reason != tt.reason) {
				t.Fatalf("projection = (%s, %q), want (%s, %q)", state, reason, tt.to, tt.reason)
			}
			if !tt.ok && (state != string(tt.from) || journalCount(t, db) != before) {
				t.Fatalf("rejected transition changed state to %s or wrote a journal row", state)
			}
		})
	}
}

func TestTransitionUnknownRun(t *testing.T) {
	j, _ := openTemp(t)
	err := TransitionRun(t.Context(), j, "run_missing", RunAdmission, "", NewProducer(ids.New("sup"), 1))
	if !errors.Is(err, journal.ErrNotFound) {
		t.Fatalf("err = %v, want journal.ErrNotFound", err)
	}
}

func TestTransitionJournalsStateChange(t *testing.T) {
	j, path := openTemp(t)
	db := rawOpen(t, path)
	producerID := ids.New("sup")
	p := NewProducer(producerID, 1)
	runID := newRun(t, j, p)

	steps := []struct {
		to     RunState
		reason string
	}{
		{RunAdmission, ""}, {RunExecuting, ""}, {RunVerifying, ""}, {RunReadyForReview, "unverified"},
	}
	for _, s := range steps {
		if err := TransitionRun(t.Context(), j, runID, s.to, s.reason, p); err != nil {
			t.Fatalf("TransitionRun(%s): %v", s.to, err)
		}
	}

	evs, err := j.Events(t.Context(), runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != len(steps)+1 {
		t.Fatalf("journal has %d events, want %d", len(evs), len(steps)+1)
	}
	wantTypes := []string{"run.created", "run.state_changed", "run.state_changed", "run.state_changed", "run.state_changed"}
	wantStates := []string{"created", "admission", "executing", "verifying", "ready_for_review"}
	for i, ev := range evs {
		var got statePayload
		if err := json.Unmarshal(ev.Payload, &got); err != nil {
			t.Fatal(err)
		}
		if ev.Type != wantTypes[i] || got.State != wantStates[i] {
			t.Errorf("event %d = %s %s, want %s %s", i, ev.Type, got.State, wantTypes[i], wantStates[i])
		}
		if ev.ProducerID != producerID || ev.ProducerSequence != int64(i+1) || ev.RunSequence != int64(i+1) {
			t.Errorf("event %d producer %s seq %d run_seq %d, want %s %d %d",
				i, ev.ProducerID, ev.ProducerSequence, ev.RunSequence, producerID, i+1, i+1)
		}
	}
	last := evs[len(evs)-1]
	var payload statePayload
	_ = json.Unmarshal(last.Payload, &payload)
	if payload.Reason == nil || *payload.Reason != "unverified" {
		t.Errorf("last payload reason = %v, want \"unverified\"", payload.Reason)
	}
	if !strings.Contains(string(evs[0].Payload), `"reason":null`) {
		t.Errorf("run.created payload %s: an empty reason must be an explicit null", evs[0].Payload)
	}
	state, reason := projected(t, db, "runs", runID)
	if state != "ready_for_review" || reason != "unverified" {
		t.Errorf("projection = (%s, %q), want (ready_for_review, unverified)", state, reason)
	}
}

func TestTransitionAtomicWithJournal(t *testing.T) {
	j, path := openTemp(t)
	db := rawOpen(t, path)
	p := NewProducer(ids.New("sup"), 1)
	runID := newRun(t, j, p)
	attemptID := newAttempt(t, j, p, runID)

	mustExec(t, db, `CREATE TRIGGER inject_run BEFORE UPDATE ON runs BEGIN SELECT RAISE(ABORT, 'injected projection error'); END`)
	mustExec(t, db, `CREATE TRIGGER inject_attempt BEFORE UPDATE ON attempts BEGIN SELECT RAISE(ABORT, 'injected projection error'); END`)
	before := journalCount(t, db)

	if err := TransitionRun(t.Context(), j, runID, RunAdmission, "", p); err == nil || !strings.Contains(err.Error(), "injected projection error") {
		t.Fatalf("TransitionRun err = %v, want the injected projection error", err)
	}
	if err := TransitionAttempt(t.Context(), j, attemptID, AttemptLaunching, "", p); err == nil || !strings.Contains(err.Error(), "injected projection error") {
		t.Fatalf("TransitionAttempt err = %v, want the injected projection error", err)
	}
	if after := journalCount(t, db); after != before {
		t.Fatalf("journal rows %d -> %d after failed projections, want no new row", before, after)
	}
	if state, _ := projected(t, db, "runs", runID); state != "created" {
		t.Fatalf("run state = %s, want created", state)
	}

	// The failed transitions consumed no producer sequence: once the fault is
	// gone the same producer appends without a gap.
	mustExec(t, db, `DROP TRIGGER inject_run`)
	mustExec(t, db, `DROP TRIGGER inject_attempt`)
	if err := TransitionRun(t.Context(), j, runID, RunAdmission, "", p); err != nil {
		t.Fatalf("TransitionRun after the fault: %v", err)
	}
	if err := TransitionAttempt(t.Context(), j, attemptID, AttemptLaunching, "", p); err != nil {
		t.Fatalf("TransitionAttempt after the fault: %v", err)
	}
	if after := journalCount(t, db); after != before+2 {
		t.Fatalf("journal rows %d -> %d, want two new rows", before, after)
	}
}

func TestConcurrentTransitionsSerialise(t *testing.T) {
	j, path := openTemp(t)
	db := rawOpen(t, path)
	runID := newRun(t, j, NewProducer(ids.New("sup"), 1))
	mustExec(t, db, `UPDATE runs SET state = 'executing' WHERE run_id = ?`, runID)
	before := journalCount(t, db)

	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			errs[i] = TransitionRun(t.Context(), j, runID, RunStopping, "", NewProducer(ids.New("sup"), 1))
		})
	}
	wg.Wait()

	var ok int
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, ErrIllegalTransition):
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("%d transitions executing -> stopping succeeded, want exactly 1", ok)
	}
	if after := journalCount(t, db); after != before+1 {
		t.Fatalf("journal rows %d -> %d, want exactly one new row", before, after)
	}
}

var legalAttempt = map[AttemptState][]AttemptState{
	AttemptLaunchIntentRecorded: {AttemptLaunching, AttemptInterrupted},
	AttemptLaunching:            {AttemptRunning, AttemptInterrupted},
	AttemptRunning:              {AttemptSucceededNative, AttemptFailedNative, AttemptStopRequested, AttemptInterrupted},
	AttemptStopRequested:        {AttemptStopped, AttemptInterrupted},
	AttemptInterrupted:          {AttemptQuarantined},
}

var allAttemptStates = []AttemptState{
	AttemptLaunchIntentRecorded, AttemptLaunching, AttemptRunning, AttemptSucceededNative, AttemptFailedNative,
	AttemptStopRequested, AttemptStopped, AttemptInterrupted, AttemptQuarantined,
}

func TestIllegalAttemptTransitionRejected(t *testing.T) {
	j, path := openTemp(t)
	db := rawOpen(t, path)
	p := NewProducer(ids.New("sup"), 1)
	runID := newRun(t, j, p)

	for _, from := range allAttemptStates {
		for _, to := range allAttemptStates {
			legal := slices.Contains(legalAttempt[from], to)
			t.Run(fmt.Sprintf("%s to %s", from, to), func(t *testing.T) {
				attemptID := newAttempt(t, j, p, runID)
				mustExec(t, db, `UPDATE attempts SET state = ? WHERE attempt_id = ?`, string(from), attemptID)
				before := journalCount(t, db)

				err := TransitionAttempt(t.Context(), j, attemptID, to, reasonFor(to), p)

				state, _ := projected(t, db, "attempts", attemptID)
				after := journalCount(t, db)
				if legal {
					if err != nil {
						t.Fatalf("legal transition rejected: %v", err)
					}
					if state != string(to) || after != before+1 {
						t.Fatalf("state = %s, journal rows %d -> %d; want %s and one new row", state, before, after, to)
					}
					return
				}
				if !errors.Is(err, ErrIllegalTransition) {
					t.Fatalf("err = %v, want ErrIllegalTransition", err)
				}
				if state != string(from) || after != before {
					t.Fatalf("state = %s, journal rows %d -> %d; want %s and no new row", state, before, after, from)
				}
			})
		}
	}
}

func TestAttemptTransitionJournalsEnvelope(t *testing.T) {
	j, path := openTemp(t)
	db := rawOpen(t, path)
	p := NewProducer(ids.New("sup"), 1)
	runID := newRun(t, j, p)
	attemptID := newAttempt(t, j, p, runID)

	for _, s := range []struct {
		to     AttemptState
		reason string
	}{{AttemptLaunching, ""}, {AttemptRunning, ""}, {AttemptFailedNative, "result_unobserved"}} {
		if err := TransitionAttempt(t.Context(), j, attemptID, s.to, s.reason, p); err != nil {
			t.Fatalf("TransitionAttempt(%s): %v", s.to, err)
		}
	}
	if err := TransitionAttempt(t.Context(), j, attemptID, AttemptSucceededNative, "", p); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("failed_native -> succeeded_native: err = %v, want ErrIllegalTransition", err)
	}

	evs, err := j.Events(t.Context(), runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, ev := range evs {
		types = append(types, ev.Type)
		if ev.Type == "run.created" {
			continue
		}
		if ev.AttemptID != attemptID || ev.TaskID != "task_1" {
			t.Errorf("%s event attempt %q task %q, want %q task_1", ev.Type, ev.AttemptID, ev.TaskID, attemptID)
		}
	}
	want := "run.created attempt.launch_intent_recorded attempt.state_changed attempt.state_changed attempt.state_changed"
	if got := strings.Join(types, " "); got != want {
		t.Fatalf("event types = %s, want %s", got, want)
	}
	if !strings.Contains(string(evs[1].Payload), `"launch_token_sha256":"`+strings.Repeat("b", 64)+`"`) {
		t.Errorf("launch intent payload = %s, want the launch token digest", evs[1].Payload)
	}
	state, reason := projected(t, db, "attempts", attemptID)
	if state != "failed_native" || reason != "result_unobserved" {
		t.Errorf("projection = (%s, %q), want (failed_native, result_unobserved)", state, reason)
	}
}
