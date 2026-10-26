//go:build windows

package viewmodel

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestTransientReadErrClassifiesWindowsErrnos(t *testing.T) {
	t.Parallel()
	pathErr := func(errno windows.Errno) error { return &fs.PathError{Op: "open", Path: "task.md", Err: errno} }
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "sharing violation retries", err: windows.ERROR_SHARING_VIOLATION, want: true},
		{name: "lock violation retries", err: windows.ERROR_LOCK_VIOLATION, want: true},
		{name: "sharing violation in PathError retries", err: pathErr(windows.ERROR_SHARING_VIOLATION), want: true},
		{name: "lock violation in PathError retries", err: pathErr(windows.ERROR_LOCK_VIOLATION), want: true},
		{name: "wrapped PathError sharing violation retries", err: fmt.Errorf("ctx: %w", pathErr(windows.ERROR_SHARING_VIOLATION)), want: true},
		{name: "access denied is permanent", err: pathErr(windows.ERROR_ACCESS_DENIED), want: false},
		{name: "file not found is permanent", err: pathErr(windows.ERROR_FILE_NOT_FOUND), want: false},
		{name: "not exist sentinel is permanent", err: fs.ErrNotExist, want: false},
		{name: "path not found is permanent", err: pathErr(windows.ERROR_PATH_NOT_FOUND), want: false},
		{name: "other error is permanent", err: errors.New("other"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := transientReadErr(tc.err); got != tc.want {
				t.Errorf("transientReadErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// holdDenyingShare writes the task file and opens it with no share mode, so
// every other open fails with ERROR_SHARING_VIOLATION until the handle closes.
func holdDenyingShare(t *testing.T) (dir string, release func()) {
	t.Helper()
	dir = t.TempDir()
	path := filepath.Join(dir, "runs", "run-1", "task.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(taskBody), 0o600); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
	closed := false
	release = func() {
		if closed {
			return
		}
		closed = true
		if err := windows.CloseHandle(h); err != nil {
			t.Errorf("CloseHandle: %v", err)
		}
	}
	t.Cleanup(release)
	if _, err := os.ReadFile(path); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) { // #nosec G304 -- test temp dir
		t.Fatalf("read while held = %v, want ERROR_SHARING_VIOLATION", err)
	}
	return dir, release
}

func TestTaskGoalRecoversWhenRealShareDenyingHandleReleases(t *testing.T) {
	t.Parallel()
	dir, release := holdDenyingShare(t)
	reads := 0
	r := systemTaskReader()
	r.read = func(name string) ([]byte, error) {
		reads++
		return os.ReadFile(name) // #nosec G304 -- test temp dir
	}
	r.wait = func(ctx context.Context, d time.Duration) error {
		release()
		return waitCtx(ctx, d)
	}

	goal, err := taskGoal(context.Background(), r, dir, "run-1", taskDigest(taskBody))
	if err != nil {
		t.Fatalf("taskGoal: %v", err)
	}
	if goal != "Ship the thing" {
		t.Errorf("goal = %q, want %q", goal, "Ship the thing")
	}
	if reads != 2 {
		t.Errorf("reads = %d, want 2", reads)
	}
}

func TestTaskGoalRefusesWhileRealHandleStaysHeld(t *testing.T) {
	t.Parallel()
	dir, _ := holdDenyingShare(t)
	reads := 0
	r := systemTaskReader()
	r.read = func(name string) ([]byte, error) {
		reads++
		return os.ReadFile(name) // #nosec G304 -- test temp dir
	}

	_, err := taskGoal(context.Background(), r, dir, "run-1", taskDigest(taskBody))
	if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("err = %v, want wrapped ERROR_SHARING_VIOLATION", err)
	}
	if reads != 10 {
		t.Errorf("reads = %d, want 10", reads)
	}
}
