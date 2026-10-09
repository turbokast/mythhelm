package supervisor

import (
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/qualify"
	"github.com/turbokast/mythhelm/internal/workers"
)

func qualifyRecord(harness, surface, entitlement string) qualify.Record {
	return qualify.Record{Key: qualify.Key{Harness: harness, Surface: surface, EntitlementClass: entitlement}}
}

func TestRetryScheduleResetAndUnknown(t *testing.T) {
	t.Parallel()
	reset := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name    string
		reset   *time.Time
		used    int
		wait    time.Duration
		giveUp  bool
		comment string
	}{
		{"authoritative reset, first retry", &reset, 0, 0, false, "waits for the reset itself"},
		{"authoritative reset, second retry", &reset, 1, 0, false, ""},
		{"authoritative reset, third retry", &reset, 2, 0, false, ""},
		{"authoritative reset, fourth retry is refused", &reset, 3, 0, true, ""},
		{"unknown reset backs off 1m", nil, 0, time.Minute, false, ""},
		{"unknown reset backs off 5m", nil, 1, 5 * time.Minute, false, ""},
		{"unknown reset backs off 15m", nil, 2, 15 * time.Minute, false, ""},
		{"unknown reset gives up after three", nil, 3, 0, true, "timing shown unknown"},
		{"unknown reset stays given up", nil, 4, 0, true, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			wait, giveUp := RetrySchedule(c.reset, c.used)
			if wait != c.wait || giveUp != c.giveUp {
				t.Fatalf("RetrySchedule(%v, %d) = %v, %t; want %v, %t", c.reset, c.used, wait, giveUp, c.wait, c.giveUp)
			}
		})
	}
}

func TestEvaluateBucketSchedule(t *testing.T) {
	t.Parallel()
	exhausted := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	reset := exhausted.Add(2 * time.Hour)
	resetText, exhaustedText := reset.Format(time.RFC3339Nano), exhausted.Format(time.RFC3339Nano)
	row := func(resetAt *string, used int64) journal.BucketRow {
		return journal.BucketRow{Bucket: "b", ExhaustedAt: exhaustedText, ResetAt: resetAt, RetriesUsed: used}
	}
	for _, c := range []struct {
		name   string
		row    journal.BucketRow
		now    time.Time
		admit  bool
		wait   time.Duration
		giveUp bool
	}{
		{"unknown reset, before 1m", row(nil, 0), exhausted.Add(59 * time.Second), false, time.Second, false},
		{"unknown reset, at 1m", row(nil, 0), exhausted.Add(time.Minute), true, 0, false},
		{"unknown reset, second retry early", row(nil, 1), exhausted.Add(4*time.Minute + 59*time.Second), false, time.Second, false},
		{"unknown reset, second retry due", row(nil, 1), exhausted.Add(5 * time.Minute), true, 0, false},
		{"unknown reset, third retry early", row(nil, 2), exhausted.Add(14 * time.Minute), false, time.Minute, false},
		{"unknown reset, third retry due", row(nil, 2), exhausted.Add(15 * time.Minute), true, 0, false},
		{"unknown reset, retries consumed", row(nil, 3), exhausted.Add(24 * time.Hour), false, 0, true},
		{"authoritative reset not reached", row(&resetText, 0), reset.Add(-time.Minute), false, time.Minute, false},
		{"authoritative reset reached", row(&resetText, 0), reset, true, 0, false},
		{"authoritative reset, retries consumed", row(&resetText, 3), reset.Add(time.Hour), false, 0, true},
		{"unreadable reset is never invented", row(new("tomorrow"), 0), reset, false, 0, false},
		{"unreadable exhaustion time is never invented", journal.BucketRow{Bucket: "b", ExhaustedAt: "soon"}, reset, false, 0, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			admit, wait, giveUp := EvaluateBucket(c.row, c.now)
			if admit != c.admit || wait != c.wait || giveUp != c.giveUp {
				t.Fatalf("EvaluateBucket = %t, %v, %t; want %t, %v, %t", admit, wait, giveUp, c.admit, c.wait, c.giveUp)
			}
		})
	}
}

// bucketWorld is a journal with a run and an exhausted-bucket row for the
// bucket the test run would hold.
type bucketWorld struct {
	j      *journal.Journal
	runID  string
	bucket string
}

func newBucketWorld(t *testing.T) *bucketWorld {
	t.Helper()
	j, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	w := &bucketWorld{j: j, runID: ids.New("run")}
	now := time.Now()
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.InsertRun(t.Context(), tx, journal.RunRow{RunID: w.runID, State: "admission", AdapterID: "builtin/fake", SourceRepo: "/tmp/repo",
			TaskSHA256: "t", BillingPosture: "local-scripted", ExecutionProfile: "trusted-host", CreatedAt: now, UpdatedAt: now})
	}); err != nil {
		t.Fatal(err)
	}
	return w
}

func (w *bucketWorld) exhaust(t *testing.T, exhaustedAgo time.Duration, resetIn *time.Duration, retries int) {
	t.Helper()
	now := time.Now().UTC()
	var reset *string
	if resetIn != nil {
		reset = new(now.Add(*resetIn).Format(time.RFC3339Nano))
	}
	if err := w.j.Transact(t.Context(), func(tx *sql.Tx) error {
		if err := journal.SetBucketExhausted(t.Context(), tx, w.bucket, now.Add(-exhaustedAgo).Format(time.RFC3339Nano), reset); err != nil {
			return err
		}
		for range retries {
			if err := journal.NoteBucketRetry(t.Context(), tx, w.bucket); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func (w *bucketWorld) hold(t *testing.T) (string, error) {
	t.Helper()
	var id string
	err := w.j.Transact(t.Context(), func(tx *sql.Tx) error {
		var err error
		id, err = admission.HoldQuotaReservation(t.Context(), tx, admission.NewJournalReserver(billing.BuiltInCeilings(), EvaluateBucket), w.runID, bucketRec, bucketIdentity)
		return err
	})
	return id, err
}

func (w *bucketWorld) row(t *testing.T) (journal.BucketRow, error) {
	t.Helper()
	return w.j.BucketState(t.Context(), w.bucket)
}

func (w *bucketWorld) reservations(t *testing.T) int {
	t.Helper()
	var n int
	if err := w.j.Transact(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM reservations WHERE run_id = ?`, w.runID).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBucketRefusesNewWork(t *testing.T) {
	t.Parallel()
	hour := time.Hour
	for _, c := range []struct {
		name      string
		ago       time.Duration
		resetIn   *time.Duration
		retries   int
		wantGiven bool
	}{
		{"unknown reset, inside the first backoff", 20 * time.Second, nil, 0, false},
		{"authoritative reset still ahead", time.Minute, &hour, 0, false},
		{"retries consumed", time.Hour, nil, 3, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := newBucketWorld(t)
			w.bucket = admission.QuotaBucket(bucketRec, bucketIdentity)
			w.exhaust(t, c.ago, c.resetIn, c.retries)
			before, err := w.row(t)
			if err != nil {
				t.Fatal(err)
			}
			_, err = w.hold(t)
			var ex *admission.ExhaustedError
			if !errors.Is(err, billing.ErrAllowanceExhausted) || !errors.As(err, &ex) || ex.GiveUp != c.wantGiven {
				t.Fatalf("hold on a live exhausted bucket err = %v, want allowance_exhausted (giveUp %t)", err, c.wantGiven)
			}
			if n := w.reservations(t); n != 0 {
				t.Fatalf("a refused admission holds %d reservations, want 0", n)
			}
			after, err := w.row(t)
			if err != nil || after.RetriesUsed != before.RetriesUsed {
				t.Fatalf("a refusal changed the bucket row: %+v -> %+v, %v (refusals never increment)", before, after, err)
			}
		})
	}
	// A different bucket is unaffected.
	w := newBucketWorld(t)
	w.bucket = "some other bucket"
	w.exhaust(t, 20*time.Second, nil, 0)
	if _, err := w.hold(t); err != nil {
		t.Fatalf("hold on an unexhausted bucket: %v", err)
	}
}

func TestEvaluateBucketAdmitsAndClears(t *testing.T) {
	t.Parallel()
	past, future := -time.Hour, time.Hour
	for _, c := range []struct {
		name        string
		ago         time.Duration
		resetIn     *time.Duration
		retries     int
		wantRetries int64
		wantCleared bool
	}{
		{"first backoff due: counted, row kept", 2 * time.Minute, nil, 0, 1, false},
		{"second backoff due: counted, row kept", 6 * time.Minute, nil, 1, 2, false},
		{"third retry consumes the schedule: row kept, retries spent", 16 * time.Minute, nil, 2, 3, false},
		{"authoritative reset reached: row cleared", 2 * time.Hour, &past, 0, 1, true},
		{"authoritative reset ahead stays refused", time.Minute, &future, 0, 0, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := newBucketWorld(t)
			w.bucket = admission.QuotaBucket(bucketRec, bucketIdentity)
			w.exhaust(t, c.ago, c.resetIn, c.retries)
			_, err := w.hold(t)
			if c.wantRetries == 0 {
				if err == nil {
					t.Fatal("an early admission was admitted")
				}
				if row, rerr := w.row(t); rerr != nil || row.RetriesUsed != int64(c.retries) {
					t.Fatalf("refusal changed the row: %+v, %v", row, rerr)
				}
				return
			}
			if err != nil {
				t.Fatalf("a due re-admission was refused: %v", err)
			}
			row, rerr := w.row(t)
			switch {
			case c.wantCleared && !errors.Is(rerr, sql.ErrNoRows):
				t.Fatalf("bucket row after the last retry = %+v, %v; want it cleared", row, rerr)
			case !c.wantCleared && (rerr != nil || row.RetriesUsed != c.wantRetries):
				t.Fatalf("bucket row = %+v, %v; want retries_used %d", row, rerr, c.wantRetries)
			}
			if n := w.reservations(t); n != 1 {
				t.Fatalf("an admitted re-admission holds %d reservations, want 1", n)
			}
		})
	}
}

// TestSpentScheduleGivesUp pins AC-6.2's "then stop": after the unknown-reset
// schedule's three retries are consumed, the row stays at MaxBucketRetries
// and the next admission refuses with giveUp instead of admitting; a further
// native exhaustion signal does not restart the schedule.
func TestSpentScheduleGivesUp(t *testing.T) {
	t.Parallel()
	w := newBucketWorld(t)
	w.bucket = admission.QuotaBucket(bucketRec, bucketIdentity)
	w.exhaust(t, 16*time.Minute, nil, 0)
	holdNewRun := func() error {
		t.Helper()
		runID := ids.New("run")
		now := time.Now()
		err := w.j.Transact(t.Context(), func(tx *sql.Tx) error {
			if err := journal.InsertRun(t.Context(), tx, journal.RunRow{RunID: runID, State: "admission", AdapterID: "builtin/fake", SourceRepo: "/tmp/repo",
				TaskSHA256: "t", BillingPosture: "local-scripted", ExecutionProfile: "trusted-host", CreatedAt: now, UpdatedAt: now}); err != nil {
				return err
			}
			_, err := admission.HoldQuotaReservation(t.Context(), tx, admission.NewJournalReserver(billing.BuiltInCeilings(), EvaluateBucket), runID, bucketRec, bucketIdentity)
			return err
		})
		return err
	}
	for i := range 3 {
		if err := holdNewRun(); err != nil {
			t.Fatalf("scheduled retry %d was refused: %v", i+1, err)
		}
	}
	row, err := w.row(t)
	if err != nil || row.RetriesUsed != int64(admission.MaxBucketRetries) {
		t.Fatalf("bucket row = %+v, %v; want retries_used %d", row, err, admission.MaxBucketRetries)
	}
	err = holdNewRun()
	var exhausted *admission.ExhaustedError
	if !errors.As(err, &exhausted) || !exhausted.GiveUp {
		t.Fatalf("fourth admission err = %v; want giveUp ExhaustedError", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := w.j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.SetBucketExhausted(t.Context(), tx, w.bucket, now, nil)
	}); err != nil {
		t.Fatal(err)
	}
	if err := holdNewRun(); !errors.As(err, &exhausted) || !exhausted.GiveUp {
		t.Fatalf("admission after re-exhaustion err = %v; want still giveUp", err)
	}
}

var (
	bucketRec      = qualifyRecord("fake", "scripted-child-process (ndjson)", "none (no inference, no network)")
	bucketIdentity = ""
)

// exhaustionFixture is a repository, a task and a state directory for runs
// of the allowance-exhausted fake scenario.
type exhaustionFixture struct{ state, repo, task string }

func newExhaustionFixture(t *testing.T) exhaustionFixture {
	t.Helper()
	f := exhaustionFixture{state: t.TempDir(), repo: t.TempDir()}
	home := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...) // #nosec G204 -- fixed test argv only
		cmd.Dir = f.repo
		cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+home, "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "--quiet", "--initial-branch=main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(f.repo, "README.md"), []byte("demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "README.md")
	git("commit", "--quiet", "-m", "initial")
	f.task = filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(f.task, []byte("# Demo task\n\nWrite demo.txt.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f exhaustionFixture) run(t *testing.T, scenario string, hooks Hooks) (Outcome, admission.Decision, error) {
	t.Helper()
	d, err := admission.Decide(t.Context(), admission.Request{
		StateDir: f.state, Repo: f.repo, TaskFile: f.task, Adapter: admission.AdapterFake, Billing: admission.BillingLocalScripted,
		ExecutionProfile: admission.ProfileTrustedHost, Env: os.Environ(), NoChecks: true, Scenario: scenario,
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := Run(t.Context(), d, hooks)
	return out, d, err
}

func (f exhaustionFixture) journal(t *testing.T) *journal.Journal {
	t.Helper()
	j, err := journal.Open(t.Context(), f.state)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return j
}

func countEvents(t *testing.T, j *journal.Journal, runID, typ string) int {
	t.Helper()
	evs, err := j.Events(t.Context(), runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, ev := range evs {
		if ev.Type == typ {
			n++
		}
	}
	return n
}

func TestExhaustionPreservesCandidates(t *testing.T) {
	f := newExhaustionFixture(t)
	out, d, err := f.run(t, "allowance-exhausted", Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	if out.State != RunBlocked || out.Reason != "allowance_exhausted" || out.AttemptReason != "allowance_exhausted" {
		t.Fatalf("outcome = %s (%s), attempt reason %q; want blocked (allowance_exhausted)", out.State, out.Reason, out.AttemptReason)
	}
	j := f.journal(t)
	cand, err := j.Candidate(t.Context(), d.AttemptID)
	if err != nil || cand.Commit == "" {
		t.Fatalf("candidate = %+v, %v; want the partial work frozen", cand, err)
	}
	if countEvents(t, j, d.RunID, "candidate.frozen") != 1 {
		t.Fatal("exhaustion did not freeze exactly one candidate")
	}
	for _, name := range []string{"spool.jsonl", "worker.json"} {
		if _, err := os.Stat(filepath.Join(workers.AttemptDir(f.state, d.RunID, d.AttemptID), name)); err != nil {
			t.Errorf("session file %s missing: %v", name, err)
		}
	}
	// The bucket is recorded exhausted with its reset unknown, and the
	// reservation is released with evidence once the attempt is terminal.
	row, err := j.BucketState(t.Context(), admission.QuotaBucket(bucketRec, bucketIdentity))
	if err != nil || row.ResetAt != nil || row.RetriesUsed != 0 || row.ExhaustedAt == "" {
		t.Fatalf("bucket row = %+v, %v; want exhausted with an unknown reset", row, err)
	}
	var status, evidence string
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT status, COALESCE(release_evidence, '') FROM reservations WHERE run_id = ?`, d.RunID).Scan(&status, &evidence)
	}); err != nil {
		t.Fatal(err)
	}
	if status != "released" || !strings.Contains(evidence, "allowance_exhausted") {
		t.Fatalf("reservation = %s (%q), want released with exhaustion evidence", status, evidence)
	}
}

func TestNoSecondNativeLaunch(t *testing.T) {
	f := newExhaustionFixture(t)
	out, d, err := f.run(t, "allowance-exhausted", Hooks{})
	if err != nil || out.State != RunBlocked {
		t.Fatalf("first run = %s, %v", out.State, err)
	}
	j := f.journal(t)
	if got := countEvents(t, j, d.RunID, "attempt.launched"); got != 1 {
		t.Fatalf("native launches = %d, want exactly 1: MYTHHELM schedules no automatic retry", got)
	}
	// The bucket is live, so a second run is refused before any worker
	// starts, and the first run's evidence stays untouched.
	out2, d2, err := f.run(t, "allowance-exhausted", Hooks{})
	if err != nil && !errors.Is(err, billing.ErrAllowanceExhausted) {
		t.Fatal(err)
	}
	if out2.State != RunBlocked || out2.Reason != "allowance_exhausted" {
		t.Fatalf("second run = %s (%s), want blocked (allowance_exhausted)", out2.State, out2.Reason)
	}
	if got := countEvents(t, j, d2.RunID, "attempt.launched") + countEvents(t, j, d2.RunID, "attempt.launch_intent_recorded"); got != 0 {
		t.Fatalf("the refused run launched %d attempt events, want none", got)
	}
	if got := countEvents(t, j, d.RunID, "attempt.launched"); got != 1 {
		t.Fatalf("first run launches changed to %d", got)
	}
}

func TestReleaseOnlyAfterStop(t *testing.T) {
	f := newExhaustionFixture(t)
	db := rawOpen(t, filepath.Join(f.state, journal.DBName))
	status := func(runID string) string {
		var s string
		if err := db.QueryRowContext(t.Context(), `SELECT status FROM reservations WHERE run_id = ?`, runID).Scan(&s); err != nil {
			return "none"
		}
		return s
	}
	var live []string
	hooks := Hooks{Event: func(ev journal.Event) {
		switch ev.Type {
		case "attempt.launch_intent_recorded", "attempt.launched", "attempt.native_session", "attempt.progress":
			live = append(live, ev.Type+"="+status(ev.RunID))
		}
	}}
	out, d, err := f.run(t, "happy", hooks)
	if err != nil || out.State != RunReadyForReview {
		t.Fatalf("run = %s, %v", out.State, err)
	}
	if len(live) == 0 {
		t.Fatal("no live attempt events observed")
	}
	for _, seen := range live {
		if !strings.HasSuffix(seen, "=held") {
			t.Errorf("reservation not held while the attempt was live: %s", seen)
		}
	}
	var final, evidence string
	if err := db.QueryRowContext(t.Context(), `SELECT status, COALESCE(release_evidence, '') FROM reservations WHERE run_id = ?`, d.RunID).Scan(&final, &evidence); err != nil {
		t.Fatal(err)
	}
	if final != "released" || evidence == "" {
		t.Fatalf("reservation after the attempt ended = %s (%q), want released with evidence", final, evidence)
	}
}

func (w *gateWorld) holdRow(t *testing.T, bucket string) string {
	t.Helper()
	at := time.Now().UTC()
	id := ids.New("rsv")
	if err := w.j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.InsertReservation(t.Context(), tx, journal.ReservationRow{ReservationID: id, RunID: w.runID, Bucket: bucket, Scope: bucket,
			Owner: w.runID, Quantity: "unknown", Status: "held", ExpiresAt: at.Add(time.Hour).Format(time.RFC3339Nano), CreatedAt: at.Format(time.RFC3339Nano)})
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestHandleExhaustionRecordsBlocksAndReleases(t *testing.T) {
	t.Parallel()
	w := newGateWorld(t, gateEnvelope(), 1)
	id := w.holdRow(t, "bucket-1")
	if err := HandleExhaustion(t.Context(), w.j, w.prod, w.runID, id, nil); err != nil {
		t.Fatal(err)
	}
	run, err := w.j.Run(t.Context(), w.runID)
	if err != nil || run.State != string(RunBlocked) || run.Reason != "allowance_exhausted" {
		t.Fatalf("run = %+v, %v; want blocked (allowance_exhausted)", run, err)
	}
	row, err := w.j.BucketState(t.Context(), "bucket-1")
	if err != nil || row.ResetAt != nil || row.RetriesUsed != 0 {
		t.Fatalf("bucket = %+v, %v; want exhausted with the reset unknown", row, err)
	}
	res, err := w.j.Reservation(t.Context(), id)
	if err != nil || res.Status != "released" || res.ReleaseEvidence == "" {
		t.Fatalf("reservation = %+v, %v; want released with evidence", res, err)
	}

	// An authoritative reset is recorded exactly as given.
	r := newGateWorld(t, gateEnvelope(), 1)
	rid := r.holdRow(t, "bucket-2")
	reset := time.Date(2030, 3, 4, 5, 6, 7, 0, time.UTC)
	if err := HandleExhaustion(t.Context(), r.j, r.prod, r.runID, rid, &reset); err != nil {
		t.Fatal(err)
	}
	if row, err := r.j.BucketState(t.Context(), "bucket-2"); err != nil || row.ResetAt == nil || *row.ResetAt != reset.Format(time.RFC3339Nano) {
		t.Fatalf("bucket = %+v, %v; want reset %s", row, err, reset.Format(time.RFC3339Nano))
	}
}

func TestHandleExhaustionJournalsNothingOnFailure(t *testing.T) {
	t.Parallel()
	w := newGateWorld(t, gateEnvelope(), 1)
	before, err := w.j.Events(t.Context(), w.runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := HandleExhaustion(t.Context(), w.j, w.prod, w.runID, "rsv_missing", nil); err == nil {
		t.Fatal("an unknown reservation was accepted")
	}
	after, err := w.j.Events(t.Context(), w.runID, 0)
	if err != nil || len(after) != len(before) {
		t.Fatalf("events %d -> %d, %v: a failed handler must journal nothing", len(before), len(after), err)
	}
	if run, err := w.j.Run(t.Context(), w.runID); err != nil || run.State != string(RunExecuting) {
		t.Fatalf("run = %+v, %v; want it left executing", run, err)
	}
}

func TestNativeResultTouchesReservationHeartbeat(t *testing.T) {
	t.Parallel()
	f := newLedgerFixture(t)
	at := time.Now().UTC()
	id := ids.New("rsv")
	if err := f.j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.InsertReservation(t.Context(), tx, journal.ReservationRow{ReservationID: id, RunID: f.runID, Bucket: "b", Scope: "b",
			Owner: f.runID, Quantity: "unknown", Status: "held", ExpiresAt: at.Add(time.Hour).Format(time.RFC3339Nano), CreatedAt: at.Format(time.RFC3339Nano)})
	}); err != nil {
		t.Fatal(err)
	}
	if res, err := f.j.Reservation(t.Context(), id); err != nil || res.HeartbeatAt != "" {
		t.Fatalf("fresh reservation = %+v, %v; want no heartbeat", res, err)
	}
	f.append(t, f.event("wrk_1", "attempt.native_result", nativeResult(`null`, `null`)))
	res, err := f.j.Reservation(t.Context(), id)
	if err != nil || res.HeartbeatAt == "" || res.Status != "held" {
		t.Fatalf("reservation after a native result = %+v, %v; want a heartbeat and still held", res, err)
	}
}

func TestExpiredReservationOrphanedNeverReused(t *testing.T) {
	t.Parallel()
	w := newGateWorld(t, gateEnvelope(), 1)
	bucket := admission.QuotaBucket(bucketRec, bucketIdentity)
	first := w.holdRow(t, bucket)
	if err := orphanHeldReservations(t.Context(), w.j, w.runID); err != nil {
		t.Fatal(err)
	}
	if res, err := w.j.Reservation(t.Context(), first); err != nil || res.Status != "orphaned" {
		t.Fatalf("reservation = %+v, %v; want orphaned", res, err)
	}
	var second string
	if err := w.j.Transact(t.Context(), func(tx *sql.Tx) error {
		var err error
		second, err = admission.HoldQuotaReservation(t.Context(), tx, admission.NewJournalReserver(billing.BuiltInCeilings(), EvaluateBucket), w.runID, bucketRec, bucketIdentity)
		return err
	}); err != nil {
		t.Fatalf("hold after an orphan: %v", err)
	}
	if second == first {
		t.Fatalf("a new hold reused the orphaned id %s", first)
	}
	if res, err := w.j.Reservation(t.Context(), first); err != nil || res.Status != "orphaned" {
		t.Fatalf("the orphan = %+v, %v; want it left orphaned", res, err)
	}
	if res, err := w.j.Reservation(t.Context(), second); err != nil || res.Status != "held" {
		t.Fatalf("the new hold = %+v, %v; want held", res, err)
	}
}
