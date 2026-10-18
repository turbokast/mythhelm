//go:build windows

package workers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
)

func TestWorkerClimbsPinnedLadder(t *testing.T) {
	// Windows has one qualified rung: terminate the owned Job Object.
	ladder := []adapter.StopStep{{Signal: adapter.StopKill, Grace: 5 * time.Second}}
	p := climbPinnedLadder(t, "slow", ladder)
	if !p.Confirmed || !slices.Equal(p.Sent, ladder) || p.LadderVersion != "stop-ladder/v1" {
		t.Errorf("attempt.stopped = %+v, want confirmed with sent %v under stop-ladder/v1", p, ladder)
	}
	if p.DescendantScan != "job-object" {
		t.Errorf("attempt.stopped descendant_scan = %q, want job-object", p.DescendantScan)
	}
}

// waitTree captures live identities before the stop. Handles pin those exact
// processes so exit assertions cannot mistake PID reuse for termination.
func waitTree(t *testing.T, a attempt) (map[string]ProcessIdentity, map[string]windows.Handle, error) {
	t.Helper()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline.C:
			return nil, nil, errors.New("tree report deadline")
		case <-tick.C:
			data, err := os.ReadFile(filepath.Join(a.workdir, "tree.jsonl"))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, nil, err
			}
			dec := json.NewDecoder(bytes.NewReader(data))
			rows := map[string]ProcessIdentity{}
			complete := false
			for {
				var row struct {
					Role      string    `json:"role"`
					PID       int       `json:"pid"`
					StartTime time.Time `json:"start_time"`
					Error     *uint32   `json:"error"`
				}
				err := dec.Decode(&row)
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					break
				}
				if err != nil {
					return nil, nil, err
				}
				if row.Role == "breakaway" {
					if row.Error == nil || *row.Error != 5 || row.PID != 0 {
						return nil, nil, fmt.Errorf("breakaway not denied: %+v", row)
					}
					complete = true
				} else {
					rows[row.Role] = ProcessIdentity{PID: row.PID, StartTime: row.StartTime}
				}
			}
			if !complete || len(rows) != 2 {
				continue
			}
			handles := map[string]windows.Handle{}
			for role, id := range rows {
				h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(id.PID)) //nolint:gosec // G115: fixture PIDs originate from Windows uint32 identifiers.
				if err != nil {
					return nil, nil, err
				}
				t.Cleanup(func() {
					_ = windows.TerminateProcess(h, 1)
					_, _ = windows.WaitForSingleObject(h, 5000)
					_ = windows.CloseHandle(h)
				})
				var created, exited, kernel, user windows.Filetime
				if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
					return nil, nil, err
				}
				var code uint32
				if err := windows.GetExitCodeProcess(h, &code); err != nil {
					return nil, nil, err
				}
				if code != 259 || !time.Unix(0, created.Nanoseconds()).Equal(id.StartTime) {
					return nil, nil, fmt.Errorf("%s not live at reported identity", role)
				}
				handles[role] = h
			}
			return rows, handles, nil
		}
	}
}

func TestStopKillsChildAndGrandchild(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	w := &worker{}
	var handles map[string]windows.Handle
	w.jobOps.afterResume = func() error {
		var err error
		_, handles, err = waitTree(t, a)
		if err != nil {
			return err
		}
		return RequestStop(a.dir, "test-stop")
	}
	receiptObserved := false
	w.spoolSync = func(f *os.File) error {
		data, err := os.ReadFile(f.Name())
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte(`"type":"attempt.stopped"`)) && !receiptObserved {
			receiptObserved = true
			if len(handles) != 2 {
				t.Error("stopped receipt without observed tree")
			}
			for role, h := range handles {
				var code uint32
				if err := windows.GetExitCodeProcess(h, &code); err != nil || code == 259 {
					t.Errorf("%s alive at stopped persistence: exit=%d err=%v", role, code, err)
				}
			}
		}
		return f.Sync()
	}
	l := a.launch(t, "tree")
	l.LadderVersion = "stop-ladder/v2-windows"
	if err := a.runInProcess(t, l, w); err != nil {
		t.Fatal(err)
	}
	p := stoppedOf(t, a.events(t))
	// I06/I09 (v2 §6.4): tree evidence, not native exit alone, confirms stop.
	if !p.Confirmed || p.DescendantScan != "job-object" || p.LadderVersion != "stop-ladder/v2-windows" {
		t.Fatalf("stopped=%+v", p)
	}
	if !receiptObserved {
		t.Fatal("stopped persistence was not observed")
	}
	if len(handles) != 2 {
		t.Fatal("tree was not observed")
	}
	for role, h := range handles {
		var code uint32
		if err := windows.GetExitCodeProcess(h, &code); err != nil || code == 259 {
			t.Errorf("%s exit=%d error=%v; descendant survived", role, code, err)
		}
	}
}

func TestStopTimeoutEndsStopUnconfirmed(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	w := &worker{}
	jobReady := make(chan *jobObject, 1)
	w.jobOps.afterJob = func(j *jobObject) error { jobReady <- j; return nil }
	stopReady := make(chan struct{}, 1)
	calls := 0
	w.jobOps.terminate = func(*jobObject) error { calls++; return nil }
	w.jobOps.afterResume = func() error {
		if err := RequestStop(a.dir, "test-stop"); err != nil {
			return err
		}
		stopReady <- struct{}{}
		return nil
	}
	l := a.launch(t, "slow")
	// Run with the real pinned 5s grace. If the bounded wait is absent, release
	// this test's job at the deadline and join the worker before failing.
	finished := make(chan error, 1)
	go func() { finished <- a.runInProcess(t, l, w) }()
	select {
	case <-stopReady:
	case err := <-finished:
		t.Fatalf("stop not requested: %v", err)
	}
	timer := time.NewTimer(12 * time.Second)
	defer timer.Stop()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-timer.C:
		job := <-jobReady
		_ = job.terminate()
		<-finished
		t.Fatal("worker exceeded ladder grace plus bounded wait; waited on live native")
	}
	if calls != 1 {
		t.Fatalf("termination calls=%d, want one ladder", calls)
	}
	evs := a.events(t)
	p := stoppedOf(t, evs)
	if state, reason := final(t, evs); state != "interrupted" || reason != "stop_unconfirmed" || p.Confirmed {
		t.Fatalf("final=%s/%s stopped=%+v", state, reason, p)
	}
	if len(p.UnresolvedPIDs) == 0 {
		t.Fatalf("live native omitted from survivors: %+v", p)
	}
}

func TestEnumerationFailureIsFailedScan(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	w := &worker{}
	w.jobOps.members = func(*jobObject) ([]ProcessIdentity, error) { return nil, windows.ERROR_ACCESS_DENIED }
	w.jobOps.afterResume = func() error { return RequestStop(a.dir, "test-stop") }
	l := a.launch(t, "slow")
	l.StopLadder = []adapter.StopStep{{Signal: adapter.StopKill, Grace: 100 * time.Millisecond}}
	if err := a.runInProcess(t, l, w); err != nil {
		t.Fatal(err)
	}
	p := stoppedOf(t, a.events(t))
	if state, reason := final(t, a.events(t)); state != "interrupted" || reason != "stop_unconfirmed" || p.Confirmed || p.DescendantScan != "failed" {
		t.Fatalf("final=%s/%s stopped=%+v", state, reason, p)
	}
	if w.proc.GroupGone() {
		t.Fatal("failed enumeration reported job gone")
	}
}

func TestStopReportsSurvivorsWithIdentity(t *testing.T) {
	t.Parallel()
	t.Run("native survives", TestStopTimeoutEndsStopUnconfirmed)
	t.Run("native gone grandchild survives", func(t *testing.T) {
		t.Parallel()
		a := newAttempt(t)
		w := &worker{}
		var rows map[string]ProcessIdentity
		w.jobOps.afterResume = func() error {
			var err error
			rows, _, err = waitTree(t, a)
			if err != nil {
				return err
			}
			return RequestStop(a.dir, "test-stop")
		}
		// Deliberately break tree termination: stop only the native, leaving real
		// descendants in its owned job until after the receipt is persisted.
		w.jobOps.terminate = func(*jobObject) error { return w.proc.cmd.Process.Kill() }
		l := a.launch(t, "tree")
		l.StopLadder = []adapter.StopStep{{Signal: adapter.StopKill, Grace: 100 * time.Millisecond}}
		if err := a.runInProcess(t, l, w); err != nil {
			t.Fatal(err)
		}
		evs := a.events(t)
		p := stoppedOf(t, evs)
		if state, reason := final(t, evs); state != "interrupted" || reason != "stop_unconfirmed" || p.Confirmed {
			t.Fatalf("final=%s/%s stopped=%+v", state, reason, p)
		}
		var receipt struct {
			Identities []ProcessIdentity `json:"unresolved_identities"`
		}
		if err := json.Unmarshal(find(t, evs, "attempt.stopped").Payload, &receipt); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(receipt.Identities, rows["grandchild"]) {
			t.Fatalf("identities=%+v, missing grandchild %+v", receipt.Identities, rows["grandchild"])
		}
		if slices.Contains(p.UnresolvedPIDs, *w.id.NativePID) {
			t.Fatal("reaped native reported as survivor")
		}
	})
}

func TestCreateNewProcessGroupDescendantDies(t *testing.T) { TestStopKillsChildAndGrandchild(t) }

func TestWindowsLadderHasOnlyKill(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	l := a.launch(t, "slow")
	if !slices.Equal(l.StopLadder, []adapter.StopStep{{Signal: adapter.StopKill, Grace: 5 * time.Second}}) {
		t.Fatalf("ladder=%v", l.StopLadder)
	}
}

func TestSignalRejectsNonKillOnWindows(t *testing.T) {
	t.Parallel()
	for _, sig := range []adapter.StopSignal{adapter.StopInterrupt, adapter.StopTerminate} {
		if err := (&ownedProc{}).Signal(sig); err == nil {
			t.Fatalf("signal %s accepted", sig)
		}
	}
}

func TestLateJobScanInvalidatesConfirmation(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"survivor", "error"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			a := newAttempt(t)
			w := &worker{}
			// The ladder observes an empty job, but the final evidence query changes.
			queries := 0
			identity := ProcessIdentity{PID: 1234, StartTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
			w.jobOps.members = func(j *jobObject) ([]ProcessIdentity, error) {
				if !w.proc.waited.Load() {
					return j.members()
				}
				queries++
				if queries == 1 {
					return nil, nil
				}
				if mode == "error" {
					return nil, windows.ERROR_ACCESS_DENIED
				}
				return []ProcessIdentity{identity}, nil
			}
			if err := a.runInProcess(t, a.launch(t, "happy"), w); err != nil {
				t.Fatal(err)
			}
			evs := a.events(t)
			p := stoppedOf(t, evs)
			if state, reason := final(t, evs); state != "interrupted" || reason != "stop_unconfirmed" || p.Confirmed {
				t.Fatalf("final=%s/%s stopped=%+v", state, reason, p)
			}
			if mode == "survivor" {
				var receipt struct {
					Identities []ProcessIdentity `json:"unresolved_identities"`
				}
				if err := json.Unmarshal(find(t, evs, "attempt.stopped").Payload, &receipt); err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(receipt.Identities, []ProcessIdentity{identity}) {
					t.Fatalf("same-scan identities lost: %+v", receipt.Identities)
				}
			} else if p.DescendantScan != "failed" {
				t.Fatalf("scan=%q", p.DescendantScan)
			}
		})
	}
}

func TestPanicBeforeJobAssignmentTerminatesSuspendedNative(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	w := &worker{}
	nativeReady := make(chan windows.Handle, 1)
	w.jobOps.afterSpawn = func(pid int) error {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(pid)) //nolint:gosec // G115: native PID originates from Windows.
		if err != nil {
			return err
		}
		nativeReady <- h
		panic("synthetic pre-assignment panic")
	}
	finished := make(chan error, 1)
	l := a.launch(t, "slow")
	go func() { finished <- a.runInProcess(t, l, w) }()
	var h windows.Handle
	select {
	case h = <-nativeReady:
	case err := <-finished:
		t.Fatalf("native not created: %v", err)
	}
	defer func() { _ = windows.TerminateProcess(h, 1); _ = windows.CloseHandle(h) }()
	timer := time.NewTimer(7 * time.Second)
	defer timer.Stop()
	select {
	case <-finished:
	case <-timer.C:
		var code uint32
		err := windows.GetExitCodeProcess(h, &code)
		_ = windows.TerminateProcess(h, 1)
		<-finished
		t.Fatalf("pre-assignment panic left native alive: exit=%d error=%v", code, err)
	}
	status, err := windows.WaitForSingleObject(h, 0)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("native not stopped: status=%d error=%v", status, err)
	}
	evs := a.events(t)
	p := stoppedOf(t, evs)
	if state, reason := final(t, evs); state != "interrupted" || reason != "worker_panic" || !p.Confirmed {
		t.Fatalf("panic result=%s/%s stopped=%+v", state, reason, p)
	}
}

func TestPanicAfterAssignmentStopsTree(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	w := &worker{}
	var handles map[string]windows.Handle
	w.jobOps.afterResume = func() error {
		var err error
		_, handles, err = waitTree(t, a)
		if err != nil {
			return err
		}
		panic("synthetic assigned-job panic")
	}
	if err := a.runInProcess(t, a.launch(t, "tree"), w); err == nil {
		t.Fatal("panic was not returned")
	}
	evs := a.events(t)
	p := stoppedOf(t, evs)
	if state, reason := final(t, evs); state != "interrupted" || reason != "worker_panic" || !p.Confirmed || len(handles) != 2 {
		t.Fatalf("panic result=%s/%s stopped=%+v tree=%d", state, reason, p, len(handles))
	}
	for role, h := range handles {
		var code uint32
		if err := windows.GetExitCodeProcess(h, &code); err != nil || code == 259 {
			t.Fatalf("%s survived panic: exit=%d err=%v", role, code, err)
		}
	}
}

func TestPanicCleanupTimeoutRemainsUnconfirmed(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	w := &worker{}
	nativeReady := make(chan windows.Handle, 1)
	w.jobOps.terminate = func(*jobObject) error { return windows.ERROR_ACCESS_DENIED }
	w.jobOps.terminateNative = func(*os.Process) error { return windows.ERROR_ACCESS_DENIED }
	w.jobOps.afterSpawn = func(pid int) error {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(pid)) //nolint:gosec // G115: native PID originates from Windows.
		if err != nil {
			return err
		}
		nativeReady <- h
		panic("synthetic failed cleanup")
	}
	l := a.launch(t, "slow")
	finished := make(chan error, 1)
	go func() { finished <- a.runInProcess(t, l, w) }()
	var h windows.Handle
	select {
	case h = <-nativeReady:
	case err := <-finished:
		t.Fatalf("native not created: %v", err)
	}
	defer func() {
		_ = windows.TerminateProcess(h, 1)
		_, _ = windows.WaitForSingleObject(h, 5000)
		_ = windows.CloseHandle(h)
	}()
	timer := time.NewTimer(7 * time.Second)
	defer timer.Stop()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("panic not returned")
		}
	case <-timer.C:
		_ = windows.TerminateProcess(h, 1)
		<-finished
		t.Fatal("panic cleanup exceeded bounded wait")
	}
	p := stoppedOf(t, a.events(t))
	state, reason := final(t, a.events(t))
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil || code != 259 {
		t.Fatalf("failure injection did not retain suspended native: code=%d err=%v", code, err)
	}
	if p.Confirmed || state != "interrupted" || reason != "worker_panic" {
		t.Fatalf("panic result=%s/%s stopped=%+v", state, reason, p)
	}
}

// Natural native exit does not imply the first job snapshot is empty. The
// cleanup receipt records the ladder actually taken, never a fixed signal list.
func TestNaturalExitJobSnapshotDeterminesSentSignals(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"empty", "member", "error"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			a := newAttempt(t)
			w := &worker{}
			queries, kills := 0, 0
			w.jobOps.members = func(*jobObject) ([]ProcessIdentity, error) {
				if !w.proc.waited.Load() {
					t.Error("cleanup query preceded native Wait")
				}
				queries++
				if queries == 1 {
					switch mode {
					case "member":
						return []ProcessIdentity{{PID: 1234, StartTime: time.Unix(1, 0)}}, nil
					case "error":
						return nil, windows.ERROR_ACCESS_DENIED
					}
				}
				return nil, nil
			}
			w.jobOps.terminate = func(j *jobObject) error { kills++; return j.terminate() }
			launch := a.launch(t, "happy")
			launch.StopLadder = []adapter.StopStep{{Signal: adapter.StopKill, Grace: 5 * time.Second}}
			launch.LadderVersion = "stop-ladder/v2-windows"
			if err := a.runInProcess(t, launch, w); err != nil {
				t.Fatal(err)
			}
			evs := a.events(t)
			stopped := stoppedOf(t, evs)
			wantKills := 1
			if mode == "empty" {
				wantKills = 0
			}
			if kills != wantKills || len(stopped.Sent) != wantKills || !stopped.Confirmed || stopped.DescendantScan != "job-object" {
				t.Fatalf("queries=%d kills=%d stopped=%+v", queries, kills, stopped)
			}
			if wantKills == 1 && !slices.Equal(stopped.Sent, launch.StopLadder) {
				t.Fatalf("sent=%v", stopped.Sent)
			}
			if state, reason := final(t, evs); state != "succeeded_native" {
				t.Fatalf("final=%s/%s", state, reason)
			}
		})
	}
}
