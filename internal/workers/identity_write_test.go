package workers

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRenameWithRetrySucceedsAfterTransientFailures(t *testing.T) {
	t.Parallel()
	calls := 0
	transient := errors.New("transient sharing conflict")
	err := renameWithRetry(func() error {
		calls++
		if calls < 3 {
			return transient
		}
		return nil
	}, func(err error) bool { return errors.Is(err, transient) }, 5, time.Millisecond)
	if err != nil || calls != 3 {
		t.Fatalf("renameWithRetry = %v after %d calls, want nil after 3", err, calls)
	}
}

func TestRenameWithRetryStopsAfterBudget(t *testing.T) {
	t.Parallel()
	calls := 0
	stuck := errors.New("still locked")
	err := renameWithRetry(func() error {
		calls++
		return stuck
	}, func(error) bool { return true }, 5, time.Millisecond)
	if !errors.Is(err, stuck) || calls != 6 {
		t.Fatalf("renameWithRetry = %v after %d calls, want the error after 6", err, calls)
	}
}

func TestRenameWithRetryFailsFastOnPermanentErrors(t *testing.T) {
	t.Parallel()
	calls := 0
	denied := &os.PathError{Op: "rename", Path: "worker.json", Err: errors.New("permanent")}
	err := renameWithRetry(func() error {
		calls++
		return denied
	}, func(error) bool { return false }, 5, time.Millisecond)
	if !errors.Is(err, denied) || calls != 1 {
		t.Fatalf("renameWithRetry = %v after %d calls, want one fast failure", err, calls)
	}
}

func TestWriteFileAtomicFailsOnMissingDir(t *testing.T) {
	t.Parallel()
	if err := writeFileAtomic(filepath.Join(t.TempDir(), "missing"), "worker.json", []byte("{}")); err == nil {
		t.Fatal("write to a missing directory must fail")
	}
}
