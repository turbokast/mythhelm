//go:build windows

package workers

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestRetryIdentityWriteWindowsSharingOnly(t *testing.T) {
	// Unlike the read path, the write path retries ERROR_ACCESS_DENIED:
	// renaming over an open reader reports 5, observed in CI as
	// "rename ... worker.json: Access is denied." A genuine ACL denial
	// still fails after the bounded budget.
	for _, code := range []syscall.Errno{5, 32, 33} {
		if !retryIdentityWrite(&os.PathError{Op: "rename", Path: "worker.json", Err: code}) {
			t.Fatalf("Windows error %d must be retried", code)
		}
	}
	if retryIdentityWrite(&os.PathError{Op: "rename", Path: "worker.json", Err: syscall.Errno(2)}) {
		t.Fatal("missing path must not be retried")
	}
}

func TestLaunchRecordsNativeStartTimeWindows(t *testing.T) { // I18 (v2 §2): persisted launch identity matches the native.
	t.Parallel()
	a := newAttempt(t)
	w := &worker{}
	var observed time.Time
	w.jobOps.afterResume = func() error {
		var err error
		observed, err = ProcessStartTime(*w.id.NativePID)
		return err
	}
	if err := a.runInProcess(t, a.launch(t, "happy"), w); err != nil {
		t.Fatal(err)
	}
	id, err := ReadIdentity(a.dir)
	if err != nil {
		t.Fatal(err)
	}
	if observed.IsZero() || id.NativeStartTime == nil || !id.NativeStartTime.Equal(observed) {
		t.Fatalf("worker.json creation time = %v, observed = %v", id.NativeStartTime, observed)
	}
	ev := find(t, a.events(t), "attempt.launched")
	if ev.Type != "attempt.launched" {
		t.Fatalf("type = %q", ev.Type)
	}
	var payload struct {
		NativePID       *int       `json:"native_pid"`
		NativeStartTime *time.Time `json:"native_start_time"`
	}
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.NativePID == nil || *payload.NativePID != *id.NativePID || payload.NativeStartTime == nil || !payload.NativeStartTime.Equal(observed) {
		t.Fatalf("launched identity = %+v, want PID %d time %v", payload, *id.NativePID, observed)
	}
}

func TestStartTimeReadFailureTerminatesJob(t *testing.T) { // I09/I18 (v2 §2): an unknown identity never runs.
	t.Parallel()
	for _, unconfirmed := range []bool{false, true} {
		t.Run(fmt.Sprintf("termination_fails=%t", unconfirmed), func(t *testing.T) {
			t.Parallel()
			readSuspended, assigned, resumed, empty := false, false, false, false
			ops := jobOps{
				startTime: func(pid int) (time.Time, error) {
					assertNativeThreadSuspended(t, pid)
					readSuspended = true
					return time.Time{}, windows.ERROR_ACCESS_DENIED
				},
				assign:       func(j *jobObject, h windows.Handle) error { assigned = true; return j.assign(h) },
				beforeResume: func(*jobObject) error { resumed = true; return nil },
				members: func(j *jobObject) ([]ProcessIdentity, error) {
					members, err := j.members()
					if err == nil && len(members) == 0 {
						empty = true
					}
					return members, err
				},
			}
			if unconfirmed {
				ops.terminateNative = func(*os.Process) error { return windows.ERROR_ACCESS_DENIED }
			}
			checkJobLaunchFailure(t, ops, true, unconfirmed)
			if !readSuspended || assigned || resumed || !empty {
				t.Fatalf("read suspended=%t assigned=%t resumed=%t empty job=%t", readSuspended, assigned, resumed, empty)
			}
		})
	}
}
