//go:build windows

package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestProcessStartTimeRejectsExitedWindows(t *testing.T) {
	t.Parallel()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^$")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid)) //nolint:gosec // G115: child PID originates from Windows.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.CloseHandle(h) }()
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if start, err := ProcessStartTime(cmd.Process.Pid); !errors.Is(err, ErrNoProcess) {
		t.Fatalf("exited process retained by handle: start=%s err=%v, want ErrNoProcess", start, err)
	}
}

// TestCrashWorkerHelper is selected only by the re-executed crash-worker mode.
func TestCrashWorkerHelper(t *testing.T) {
	if len(os.Args) != 7 || os.Args[2] != "--" {
		return
	}
	state, runID, attemptID, boundary := os.Args[3], os.Args[4], os.Args[5], os.Args[6]
	l, err := readLaunch(os.Stdin)
	if err != nil {
		t.Fatal(err)
	}
	dir := AttemptDir(state, runID, attemptID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	w := &worker{dir: dir, runID: runID, attemptID: attemptID, launch: l, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	crash := func() error { os.Exit(3); return nil }
	spawned := func(pid int) error {
		start, err := ProcessStartTime(pid)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(ProcessIdentity{PID: pid, StartTime: start})
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "spawned.json"), raw, 0o600)
	}
	w.jobOps.afterSpawn = spawned
	switch boundary {
	case "afterIntent":
		w.jobOps.afterIntent = crash
	case "afterJob":
		w.jobOps.afterJob = func(*jobObject) error { return crash() }
	case "afterSpawn":
		w.jobOps.afterSpawn = func(pid int) error {
			if err := spawned(pid); err != nil {
				return err
			}
			return crash()
		}
	case "afterStart":
		w.jobOps.afterStart = func(int) error { return crash() }
	case "beforeResume":
		w.jobOps.beforeResume = func(*jobObject) error { return crash() }
	case "afterResume":
		w.jobOps.afterResume = func() error {
			// Wait for real child and grandchild identities before the crash; closing
			// the job must kill actual descendants, rather than an empty launch.
			a := attempt{dir: dir, workdir: l.Dir}
			_, _, err := waitTree(t, a)
			if err != nil {
				return err
			}
			return crash()
		}
	default:
		t.Fatal("unknown boundary", boundary)
	}
	if err := w.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Fatal("worker returned without reaching crash hook")
}

func crashBoundary(t *testing.T, boundary string) (attempt, Identity, *ProcessIdentity) {
	t.Helper()
	a := newAttempt(t)
	raw, err := json.Marshal(a.launch(t, "tree"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), os.Args[0], "__crash-worker", a.state, a.runID, a.attemptID, boundary)
	cmd.Stdin = bytes.NewReader(raw)
	output, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 {
		t.Fatalf("crash %s: %v %s", boundary, err, output)
	}
	id, err := ReadIdentity(a.dir)
	if err != nil {
		t.Fatal(err)
	}
	if id.NativeLaunchIntent == nil {
		t.Fatal("missing launch intent")
	}
	var spawned *ProcessIdentity
	data, err := os.ReadFile(filepath.Join(a.dir, "spawned.json"))
	if err == nil {
		spawned = &ProcessIdentity{}
		if err := json.Unmarshal(data, spawned); err != nil {
			t.Fatal(err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if spawned != nil {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(spawned.PID)) //nolint:gosec // G115: fixture PID.
		if err == nil {
			var created, exited, kernel, user windows.Filetime
			if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
				_ = windows.CloseHandle(h)
				t.Fatal(err)
			}
			if !time.Unix(0, created.Nanoseconds()).Equal(spawned.StartTime) {
				_ = windows.CloseHandle(h)
				t.Fatal("spawned PID reused before cleanup handle was pinned")
			}
			t.Cleanup(func() {
				_ = windows.TerminateProcess(h, 1)
				_, _ = windows.WaitForSingleObject(h, 5000)
				_ = windows.CloseHandle(h)
			})
		} else if !errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			t.Fatal(err)
		}
	}
	return a, id, spawned
}

func TestSuspendedNativeIsRecordedBeforeAssign(t *testing.T) {
	t.Parallel()
	a, id, spawned := crashBoundary(t, "afterStart")
	if spawned == nil || id.NativePID == nil || id.NativeStartTime == nil || *id.NativePID != spawned.PID || !id.NativeStartTime.Equal(spawned.StartTime) {
		t.Fatalf("pre-assignment identity=%+v spawned=%+v", id, spawned)
	}
	start, err := ProcessStartTime(spawned.PID)
	if err != nil || !start.Equal(spawned.StartTime) {
		t.Fatalf("suspended native identity: %v %s", err, start)
	}
	assertNativeThreadSuspended(t, spawned.PID)
	assertNoTreeReport(t, a)
}

func assertNoTreeReport(t *testing.T, a attempt) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(a.workdir, "tree.jsonl"))
	if !errors.Is(err, os.ErrNotExist) || len(raw) != 0 {
		t.Fatalf("suspended native executed: report=%q err=%v", raw, err)
	}
}

func TestLaunchBoundaryCrashRecords(t *testing.T) {
	t.Parallel()
	var intentState, recordedState map[string]any
	for _, boundary := range []string{"afterIntent", "afterJob", "afterSpawn", "afterStart", "beforeResume", "afterResume"} {
		t.Run(boundary, func(t *testing.T) {
			a, id, spawned := crashBoundary(t, boundary)
			recorded := boundary == "afterStart" || boundary == "beforeResume" || boundary == "afterResume"
			if recorded != (id.NativePID != nil && id.NativeStartTime != nil) {
				t.Fatalf("boundary %s identity=%+v", boundary, id)
			}
			if spawned == nil {
				if boundary != "afterIntent" && boundary != "afterJob" {
					t.Fatal("spawn marker missing")
				}
			} else {
				live := boundary == "afterSpawn" || boundary == "afterStart"
				start, err := ProcessStartTime(spawned.PID)
				if live {
					assertNativeThreadSuspended(t, spawned.PID)
					if err != nil || !start.Equal(spawned.StartTime) {
						t.Fatalf("suspended residual missing: %s %v", start, err)
					}
				} else if !errors.Is(err, ErrNoProcess) {
					t.Fatalf("job native still alive after crash: %s %v", start, err)
				}
				if recorded && (*id.NativePID != spawned.PID || !id.NativeStartTime.Equal(spawned.StartTime)) {
					t.Fatal("record differs from actual spawned identity")
				}
			}
			if boundary != "afterResume" {
				assertNoTreeReport(t, a)
			} else {
				data, err := os.ReadFile(filepath.Join(a.workdir, "tree.jsonl"))
				if err != nil {
					t.Fatal(err)
				}
				dec := json.NewDecoder(bytes.NewReader(data))
				count := 0
				for {
					var row struct {
						Role      string    `json:"role"`
						PID       int       `json:"pid"`
						StartTime time.Time `json:"start_time"`
					}
					err := dec.Decode(&row)
					if errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					if row.Role == "child" || row.Role == "grandchild" {
						count++
						if start, err := ProcessStartTime(row.PID); !errors.Is(err, ErrNoProcess) {
							t.Fatalf("%s survived crash: %s %v", row.Role, start, err)
						}
					}
				}
				if count != 2 {
					t.Fatalf("descendants=%d", count)
				}
			}
			// Each actual PID/time pair must equal its side-channel observation.
			// Compare all remaining persisted fields across boundaries after replacing
			// only per-invocation identities and clocks with stable placeholders.
			raw, _ := json.Marshal(id)
			var state map[string]any
			if err := json.Unmarshal(raw, &state); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"run_id", "attempt_id", "pid", "start_time", "launch_token", "native_launch_intent"} {
				delete(state, key)
			}
			if recorded {
				state["native_pid"] = "observed-pid"
				state["native_start_time"] = "observed-start"
				if recordedState == nil {
					recordedState = state
				} else if !reflect.DeepEqual(state, recordedState) {
					t.Fatalf("recorded boundary changed: %v != %v", state, recordedState)
				}
			} else {
				if intentState == nil {
					intentState = state
				} else if !reflect.DeepEqual(state, intentState) {
					t.Fatalf("intent boundary changed: %v != %v", state, intentState)
				}
			}
		})
	}
}
