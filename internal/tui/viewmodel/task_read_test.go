package viewmodel

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

var errTransient = errors.New("synthetic transient read failure")

const taskBody = "# Ship the thing\n\nbody\n"

func taskDigest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// writeTaskFile stores body as runID's admitted task file under dir.
func writeTaskFile(t *testing.T, dir, runID, body string) {
	t.Helper()
	path := filepath.Join(dir, "runs", runID, "task.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readGoal(ctx context.Context, r taskReader, dir, runID, wantSHA string) (string, error) {
	b, _, err := r.readFile(ctx, filepath.Join(dir, "runs", runID, "task.md"))
	return verifiedTaskGoal(runID, wantSHA, b, err)
}

// deniedThenFile fails the first denials reads with the scripted errors (the
// last repeating), then reads the real file; it counts every call.
type deniedThenFile struct {
	denials int
	errs    []error
	calls   int
}

func (d *deniedThenFile) read(ctx context.Context, name string) ([]byte, error) {
	d.calls++
	if d.calls <= d.denials {
		return nil, d.errs[min(d.calls, len(d.errs))-1]
	}
	return readOSFile(ctx, name)
}

type recordedWaits struct{ waits []time.Duration }

func (w *recordedWaits) wait(_ context.Context, d time.Duration) error {
	w.waits = append(w.waits, d)
	return nil
}

func (w *recordedWaits) total() time.Duration {
	var sum time.Duration
	for _, d := range w.waits {
		sum += d
	}
	return sum
}

func syntheticTransient(err error) bool { return errors.Is(err, errTransient) }

func transientPathErr() error {
	return &fs.PathError{Op: "open", Path: "task.md", Err: errTransient}
}

func TestTaskGoalRetriesTransientReadsThenReturnsVerifiedTitle(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTaskFile(t, dir, "run-1", taskBody)
	reads := &deniedThenFile{denials: 3, errs: []error{transientPathErr()}}
	waits := &recordedWaits{}
	r := taskReader{read: reads.read, wait: waits.wait, transient: syntheticTransient}

	goal, err := readGoal(context.Background(), r, dir, "run-1", taskDigest(taskBody))
	if err != nil {
		t.Fatalf("taskGoal: %v", err)
	}
	if goal != "Ship the thing" {
		t.Errorf("goal = %q, want %q", goal, "Ship the thing")
	}
	if reads.calls != 4 {
		t.Errorf("reads = %d, want 4", reads.calls)
	}
	if len(waits.waits) != 3 {
		t.Errorf("waits = %d, want 3", len(waits.waits))
	}
}

func TestTaskGoalFailsWithWrappedErrorAfterBoundedTransientReads(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTaskFile(t, dir, "run-1", taskBody)
	reads := &deniedThenFile{denials: 100, errs: []error{transientPathErr()}}
	waits := &recordedWaits{}
	r := taskReader{read: reads.read, wait: waits.wait, transient: syntheticTransient}

	_, err := readGoal(context.Background(), r, dir, "run-1", taskDigest(taskBody))
	if !errors.Is(err, errTransient) {
		t.Fatalf("err = %v, want wrapped transient error", err)
	}
	if _, ok := errors.AsType[*fs.PathError](err); !ok {
		t.Errorf("err = %v, want the *fs.PathError preserved", err)
	}
	if !strings.Contains(err.Error(), "reading admitted task for run-1") || !strings.Contains(err.Error(), "10 attempts") {
		t.Errorf("err = %q, want run and attempt context", err)
	}
	if reads.calls != 10 {
		t.Errorf("reads = %d, want 10", reads.calls)
	}
	if len(waits.waits) != 9 {
		t.Errorf("waits = %d, want 9", len(waits.waits))
	}
	if waits.total() > 500*time.Millisecond {
		t.Errorf("total wait = %v, want <= 500ms", waits.total())
	}
}

func TestTaskGoalPermanentReadErrorFailsWithoutRetry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTaskFile(t, dir, "run-1", taskBody)
	permanent := &fs.PathError{Op: "open", Path: "task.md", Err: syscall.Errno(5)}
	reads := &deniedThenFile{denials: 100, errs: []error{permanent}}
	waits := &recordedWaits{}
	r := taskReader{read: reads.read, wait: waits.wait, transient: syntheticTransient}

	_, err := readGoal(context.Background(), r, dir, "run-1", taskDigest(taskBody))
	if !errors.Is(err, syscall.Errno(5)) {
		t.Fatalf("err = %v, want errno 5", err)
	}
	if !strings.Contains(err.Error(), "reading admitted task for run-1") {
		t.Errorf("err = %q, want run context", err)
	}
	if reads.calls != 1 || len(waits.waits) != 0 {
		t.Errorf("reads = %d waits = %d, want 1 and 0", reads.calls, len(waits.waits))
	}
}

func TestTaskGoalMissingFileIsUnknownWithoutRetry(t *testing.T) {
	t.Parallel()
	reads := &deniedThenFile{}
	waits := &recordedWaits{}
	r := taskReader{read: reads.read, wait: waits.wait, transient: transientReadErr}

	goal, err := readGoal(context.Background(), r, t.TempDir(), "run-1", taskDigest(taskBody))
	if err != nil {
		t.Fatalf("taskGoal: %v", err)
	}
	if goal != "unknown" {
		t.Errorf("goal = %q, want unknown", goal)
	}
	if reads.calls != 1 || len(waits.waits) != 0 {
		t.Errorf("reads = %d waits = %d, want 1 and 0", reads.calls, len(waits.waits))
	}
}

func TestTaskGoalCancelledWhileWaitingStopsRetrying(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTaskFile(t, dir, "run-1", taskBody)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reads := &deniedThenFile{denials: 1, errs: []error{transientPathErr()}}
	wait := func(ctx context.Context, _ time.Duration) error {
		cancel()
		return waitCtx(ctx, time.Hour)
	}
	r := taskReader{read: reads.read, wait: wait, transient: syntheticTransient}

	_, err := readGoal(ctx, r, dir, "run-1", taskDigest(taskBody))
	if !errors.Is(err, context.Canceled) || !errors.Is(err, errTransient) {
		t.Fatalf("err = %v, want both cancellation and the transient cause", err)
	}
	if reads.calls != 1 {
		t.Errorf("reads = %d, want 1", reads.calls)
	}
}

// The timer wins the wait, then the loop notices cancellation before the
// next read: the last transient cause must survive beside context.Canceled.
func TestTaskGoalCancelledAfterWaitTimerKeepsTransientCause(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTaskFile(t, dir, "run-1", taskBody)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reads := &deniedThenFile{denials: 1, errs: []error{transientPathErr()}}
	wait := func(context.Context, time.Duration) error {
		cancel()
		return nil
	}
	r := taskReader{read: reads.read, wait: wait, transient: syntheticTransient}

	_, err := readGoal(ctx, r, dir, "run-1", taskDigest(taskBody))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if !errors.Is(err, errTransient) {
		t.Errorf("err = %v, want the last transient cause kept", err)
	}
	if reads.calls != 1 {
		t.Errorf("reads = %d, want 1", reads.calls)
	}
}

func TestTaskGoalPreCancelledContextReadsNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTaskFile(t, dir, "run-1", taskBody)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reads := &deniedThenFile{}
	waits := &recordedWaits{}
	r := taskReader{read: reads.read, wait: waits.wait, transient: syntheticTransient}

	_, err := readGoal(ctx, r, dir, "run-1", taskDigest(taskBody))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if reads.calls != 0 || len(waits.waits) != 0 {
		t.Errorf("reads = %d waits = %d, want 0 and 0", reads.calls, len(waits.waits))
	}
}

func TestTaskGoalDigestMismatchAfterTransientRecoveryFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTaskFile(t, dir, "run-1", "# Tampered\n")
	reads := &deniedThenFile{denials: 1, errs: []error{transientPathErr()}}
	waits := &recordedWaits{}
	r := taskReader{read: reads.read, wait: waits.wait, transient: syntheticTransient}

	goal, err := readGoal(context.Background(), r, dir, "run-1", taskDigest(taskBody))
	if err == nil || !strings.Contains(err.Error(), "admitted task digest mismatch for run-1") {
		t.Fatalf("goal = %q err = %v, want digest mismatch", goal, err)
	}
	if reads.calls != 2 || len(waits.waits) != 1 {
		t.Errorf("reads = %d waits = %d, want 2 and 1", reads.calls, len(waits.waits))
	}
}

func TestWaitCtxReturnsOnCancellationBeforeTimer(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitCtx(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitCtx = %v, want context.Canceled", err)
	}
}

func TestSystemTaskReaderNeverRetriesOffWindows(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows retries sharing and lock violations; covered in task_read_windows_test.go")
	}
	for _, errno := range []syscall.Errno{32, 33} {
		calls := 0
		cause := &fs.PathError{Op: "open", Path: "task.md", Err: errno}
		r := systemTaskReader()
		r.read = func(context.Context, string) ([]byte, error) {
			calls++
			return nil, cause
		}
		_, err := readGoal(context.Background(), r, t.TempDir(), "run-1", taskDigest(taskBody))
		if !errors.Is(err, errno) {
			t.Errorf("errno %d: err = %v, want it wrapped", errno, err)
		}
		if calls != 1 {
			t.Errorf("errno %d: reads = %d, want 1", errno, calls)
		}
	}
}

// heldRun journals one run in a fresh state dir; each write opens and closes
// the journal so a concurrent read-only load sees committed rows only.
type heldRun struct {
	t     *testing.T
	dir   string
	runID string
	seq   int64
}

func newHeldRun(t *testing.T, state string) *heldRun {
	t.Helper()
	h := &heldRun{t: t, dir: t.TempDir(), runID: "run-held"}
	writeTaskFile(t, h.dir, h.runID, taskBody)
	now := time.Now().UTC()
	row := journal.RunRow{
		RunID: h.runID, State: state, AdapterID: "builtin/fake", SourceRepo: "/repo",
		TaskSHA256: taskDigest(taskBody), BillingPosture: "local-scripted", ExecutionProfile: "trusted-host",
		CreatedAt: now, UpdatedAt: now,
	}
	h.append("run.created", `{}`, func(tx *sql.Tx) error { return journal.InsertRun(t.Context(), tx, row) })
	return h
}

func (h *heldRun) append(typ, payload string, project func(*sql.Tx) error) {
	h.t.Helper()
	j, err := journal.Open(h.t.Context(), h.dir)
	if err != nil {
		h.t.Fatalf("journal.Open: %v", err)
	}
	defer func() {
		if err := j.Close(); err != nil {
			h.t.Errorf("journal.Close: %v", err)
		}
	}()
	h.seq++
	ev := journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"), RunID: h.runID,
		ProducerID: "producer_test", ProducerSequence: h.seq, Generation: 1,
		ObservedAt: time.Now().UTC(), Type: typ, Payload: json.RawMessage(payload),
	}
	if err := j.Append(h.t.Context(), ev, project); err != nil {
		h.t.Fatalf("Append(%s): %v", typ, err)
	}
}

func (h *heldRun) complete(title string) {
	h.t.Helper()
	h.append("run.state_changed", `{"state":"completed"}`, func(tx *sql.Tx) error {
		return journal.SetRunState(h.t.Context(), tx, h.runID, "completed", "", time.Now().UTC())
	})
	r := supervisor.Receipt{
		"schema_version": 1, "run_id": h.runID, "state": "completed",
		"requested_outcome": map[string]any{"title": title},
	}
	sha, err := supervisor.WriteReceipt(filepath.Join(h.dir, "runs", h.runID), r)
	if err != nil {
		h.t.Fatal(err)
	}
	h.append("receipt.written", `{"schema_version":1,"sha256":"`+sha+`","path":"receipt.json"}`, nil)
}

func (h *heldRun) taskPath() string { return filepath.Join(h.dir, "runs", h.runID, "task.md") }

// heldTaskRead holds the first task read on a barrier, reports a transient
// denial once released, then reads the real file.
type heldTaskRead struct {
	held    chan struct{}
	release chan struct{}
	calls   int
}

func (h *heldTaskRead) read(ctx context.Context, name string) ([]byte, error) {
	h.calls++
	if h.calls == 1 {
		close(h.held)
		<-h.release
		return nil, transientPathErr()
	}
	return readOSFile(ctx, name)
}

func TestTaskReadDoesNotReturnBytesAfterCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r := systemTaskReader()
	r.read = func(context.Context, string) ([]byte, error) {
		cancel()
		return []byte("finished after cancellation"), nil
	}
	b, _, err := r.readFile(ctx, "ignored")
	if !errors.Is(err, context.Canceled) || b != nil {
		t.Fatalf("readFile = %q, %v, want cancellation without bytes", b, err)
	}
}

func TestLoadDoesNotHideCancellationWhenReceiptArrives(t *testing.T) {
	t.Parallel()
	h := newHeldRun(t, "executing")
	ctx := t.Context()
	held, release := make(chan struct{}), make(chan struct{})
	r := systemTaskReader()
	r.read = func(context.Context, string) ([]byte, error) { return nil, transientPathErr() }
	r.transient = syntheticTransient
	r.wait = func(context.Context, time.Duration) error {
		close(held)
		<-release
		return context.Canceled
	}
	result := make(chan error, 1)
	go func() { _, err := load(ctx, r, h.dir, h.runID); result <- err }()
	<-held
	h.complete("Final receipt")
	close(release)
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("load error = %v, want cancellation", err)
	}
}

// TestLoadAfterHeldTaskReadReflectsTheFinalRun holds the task read after the
// first assembly, changes the journal or task file while it is held, and
// asserts the returned snapshot reflects the journal after the delay.
func TestLoadAfterHeldTaskReadReflectsTheFinalRun(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		whileHeld func(h *heldRun)
		wantErr   string
		wantState string
		wantGoal  string
		wantSeq   int64
		receipt   bool
	}{
		{
			name:      "completion during the held read reaches state receipt and sequence",
			whileHeld: func(h *heldRun) { h.complete("Receipt title") },
			wantState: "completed", wantGoal: "Receipt title", wantSeq: 3, receipt: true,
		},
		{
			name: "completion with the task file removed still uses the receipt",
			whileHeld: func(h *heldRun) {
				h.complete("Receipt title")
				if err := os.Remove(h.taskPath()); err != nil {
					h.t.Fatal(err)
				}
			},
			wantState: "completed", wantGoal: "Receipt title", wantSeq: 3, receipt: true,
		},
		{
			name:      "progress without completion keeps the verified task title",
			whileHeld: func(h *heldRun) { h.append("attempt.progress", `{"assistant_turns":4}`, nil) },
			wantState: "executing", wantGoal: "Ship the thing", wantSeq: 2,
		},
		{
			name: "task file tampered during the hold is refused",
			whileHeld: func(h *heldRun) {
				if err := os.WriteFile(h.taskPath(), []byte("# Tampered\n"), 0o600); err != nil {
					h.t.Fatal(err)
				}
			},
			wantErr: "admitted task digest mismatch",
		},
		{
			name: "run digest changed during the hold is refused",
			whileHeld: func(h *heldRun) {
				h.append("run.task_changed", `{}`, func(tx *sql.Tx) error {
					_, err := tx.ExecContext(h.t.Context(), `UPDATE runs SET task_sha256 = ? WHERE run_id = ?`,
						taskDigest("# Another task\n"), h.runID)
					return err
				})
			},
			wantErr: "admitted task digest mismatch",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHeldRun(t, "executing")
			held := &heldTaskRead{held: make(chan struct{}), release: make(chan struct{})}
			release := sync.OnceFunc(func() { close(held.release) })
			t.Cleanup(release)
			r := taskReader{read: held.read, wait: (&recordedWaits{}).wait, transient: syntheticTransient}
			type result struct {
				snap Snapshot
				err  error
			}
			done := make(chan result, 1)
			go func() {
				snap, err := load(t.Context(), r, h.dir, h.runID)
				done <- result{snap, err}
			}()
			select {
			case <-held.held:
			case got := <-done:
				t.Fatalf("load returned before reading the task file: %+v, %v", got.snap.Run, got.err)
			}
			tc.whileHeld(h)
			release()
			got := <-done

			if tc.wantErr != "" {
				if got.err == nil || !strings.Contains(got.err.Error(), tc.wantErr) {
					t.Fatalf("load = %+v, %v; want error %q", got.snap.Run, got.err, tc.wantErr)
				}
				return
			}
			if got.err != nil {
				t.Fatalf("load: %v", got.err)
			}
			if got.snap.Run.State != tc.wantState {
				t.Errorf("state = %q, want %q", got.snap.Run.State, tc.wantState)
			}
			if got.snap.Goal != tc.wantGoal {
				t.Errorf("goal = %q, want %q", got.snap.Goal, tc.wantGoal)
			}
			if got.snap.LastRunSeq != tc.wantSeq {
				t.Errorf("LastRunSeq = %d, want %d", got.snap.LastRunSeq, tc.wantSeq)
			}
			if (got.snap.Receipt != nil) != tc.receipt {
				t.Errorf("receipt = %v, want present=%v", got.snap.Receipt, tc.receipt)
			}
		})
	}
}

func TestLoadReceiptBackedGoalNeverReadsTheTaskFile(t *testing.T) {
	t.Parallel()
	h := newHeldRun(t, "executing")
	h.complete("Receipt title")
	if err := os.Remove(h.taskPath()); err != nil {
		t.Fatal(err)
	}
	reads := 0
	r := taskReader{
		read: func(context.Context, string) ([]byte, error) {
			reads++
			return nil, errors.New("task file read for a receipt-backed run")
		},
		wait:      (&recordedWaits{}).wait,
		transient: syntheticTransient,
	}
	snap, err := load(t.Context(), r, h.dir, h.runID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if snap.Goal != "Receipt title" || snap.Run.State != "completed" {
		t.Errorf("goal = %q state = %q, want the receipt title on the completed run", snap.Goal, snap.Run.State)
	}
	if reads != 0 {
		t.Errorf("task reads = %d, want 0", reads)
	}
}
