//go:build windows

package workers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"

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
	checkStopTimeoutEndsStopUnconfirmed(t, false, false)
}

func TestStopTimeoutExcludesLaunchPersistence(t *testing.T) {
	t.Parallel()
	// Keep the counterexample: launch persistence can consume the old entire
	// watchdog before the stop ladder even starts. The supervision budget stays
	// 12s; this deliberate barrier belongs to setup, outside that budget.
	checkStopTimeoutEndsStopUnconfirmed(t, true, false)
}

func TestStopTimeoutExcludesObserverLatency(t *testing.T) {
	t.Parallel()
	checkStopTimeoutEndsStopUnconfirmed(t, false, true)
}

// handoffStopTestNative keeps ownership until the receiver accepts the handle.
// Teardown can win before afterSpawn arrives: the producer then kills, waits
// and closes its own handle instead of publishing it after cleanup has left.
func handoffStopTestNative(pid int, ready chan<- windows.Handle, stopping <-chan struct{}) error {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid)) //nolint:gosec // G115: Windows PID.
	if err != nil {
		return err
	}
	transferred := false
	defer func() {
		if !transferred {
			_ = windows.TerminateProcess(h, 1)
			_, _ = windows.WaitForSingleObject(h, 5000)
			_ = windows.CloseHandle(h)
		}
	}()
	cancelled := errors.New("test native handoff cancelled during setup")
	select {
	case <-stopping:
		return cancelled
	default:
	}
	select {
	case ready <- h:
		transferred = true
		return nil
	case <-stopping:
		return cancelled
	}
}

func checkStopTimeoutEndsStopUnconfirmed(t *testing.T, delayLaunchPersistence, delayObserver bool) {
	t.Helper()
	const watchdog = 12 * time.Second
	a := newAttempt(t)
	w := &worker{}
	var diagnosticMu sync.Mutex
	phase, lastEvent := "worker startup", "none"
	setPhase := func(next string) {
		diagnosticMu.Lock()
		phase = next
		diagnosticMu.Unlock()
	}
	dumpFailure := func() {
		diagnosticMu.Lock()
		currentPhase, currentEvent := phase, lastEvent
		diagnosticMu.Unlock()
		// Fixed storage bounds failure output; capture before cleanup changes
		// the blocked call. No spool reads or waits belong in this diagnostic.
		stack := make([]byte, 128<<10)
		n := runtime.Stack(stack, true)
		t.Logf("worker diagnostic: phase=%s last-event=%s stack-bytes=%d capacity=%d\n%s", currentPhase, currentEvent, n, len(stack), stack[:n])
	}
	stopReady := make(chan time.Time, 1)
	firstSignal := make(chan time.Time, 1)
	launchBlocked := make(chan time.Time, 1)
	supervisionReached := make(chan time.Time, 1)
	releaseLaunch := make(chan struct{})
	releaseResult := make(chan struct{})
	unblockLaunch := sync.OnceFunc(func() { close(releaseLaunch) })
	unblockResult := sync.OnceFunc(func() { close(releaseResult) })
	nativeReady := make(chan windows.Handle)
	stopping := make(chan struct{})
	w.jobOps.afterSpawn = func(pid int) error {
		setPhase("native handle handoff")
		err := handoffStopTestNative(pid, nativeReady, stopping)
		setPhase("native handle handoff returned")
		return err
	}
	calls := 0
	w.jobOps.terminate = func(*jobObject) error {
		calls++
		if calls == 1 {
			firstSignal <- time.Now()
		}
		return nil
	}
	w.jobOps.afterResume = func() error {
		setPhase("stop request persistence")
		if err := RequestStop(a.dir, "test-stop"); err != nil {
			return err
		}
		stopReady <- time.Now()
		setPhase("stop request persisted")
		return nil
	}
	var observerMu sync.Mutex
	var observerEntry time.Time
	observerActive := false
	launchPaused, resultObserved := false, false
	w.spoolSync = func(f *os.File) error {
		enteredAt := time.Now() // Before ReadFile or any observer classification work.
		observerMu.Lock()
		observerEntry, observerActive = enteredAt, true
		observerMu.Unlock()
		defer func() { observerMu.Lock(); observerActive = false; observerMu.Unlock() }()
		setPhase("spool observer read/classification")
		body, err := os.ReadFile(f.Name())
		if err != nil {
			return err
		}
		line := bytes.TrimSpace(body)
		line = line[bytes.LastIndexByte(line, '\n')+1:]
		var event struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(line, &event) == nil {
			diagnosticMu.Lock()
			lastEvent = event.Type
			diagnosticMu.Unlock()
		}
		if delayLaunchPersistence && !launchPaused && bytes.Contains(body, []byte(`"type":"attempt.launched"`)) {
			launchPaused = true
			setPhase("launch persistence barrier")
			launchBlocked <- time.Now()
			<-releaseLaunch
		}
		if !resultObserved && bytes.Contains(body, []byte(`"type":"attempt.native_result"`)) {
			resultObserved = true
			// supervise returned: observe the append before this sync and the later
			// receipt/state persistence. Keep the native live until the test observes it.
			if delayObserver {
				setPhase("native result observer delay")
				delay := time.NewTimer(watchdog)
				select {
				case <-delay.C: // Deliberately slow classification, after recording callback entry.
				case <-stopping:
					delay.Stop()
				}
			}
			setPhase("native result observation barrier")
			supervisionReached <- enteredAt
			<-releaseResult
		}
		setPhase("spool f.Sync")
		err = f.Sync()
		setPhase("spool f.Sync returned; worker outside observer")
		return err
	}
	l := a.launch(t, "slow")
	if !slices.Equal(l.StopLadder, []adapter.StopStep{{Signal: adapter.StopKill, Grace: 5 * time.Second}}) {
		t.Fatalf("ladder=%+v, want real pinned 5s kill rung", l.StopLadder)
	}
	finished := make(chan struct{})
	var runErr error
	started := time.Now()
	go func() {
		runErr = a.runInProcess(t, l, w)
		setPhase("worker returned")
		close(finished)
	}()
	var native windows.Handle
	t.Cleanup(func() {
		close(stopping)
		unblockLaunch()
		unblockResult()
		// Cleanup owns a retained native handle, never a concurrently closed job
		// handle or a PID lookup. A missing supervision timer must not hang the test.
		if native != 0 {
			if status, _ := windows.WaitForSingleObject(native, 0); status == uint32(windows.WAIT_TIMEOUT) {
				_ = windows.TerminateProcess(native, 1)
			}
		}
		joined := time.NewTimer(watchdog)
		defer joined.Stop()
		select {
		case <-finished:
		case <-joined.C:
			dumpFailure()
			t.Error("cleanup: worker join exceeded 12s")
		}
		if native != 0 {
			if status, err := windows.WaitForSingleObject(native, 5000); err != nil || status != windows.WAIT_OBJECT_0 {
				t.Errorf("cleanup: native wait=%d err=%v", status, err)
			}
			_ = windows.CloseHandle(native)
		}
	})
	t.Cleanup(func() {
		if t.Failed() {
			dumpFailure()
		}
	})
	awaitPhase := func(ch <-chan time.Time, phase string) time.Time {
		timer := time.NewTimer(watchdog)
		defer timer.Stop()
		select {
		case at := <-ch:
			return at
		case <-finished:
			t.Fatalf("%s: worker returned early: %v", phase, runErr)
		case <-timer.C:
			t.Fatalf("%s: phase did not begin within 12s (setup elapsed %s)", phase, time.Since(started))
		}
		return time.Time{}
	}
	handoff := time.NewTimer(watchdog)
	defer handoff.Stop()
	select {
	case native = <-nativeReady:
	case <-finished:
		t.Fatalf("native setup returned before handoff: %v", runErr)
	case <-handoff.C:
		t.Fatal("native setup handoff did not arrive within 12s")
	}
	requestedAt := awaitPhase(stopReady, "stop request")
	if delayLaunchPersistence {
		blockedAt := awaitPhase(launchBlocked, "launch persistence barrier")
		delay := time.NewTimer(watchdog)
		<-delay.C
		unblockLaunch()
		t.Logf("setup: launch persistence deliberately blocked for %s", time.Since(blockedAt))
	}
	signalAt := awaitPhase(firstSignal, "first ladder signal")
	t.Logf("setup=%s request-to-first-signal=%s", signalAt.Sub(started), signalAt.Sub(requestedAt))
	timer := time.NewTimer(time.Until(signalAt.Add(watchdog)))
	defer timer.Stop()
	var supervisedAt time.Time
	select {
	case supervisedAt = <-supervisionReached:
	case <-timer.C:
		// The test goroutine may itself run late. Judge the worker's recorded
		// boundary time, rather than which ready channel select happens to choose.
		select {
		case supervisedAt = <-supervisionReached:
		default:
			observerMu.Lock()
			enteredAt, active := observerEntry, observerActive
			observerMu.Unlock()
			if !active || enteredAt.After(signalAt.Add(watchdog)) {
				t.Fatalf("supervision: no callback entered within 12s of first kill (elapsed %s)", time.Since(signalAt))
			}
			// Classification is test-side I/O. Allow it a separate bounded wait,
			// then judge the recorded entry time against the original core deadline.
			classify := time.NewTimer(watchdog)
			defer classify.Stop()
			select {
			case supervisedAt = <-supervisionReached:
			case <-finished:
				t.Fatalf("observer: worker returned before classification: %v", runErr)
			case <-classify.C:
				t.Fatal("observer: callback classification exceeded separate 12s budget")
			}
		}
	case <-finished:
		t.Fatalf("supervision: worker returned before boundary observation: %v", runErr)
	}
	if elapsed := supervisedAt.Sub(signalAt); elapsed > watchdog {
		t.Fatalf("supervision: first kill to native_result append=%s, exceeds unchanged 12s budget", elapsed)
	}
	if status, err := windows.WaitForSingleObject(native, 0); err != nil || status != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("supervision boundary did not precede native exit: wait=%d err=%v", status, err)
	}
	t.Logf("supervision=%s observer-notification-delay=%s; native remains live before receipt persistence", supervisedAt.Sub(signalAt), time.Since(supervisedAt))
	persistenceAt := time.Now()
	unblockResult()
	join := time.NewTimer(watchdog)
	defer join.Stop()
	select {
	case <-finished:
		if runErr != nil {
			t.Fatal(runErr)
		}
	case <-join.C:
		t.Fatal("persistence: worker did not finish within separate 12s join budget")
	}
	t.Logf("post-supervision persistence/join=%s", time.Since(persistenceAt))
	if calls != 1 {
		t.Fatalf("termination calls=%d, want one ladder", calls)
	}
	evs := a.events(t)
	p := stoppedOf(t, evs)
	if state, reason := final(t, evs); state != "interrupted" || reason != "stop_unconfirmed" || p.Confirmed {
		t.Fatalf("final=%s/%s stopped=%+v", state, reason, p)
	}
	id, err := ReadIdentity(a.dir)
	if err != nil {
		t.Fatal(err)
	}
	if id.NativePID == nil || id.NativeStartTime == nil || !slices.Contains(p.UnresolvedPIDs, *id.NativePID) {
		t.Fatalf("live native identity omitted from survivors: identity=%+v stopped=%+v", id, p)
	}
	var receipt struct {
		Identities []ProcessIdentity `json:"unresolved_identities"`
	}
	if err := json.Unmarshal(find(t, evs, "attempt.stopped").Payload, &receipt); err != nil {
		t.Fatal(err)
	}
	expected := ProcessIdentity{PID: *id.NativePID, StartTime: *id.NativeStartTime}
	if !slices.Contains(receipt.Identities, expected) {
		t.Fatalf("survivor identities=%+v, want native %+v", receipt.Identities, expected)
	}
}

func TestStopTimeoutCleanupOwnsLateNativeHandoff(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	w := &worker{}
	// afterSpawn fails before job assignment. Production launch cleanup must
	// not hide a missing termination in the handoff helper's deferred cleanup.
	w.jobOps.terminate = func(*jobObject) error { return nil }
	w.jobOps.terminateNative = func(*os.Process) error { return nil }
	ready := make(chan windows.Handle)
	stopping, release := make(chan struct{}), make(chan struct{})
	cancel := sync.OnceFunc(func() { close(stopping) })
	unblock := sync.OnceFunc(func() { close(release) })
	probeReady := make(chan windows.Handle)
	w.jobOps.afterSpawn = func(pid int) error {
		probe, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid)) //nolint:gosec // G115: Windows PID.
		if err != nil {
			return err
		}
		select {
		case probeReady <- probe:
		case <-stopping:
			_ = windows.TerminateProcess(probe, 1)
			_, _ = windows.WaitForSingleObject(probe, 5000)
			_ = windows.CloseHandle(probe)
			return errors.New("setup cancelled before probe ownership transfer")
		}
		<-release // Actual native exists, but its ownership handoff has not arrived.
		return handoffStopTestNative(pid, ready, stopping)
	}
	done := make(chan struct{})
	var runErr error
	l := a.launch(t, "slow")
	go func() { runErr = a.runInProcess(t, l, w); close(done) }()
	var probe windows.Handle
	t.Cleanup(func() {
		cancel()
		unblock()
		cleanup := time.NewTimer(12 * time.Second)
		defer cleanup.Stop()
		for {
			select {
			case late := <-ready:
				_ = windows.TerminateProcess(late, 1)
				_, _ = windows.WaitForSingleObject(late, 5000)
				_ = windows.CloseHandle(late)
			case <-done:
				if probe != 0 {
					_ = windows.TerminateProcess(probe, 1)
					_ = windows.CloseHandle(probe)
				}
				return
			case <-cleanup.C:
				if probe != 0 {
					_ = windows.TerminateProcess(probe, 1)
					_ = windows.CloseHandle(probe)
				}
				t.Error("late-handoff cleanup did not join worker within 12s")
				return
			}
		}
	})
	setup := time.NewTimer(12 * time.Second)
	defer setup.Stop()
	select {
	case probe = <-probeReady:
	case <-setup.C:
		t.Fatal("native was not spawned")
	}
	if status, err := windows.WaitForSingleObject(probe, 0); err != nil || status != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("late native not live before cancelled setup: wait=%d err=%v", status, err)
	}
	cancel() // Simulate setup failure before afterSpawn can publish ownership.
	unblock()
	joined := time.NewTimer(12 * time.Second)
	defer joined.Stop()
	select {
	case <-done:
	case <-joined.C:
		t.Fatal("late publisher did not observe teardown and join")
	}
	if runErr != nil {
		t.Fatal(runErr)
	}
	if status, err := windows.WaitForSingleObject(probe, 0); err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("late native survived cancelled setup: wait=%d err=%v", status, err)
	}
	if state, reason := final(t, a.events(t)); state != "failed_native" || reason != "launch_failed" {
		t.Fatalf("late cancelled setup=%s/%s", state, reason)
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
