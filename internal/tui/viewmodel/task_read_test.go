package viewmodel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

var errTransient = errors.New("synthetic transient read failure")

const taskBody = "# Ship the thing\n\nbody\n"

func taskDigest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// scriptedReads returns results in order, repeating the last one, and counts calls.
type scriptedReads struct {
	results []error
	body    string
	calls   int
}

func (s *scriptedReads) read(string) ([]byte, error) {
	err := s.results[min(s.calls, len(s.results)-1)]
	s.calls++
	if err != nil {
		return nil, err
	}
	return []byte(s.body), nil
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
	reads := &scriptedReads{results: []error{transientPathErr(), transientPathErr(), transientPathErr(), nil}, body: taskBody}
	waits := &recordedWaits{}
	r := taskReader{read: reads.read, wait: waits.wait, transient: syntheticTransient}

	goal, err := taskGoal(context.Background(), r, t.TempDir(), "run-1", taskDigest(taskBody))
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
	reads := &scriptedReads{results: []error{transientPathErr()}}
	waits := &recordedWaits{}
	r := taskReader{read: reads.read, wait: waits.wait, transient: syntheticTransient}

	_, err := taskGoal(context.Background(), r, t.TempDir(), "run-1", taskDigest(taskBody))
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
	permanent := &fs.PathError{Op: "open", Path: "task.md", Err: syscall.Errno(5)}
	reads := &scriptedReads{results: []error{permanent}}
	waits := &recordedWaits{}
	r := taskReader{read: reads.read, wait: waits.wait, transient: syntheticTransient}

	_, err := taskGoal(context.Background(), r, t.TempDir(), "run-1", taskDigest(taskBody))
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
	calls := 0
	read := func(name string) ([]byte, error) {
		calls++
		return os.ReadFile(name) // #nosec G304 -- test temp dir
	}
	waits := &recordedWaits{}
	r := taskReader{read: read, wait: waits.wait, transient: transientReadErr}

	goal, err := taskGoal(context.Background(), r, t.TempDir(), "run-1", taskDigest(taskBody))
	if err != nil {
		t.Fatalf("taskGoal: %v", err)
	}
	if goal != "unknown" {
		t.Errorf("goal = %q, want unknown", goal)
	}
	if calls != 1 || len(waits.waits) != 0 {
		t.Errorf("reads = %d waits = %d, want 1 and 0", calls, len(waits.waits))
	}
}

func TestTaskGoalCancelledWhileWaitingStopsRetrying(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reads := &scriptedReads{results: []error{transientPathErr(), nil}, body: taskBody}
	wait := func(ctx context.Context, _ time.Duration) error {
		cancel()
		return waitCtx(ctx, time.Hour)
	}
	r := taskReader{read: reads.read, wait: wait, transient: syntheticTransient}

	_, err := taskGoal(ctx, r, t.TempDir(), "run-1", taskDigest(taskBody))
	if !errors.Is(err, context.Canceled) || !errors.Is(err, errTransient) {
		t.Fatalf("err = %v, want both cancellation and the transient cause", err)
	}
	if reads.calls != 1 {
		t.Errorf("reads = %d, want 1", reads.calls)
	}
}

func TestTaskGoalPreCancelledContextReadsNothing(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reads := &scriptedReads{results: []error{nil}, body: taskBody}
	waits := &recordedWaits{}
	r := taskReader{read: reads.read, wait: waits.wait, transient: syntheticTransient}

	_, err := taskGoal(ctx, r, t.TempDir(), "run-1", taskDigest(taskBody))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if reads.calls != 0 || len(waits.waits) != 0 {
		t.Errorf("reads = %d waits = %d, want 0 and 0", reads.calls, len(waits.waits))
	}
}

func TestTaskGoalDigestMismatchAfterTransientRecoveryFails(t *testing.T) {
	t.Parallel()
	reads := &scriptedReads{results: []error{transientPathErr(), nil}, body: "# Tampered\n"}
	waits := &recordedWaits{}
	r := taskReader{read: reads.read, wait: waits.wait, transient: syntheticTransient}

	goal, err := taskGoal(context.Background(), r, t.TempDir(), "run-1", taskDigest(taskBody))
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
		r.read = func(string) ([]byte, error) {
			calls++
			return nil, cause
		}
		_, err := taskGoal(context.Background(), r, t.TempDir(), "run-1", taskDigest(taskBody))
		if !errors.Is(err, errno) {
			t.Errorf("errno %d: err = %v, want it wrapped", errno, err)
		}
		if calls != 1 {
			t.Errorf("errno %d: reads = %d, want 1", errno, calls)
		}
	}
}
