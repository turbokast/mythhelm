//go:build windows

package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/turbokast/mythhelm/internal/adapter"
)

// TestWindowsRunnerParentJobSpike measures Task 3's prerequisite; it does not
// qualify process ownership or assert that a runner permits native launch.
// Keep this independent of the production parentJob implementation: production
// changes wait for measurements from both native Windows runner architectures.
func TestWindowsRunnerParentJobSpike(t *testing.T) {
	t.Parallel()
	isProcessInJob := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")
	if err := isProcessInJob.Find(); err != nil {
		t.Fatalf("resolve IsProcessInJob: %v", err)
	}
	var inJob int32
	ok, _, callErr := isProcessInJob.Call(
		uintptr(windows.CurrentProcess()), 0, uintptr(unsafe.Pointer(&inJob)),
	)
	if ok == 0 {
		t.Fatalf("IsProcessInJob(GetCurrentProcess(), 0): %v", callErr)
	}
	if inJob == 0 {
		t.Logf("parent_job_spike goos=%s goarch=%s in_job=false limit_flags=not_queried kill_on_close=false", runtime.GOOS, runtime.GOARCH)
		return
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	err := windows.QueryInformationJobObject(0, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil)
	if err != nil {
		t.Fatalf("QueryInformationJobObject(0, JobObjectExtendedLimitInformation): %v", err)
	}
	flags := limits.BasicLimitInformation.LimitFlags
	killOnClose := flags&windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE != 0
	t.Logf("parent_job_spike goos=%s goarch=%s in_job=true limit_flags=0x%08x kill_on_close=%t", runtime.GOOS, runtime.GOARCH, flags, killOnClose)
	if killOnClose {
		t.Log("Task 3 production work must stop for a reviewed parent-job policy decision (ADR 0017 D11)")
	}
}

func TestNativeCreatedSuspended(t *testing.T) {
	t.Parallel()
	if nativeAttr().CreationFlags&windows.CREATE_SUSPENDED == 0 {
		t.Fatal("native launch lacks CREATE_SUSPENDED; native can run before assignment")
	}
}

func TestJobHandleNotInheritable(t *testing.T) {
	t.Parallel()
	j, err := newJob()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.close() }()
	var flags uint32
	get := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetHandleInformation")
	ok, _, err := get.Call(uintptr(j.h), uintptr(unsafe.Pointer(&flags)))
	if ok == 0 {
		t.Fatal(err)
	}
	if flags&windows.HANDLE_FLAG_INHERIT != 0 {
		t.Fatal("job handle is inheritable")
	}
}

func TestJobHasKillOnCloseAndNoBreakawayLimits(t *testing.T) {
	t.Parallel()
	j, err := newJob()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.close() }()
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := windows.QueryInformationJobObject(j.h, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil); err != nil {
		t.Fatal(err)
	}
	if got := limits.BasicLimitInformation.LimitFlags; got != windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE {
		t.Fatalf("job limit flags = 0x%x, want kill-on-close only (0x2000)", got)
	}
}

func TestParentJobLaunchPreparationFailsClosed(t *testing.T) {
	t.Parallel()
	denied := windows.ERROR_ACCESS_DENIED
	for _, tc := range []struct {
		name string
		ops  jobOps
	}{
		{"kill-on-close", jobOps{parentJob: func() (bool, error) { return true, nil }}},
		{"query-error", jobOps{parentJob: func() (bool, error) { return false, denied }}},
		{"create-error", jobOps{parentJob: func() (bool, error) { return false, nil }, newJob: func() (*jobObject, error) { return nil, denied }}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			l := &launcher{w: &worker{jobOps: tc.ops}}
			if err := l.prepareJob(); err == nil {
				t.Fatal("launch preparation accepted an unavailable ownership boundary")
			}
		})
	}
}

func TestNativeIsInJobBeforeResume(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	w := &worker{}
	observed := false
	w.jobOps.beforeResume = func(j *jobObject) error {
		members, err := j.members()
		if err != nil {
			return err
		}
		id, err := ReadIdentity(a.dir)
		if err != nil {
			return err
		}
		if id.NativePID == nil || id.NativeStartTime == nil {
			return errors.New("native identity not persisted before assignment")
		}
		if len(members) != 1 || members[0].PID != *id.NativePID || !members[0].StartTime.Equal(*id.NativeStartTime) {
			return fmt.Errorf("before resume members=%+v, identity=%+v", members, id)
		}
		observed = true
		return nil
	}
	if err := a.runInProcess(t, a.launch(t, "happy"), w); err != nil {
		t.Fatal(err)
	}
	if state, reason := final(t, a.events(t)); state != "succeeded_native" {
		t.Fatalf("state=%s/%s stopped=%s", state, reason, find(t, a.events(t), "attempt.stopped").Payload)
	}
	if !observed {
		t.Fatal("native was not observed in job before resume")
	}
}

func TestJobCreateFailureEndsLaunchFailed(t *testing.T) {
	t.Parallel()
	checkJobLaunchFailure(t, jobOps{newJob: func() (*jobObject, error) { return nil, windows.ERROR_ACCESS_DENIED }}, false, false)
}
func TestKillOnCloseParentJobRefused(t *testing.T) {
	t.Parallel()
	checkJobLaunchFailure(t, jobOps{parentJob: func() (bool, error) { return true, nil }}, false, false)
}
func TestParentJobQueryErrorFailsLaunch(t *testing.T) {
	t.Parallel()
	checkJobLaunchFailure(t, jobOps{parentJob: func() (bool, error) { return false, windows.ERROR_ACCESS_DENIED }}, false, false)
}
func TestAssignFailureTerminatesSuspendedNative(t *testing.T) {
	t.Parallel()
	checkJobLaunchFailure(t, jobOps{assign: func(*jobObject, windows.Handle) error { return windows.ERROR_ACCESS_DENIED }}, true, false)
}
func TestUnconfirmedTerminationIsStopUnconfirmed(t *testing.T) {
	t.Parallel()
	checkJobLaunchFailure(t, jobOps{
		assign:          func(*jobObject, windows.Handle) error { return windows.ERROR_ACCESS_DENIED },
		terminateNative: func(*os.Process) error { return windows.ERROR_ACCESS_DENIED },
	}, true, true)
}
func TestNativeStartTimeFailureTerminatesSuspendedNative(t *testing.T) {
	t.Parallel()
	checkJobLaunchFailure(t, jobOps{startTime: func(int) (time.Time, error) { return time.Time{}, windows.ERROR_ACCESS_DENIED }}, true, false)
}

func checkJobLaunchFailure(t *testing.T, ops jobOps, spawned, unconfirmed bool) {
	t.Helper()
	a := newAttempt(t)
	var native windows.Handle
	ops.afterSpawn = func(pid int) error {
		h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, uint32(pid)) //nolint:gosec // G115: Windows PID.
		native = h
		return err
	}
	t.Cleanup(func() {
		if native != 0 {
			_ = windows.TerminateProcess(native, 1)
			_, _ = windows.WaitForSingleObject(native, 5000)
			_ = windows.CloseHandle(native)
		}
	})
	w := &worker{jobOps: ops}
	if err := a.runInProcess(t, a.launch(t, "happy"), w); err != nil {
		t.Fatal(err)
	}
	events := a.events(t)
	state, reason := final(t, events)
	if unconfirmed {
		if state != "interrupted" || reason != "stop_unconfirmed" {
			t.Fatalf("state=%s/%s, want interrupted/stop_unconfirmed", state, reason)
		}
	} else if state != "failed_native" || reason != "launch_failed" {
		t.Fatalf("state=%s/%s, want failed_native/launch_failed", state, reason)
	}
	var report struct {
		Confirmed   bool   `json:"confirmed"`
		LaunchError string `json:"launch_error"`
	}
	event := find(t, events, "attempt.stopped")
	if event.Type != "attempt.stopped" {
		t.Fatalf("event type=%q", event.Type)
	}
	if err := json.Unmarshal(event.Payload, &report); err != nil {
		t.Fatal(err)
	}
	if report.Confirmed == unconfirmed || report.LaunchError == "" {
		t.Fatalf("stopped=%+v", report)
	}
	if ops.parentJob == nil && !strings.Contains(report.LaunchError, "Win32 error 5") {
		t.Fatalf("launch error lacks Win32 code: %q", report.LaunchError)
	}
	if spawned != (native != 0) {
		t.Fatalf("spawned=%t, retained native handle=%v", spawned, native)
	}
	if native != 0 {
		status, err := windows.WaitForSingleObject(native, 0)
		if err != nil {
			t.Fatal(err)
		}
		want := uint32(windows.WAIT_OBJECT_0)
		if unconfirmed {
			want = uint32(windows.WAIT_TIMEOUT)
		}
		if status != want {
			t.Fatalf("native exit wait=%d, want %d", status, want)
		}
	}
}

func TestParentJobChild(t *testing.T) {
	// Re-executed through TestMain with a private child environment.
	if os.Getenv("MYTHHELM_TEST_PARENT_JOB") != "1" {
		t.Skip("helper executes only in TestParentJobSemantics child")
	}
	kill, err := parentJob()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("parent_job_result=%t\n", kill)
}

func TestParentJobSemantics(t *testing.T) {
	t.Parallel()
	runner, err := parentJob()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("runner parentJob=%t", runner)
	for _, tc := range []struct {
		name         string
		custom, kill bool
	}{
		{"runner", false, runner}, {"non-kill-job", true, false}, {"kill-on-close-job", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(ctx, exe, "-test.run=^TestParentJobChild$", "-test.v")
			cmd.Env = []string{"MYTHHELM_TEST_PARENT_JOB=1", "SystemRoot=" + os.Getenv("SystemRoot"), "GORACE=atexit_sleep_ms=0"}
			cmd.SysProcAttr = nativeAttr()
			var output bytes.Buffer
			cmd.Stdout, cmd.Stderr = &output, &output
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			waited := false
			defer func() {
				if !waited {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			}()
			if tc.custom {
				j, err := newJob()
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = j.close() }()
				if !tc.kill {
					var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
					if _, err := windows.SetInformationJobObject(j.h, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
						t.Fatal(err)
					}
				}
				h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid)) //nolint:gosec // G115: Windows PID.
				if err != nil {
					t.Fatal(err)
				}
				err = j.assign(h)
				_ = windows.CloseHandle(h)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := resumeNative(cmd.Process.Pid); err != nil {
				t.Fatal(err)
			}
			err = cmd.Wait()
			waited = true
			if err != nil {
				t.Fatalf("child: %v; %s", err, output.String())
			}
			want := fmt.Sprintf("parent_job_result=%t", tc.kill)
			if !strings.Contains(output.String(), want) {
				t.Fatalf("child output=%q, want %q", output.String(), want)
			}
		})
	}
}

func TestJobHasNoBreakawayLimits(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	w := &worker{}
	observed := false
	// Test-owned handles ensure an incorrectly permitted breakaway is still
	// cleaned up; a report PID alone never authorizes signalling it.
	var escaped windows.Handle
	defer func() {
		if escaped != 0 {
			_ = windows.TerminateProcess(escaped, 1)
			_, _ = windows.WaitForSingleObject(escaped, 5000)
			_ = windows.CloseHandle(escaped)
		}
	}()
	w.jobOps.afterResume = func() error {
		deadline := time.NewTimer(15 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-deadline.C:
				return errors.New("tree fixture did not report child, grandchild and breakaway before deadline")
			case <-tick.C:
				f, err := os.Open(filepath.Join(a.workdir, "tree.jsonl"))
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				if err != nil {
					return err
				}
				data, err := io.ReadAll(io.LimitReader(f, 1<<20))
				_ = f.Close()
				if err != nil {
					return err
				}
				dec := json.NewDecoder(bytes.NewReader(data))
				dec.DisallowUnknownFields()
				roles := map[string]bool{}
				var breakaway struct {
					Role      string    `json:"role"`
					PID       uint32    `json:"pid"`
					StartTime time.Time `json:"start_time"`
					Error     *uint32   `json:"error"`
				}
				for {
					var row struct {
						Role      string    `json:"role"`
						PID       uint32    `json:"pid"`
						StartTime time.Time `json:"start_time"`
						Error     *uint32   `json:"error"`
					}
					err := dec.Decode(&row)
					if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
						break
					}
					if err != nil {
						return err
					}
					if roles[row.Role] {
						return fmt.Errorf("duplicate tree role %q", row.Role)
					}
					roles[row.Role] = true
					if row.Role == "breakaway" {
						breakaway = row
					}
				}
				if !roles["child"] || !roles["grandchild"] || !roles["breakaway"] {
					continue
				}
				if breakaway.PID != 0 {
					h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, breakaway.PID)
					if err != nil {
						return err
					}
					var created, exited, kernel, user windows.Filetime
					err = windows.GetProcessTimes(h, &created, &exited, &kernel, &user)
					if err != nil || !time.Unix(0, created.Nanoseconds()).Equal(breakaway.StartTime) {
						_ = windows.CloseHandle(h)
						return errors.New("breakaway identity could not be confirmed")
					}
					escaped = h
				}
				if breakaway.Error == nil || *breakaway.Error != 5 || breakaway.PID != 0 {
					return fmt.Errorf("breakaway=%+v, want error 5 and no PID", breakaway)
				}
				observed = true
				return errors.New("test ends launch after proving breakaway denial")
			}
		}
	}
	if err := a.runInProcess(t, a.launch(t, "tree"), w); err != nil {
		t.Fatal(err)
	}
	if !observed {
		t.Fatalf("breakaway denial not observed: %s", find(t, a.events(t), "attempt.stopped").Payload)
	}
	if state, reason := final(t, a.events(t)); state != "failed_native" || reason != "launch_failed" {
		t.Fatalf("cleanup state=%s/%s", state, reason)
	}
}

func TestRepeatedLaunchDoesNotStopExistingNative(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	launch := a.launch(t, "slow")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, launch.Path, launch.Args...)
	cmd.Dir, cmd.Env = launch.Dir, launch.Env
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &ownedProc{cmd: cmd}
	defer func() {
		_ = cmd.Process.Kill()
		if !p.waited.Load() {
			_ = p.Wait()
		}
	}()
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid)) //nolint:gosec // G115: Windows PID.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	if state, err := windows.WaitForSingleObject(handle, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("native not live before repeat: %d %v", state, err)
	}
	l := &launcher{w: &worker{}, proc: p}
	if _, err := l.Launch(ctx, adapter.ProcSpec{}); err == nil {
		t.Fatal("second launch was accepted")
	}
	if state, err := windows.WaitForSingleObject(handle, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("repeat launch stopped the existing native: %d %v", state, err)
	}
}

func TestConfirmationHandleFailureStillTerminatesNative(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	var native windows.Handle
	w := &worker{jobOps: jobOps{
		assign:         func(*jobObject, windows.Handle) error { return windows.ERROR_ACCESS_DENIED },
		openWaitHandle: func(int) (windows.Handle, error) { return 0, windows.ERROR_ACCESS_DENIED },
		afterSpawn: func(pid int) error {
			var err error
			native, err = windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid)) //nolint:gosec // G115: Windows PID.
			return err
		},
	}}
	t.Cleanup(func() {
		if native != 0 {
			_ = windows.TerminateProcess(native, 1)
			_, _ = windows.WaitForSingleObject(native, 5000)
			_ = windows.CloseHandle(native)
		}
	})
	if err := a.runInProcess(t, a.launch(t, "happy"), w); err != nil {
		t.Fatal(err)
	}
	if state, reason := final(t, a.events(t)); state != "interrupted" || reason != "stop_unconfirmed" {
		t.Fatalf("state=%s/%s, want interrupted/stop_unconfirmed", state, reason)
	}
	if native == 0 {
		t.Fatal("native was never spawned")
	}
	if status, err := windows.WaitForSingleObject(native, 5000); err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("native still alive after confirmation handle failure: wait=%d err=%v", status, err)
	}
}

func TestIdentityWriteFailureTerminatesSuspendedNative(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	var native windows.Handle
	spawnedSuspended, reachedResume := false, false
	w := &worker{writeFile: func(dir, name string, body []byte) error {
		var id Identity
		if err := json.Unmarshal(body, &id); err != nil {
			return err
		}
		if id.NativePID != nil {
			return errors.New("identity persistence failed")
		}
		return WriteFileAtomic(dir, name, body)
	}}
	w.jobOps.afterSpawn = func(pid int) error {
		var err error
		native, err = windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid)) //nolint:gosec // G115: Windows PID.
		if err != nil {
			return err
		}
		assertNativeThreadSuspended(t, pid)
		spawnedSuspended = true
		return nil
	}
	w.jobOps.beforeResume = func(*jobObject) error { reachedResume = true; return nil }
	t.Cleanup(func() {
		if native != 0 {
			_ = windows.TerminateProcess(native, 1)
			_, _ = windows.WaitForSingleObject(native, 5000)
			_ = windows.CloseHandle(native)
		}
	})
	if err := a.runInProcess(t, a.launch(t, "happy"), w); err != nil {
		t.Fatal(err)
	}
	if !spawnedSuspended || reachedResume {
		t.Fatalf("spawned suspended=%t, reached resume=%t", spawnedSuspended, reachedResume)
	}
	if status, err := windows.WaitForSingleObject(native, 5000); err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("native survived identity failure: wait=%d err=%v", status, err)
	}
	events := a.events(t)
	for _, event := range events {
		if event.Type == "attempt.launched" {
			t.Fatal("unpersisted native was announced as launched")
		}
	}
	if state, reason := final(t, events); state != "failed_native" || reason != "launch_failed" {
		t.Fatalf("state=%s/%s, want failed_native/launch_failed", state, reason)
	}
	if !stoppedOf(t, events).Confirmed {
		t.Fatal("suspended-native cleanup was not confirmed")
	}
}

// Temporarily increment and restore the suspend count without permitting the
// native's first instruction to run. This measures suspension, not just flags.
func assertNativeThreadSuspended(t *testing.T, pid int) {
	t.Helper()
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	var entry windows.ThreadEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	count := 0
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != uint32(pid) {
			continue
		} //nolint:gosec // G115: Windows PID.
		count++
		h, openErr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if openErr != nil {
			t.Fatal(openErr)
		}
		previous, _, suspendErr := windows.NewLazySystemDLL("kernel32.dll").NewProc("SuspendThread").Call(uintptr(h))
		if uint32(previous) == ^uint32(0) { //nolint:gosec // G115: Win32 DWORD return value.
			_ = windows.CloseHandle(h)
			t.Fatal(suspendErr)
		}
		restored, resumeErr := windows.ResumeThread(h)
		_ = windows.CloseHandle(h)
		if previous != 1 || restored != 2 || resumeErr != nil {
			t.Fatalf("native thread suspend count=%d, restore=%d err=%v, want 1 then 2", previous, restored, resumeErr)
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("native threads=%d, want one suspended thread", count)
	}
}
