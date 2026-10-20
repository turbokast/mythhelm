//go:build windows

package supervisor_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/turbokast/mythhelm/adapters/fake"
	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
)

func suspendedRecoveryNative(t *testing.T, dir string) (*exec.Cmd, workers.ProcessIdentity, windows.Handle) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), os.Args[0], fake.AgentCommand, "--scenario", "tree", "--workdir", dir) //nolint:gosec // G702: own test executable with fixed fixture selector and test-owned arguments.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	start, err := workers.ProcessStartTime(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid)) //nolint:gosec // G115: spawned Windows PID.
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(h) })
	// cmd.Wait is owned by cleanup for live cases and by the dead-case caller.
	// os/exec permits a repeated Wait to return an error; no new process is made.
	return cmd, workers.ProcessIdentity{PID: cmd.Process.Pid, StartTime: start}, h
}

func persistRecoveryIdentity(t *testing.T, f fixture, d admission.Decision, id workers.Identity) {
	t.Helper()
	raw, err := json.Marshal(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workers.AttemptDir(f.state, d.RunID, d.AttemptID), "worker.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// Ingest preserves worker event identity and payload; only RunSequence is
// assigned by Append (ingest.go decodeSpoolLine). Compare every other field.
func recordRecoveryLaunch(t *testing.T, into map[string]journal.Event, event journal.Event) {
	t.Helper()
	if event.Type != "attempt.launched" {
		return
	}
	event.RunSequence = 0
	if prior, ok := into[event.EventID]; ok && !reflect.DeepEqual(prior, event) {
		t.Fatal("preexisting launch identity disagrees between journal and spool")
	}
	into[event.EventID] = event
}

func recordRecoverySpoolLaunches(t *testing.T, into map[string]journal.Event, spool []byte) {
	t.Helper()
	if end := bytes.LastIndexByte(spool, '\n'); end >= 0 {
		for line := range bytes.SplitSeq(spool[:end+1], []byte("\n")) {
			if len(line) == 0 {
				continue
			}
			var event journal.Event
			if err := json.Unmarshal(line, &event); err != nil {
				t.Fatal(err)
			}
			recordRecoveryLaunch(t, into, event)
		}
	}
}

func recoverUnresolvedWindows(t *testing.T, f fixture, d admission.Decision, wantPIDs []int) {
	t.Helper()
	j := f.journal(t)
	before, err := j.LatestAttempt(t.Context(), d.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var beforeCount int
	if err := rawDB(t, f.state).QueryRowContext(t.Context(), "SELECT count(*) FROM attempts WHERE run_id = ?", d.RunID).Scan(&beforeCount); err != nil {
		t.Fatal(err)
	}
	dir := workers.AttemptDir(f.state, d.RunID, d.AttemptID)
	beforeSpool, err := os.ReadFile(filepath.Join(dir, supervisor.SpoolFile))
	if err != nil {
		t.Fatal(err)
	}
	// Recovery may project a launch that the worker already durably emitted.
	// Snapshot journal plus complete spool records before recovery, by event ID.
	beforeLaunches := map[string]journal.Event{}
	for _, event := range f.events(t, d.RunID) {
		recordRecoveryLaunch(t, beforeLaunches, event)
	}
	recordRecoverySpoolLaunches(t, beforeLaunches, beforeSpool)
	raw, err := os.ReadFile(filepath.Join(dir, "worker.json"))
	if err != nil {
		t.Fatal(err)
	}
	var notices []string
	out, err := supervisor.RecoverWithHooks(t.Context(), j, d.RunID, supervisor.Hooks{Notice: func(s string) { notices = append(notices, s) }})
	if !errors.Is(err, supervisor.ErrOwnership) || out.Mode != "interrupted_and_quarantined" || out.State != supervisor.RunInterrupted || out.AttemptState != supervisor.AttemptQuarantined || out.Reason != "worker_lost" || out.AttemptReason != "worker_lost" {
		t.Fatalf("recovery=%+v err=%v", out, err)
	}
	if !slices.Equal(out.UnresolvedPIDs, wantPIDs) {
		t.Fatalf("unresolved PIDs=%v want %v", out.UnresolvedPIDs, wantPIDs)
	}
	notice := strings.Join(notices, "\n")
	wantList := "matching live native PIDs ["
	for i, pid := range wantPIDs {
		if i > 0 {
			wantList += " "
		}
		wantList += strconv.Itoa(pid)
	}
	wantList += "]"
	if !strings.Contains(notice, wantList) || !strings.Contains(notice, "descendant ownership unobserved") || !strings.Contains(notice, "no process was signalled") {
		t.Fatalf("notice=%q want %q", notice, wantList)
	}
	after, err := j.LatestAttempt(t.Context(), d.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if before.AttemptID != after.AttemptID || before.RunID != after.RunID || before.TaskID != after.TaskID || before.LaunchTokenSHA256 != after.LaunchTokenSHA256 || before.WorkspacePath != after.WorkspacePath || before.AttemptNumber != after.AttemptNumber {
		t.Fatalf("launch intent changed: %+v => %+v", before, after)
	}
	var afterCount int
	if err := rawDB(t, f.state).QueryRowContext(t.Context(), "SELECT count(*) FROM attempts WHERE run_id = ?", d.RunID).Scan(&afterCount); err != nil {
		t.Fatal(err)
	}
	if beforeCount != afterCount {
		t.Fatal("recovery created another attempt")
	}
	afterLaunches := map[string]journal.Event{}
	for _, event := range f.events(t, d.RunID) {
		recordRecoveryLaunch(t, afterLaunches, event)
	}
	afterSpool, err := os.ReadFile(filepath.Join(dir, supervisor.SpoolFile))
	if err != nil || !bytes.Equal(beforeSpool, afterSpool) {
		t.Fatalf("worker spool changed during recovery: %v", err)
	}
	recordRecoverySpoolLaunches(t, afterLaunches, afterSpool)
	if !reflect.DeepEqual(beforeLaunches, afterLaunches) {
		t.Fatalf("launch observations changed: before=%v after=%v", beforeLaunches, afterLaunches)
	}
	afterRaw, err := os.ReadFile(filepath.Join(dir, "worker.json"))
	if err != nil || !bytes.Equal(raw, afterRaw) {
		t.Fatalf("worker identity rewritten: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "stop.request")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery requested signalling: %v", err)
	}
	if _, err := j.Candidate(t.Context(), d.AttemptID); !errors.Is(err, journal.ErrNotFound) {
		t.Fatalf("unresolved recovery froze candidate: %v", err)
	}
}

func TestRecoverLaunchBoundaryOutcomes(t *testing.T) {
	t.Parallel()
	for _, boundary := range []string{"intent only (i) and (ii)", "recorded live suspended (iii)", "recorded gone (iv) and (v)"} {
		t.Run(boundary, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			d, _, wait := seedLaunch(t, f, "happy")
			wait()
			truncateBeforeExit(t, f, d)
			id, err := workers.ReadIdentity(workers.AttemptDir(f.state, d.RunID, d.AttemptID))
			if err != nil {
				t.Fatal(err)
			}
			id.NativePID, id.NativeStartTime = nil, nil
			var want []int
			var native workers.ProcessIdentity
			if boundary != "intent only (i) and (ii)" {
				cmd, observed, h := suspendedRecoveryNative(t, d.Workdir)
				native = observed
				id.NativePID = &native.PID
				id.NativeStartTime = &native.StartTime
				if boundary == "recorded gone (iv) and (v)" {
					if err := cmd.Process.Kill(); err != nil {
						t.Fatal(err)
					}
					_ = cmd.Wait()
					if status, err := windows.WaitForSingleObject(h, 0); err != nil || status != windows.WAIT_OBJECT_0 {
						t.Fatalf("test native not exited: %d %v", status, err)
					}
				} else {
					want = []int{native.PID}
				}
			}
			persistRecoveryIdentity(t, f, d, id)
			recoverUnresolvedWindows(t, f, d, want)
			// Repeated recovery keeps quarantine even when no native is observable.
			recoverUnresolvedWindows(t, f, d, want)
			if len(want) > 0 {
				if start, err := workers.ProcessStartTime(native.PID); err != nil || !start.Equal(native.StartTime) {
					t.Fatalf("recovery touched suspended native: %s %v", start, err)
				}
			}
			if _, err := os.Stat(filepath.Join(d.Workdir, "tree.jsonl")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("suspended native ran: %v", err)
			}
		})
	}
}

func TestRecoverNeverMatchesReusedPID(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	d, _, wait := seedLaunch(t, f, "happy")
	wait()
	truncateBeforeExit(t, f, d)
	_, native, _ := suspendedRecoveryNative(t, d.Workdir)
	id, err := workers.ReadIdentity(workers.AttemptDir(f.state, d.RunID, d.AttemptID))
	if err != nil {
		t.Fatal(err)
	}
	wrongStart := native.StartTime.Add(-time.Second)
	id.NativePID = &native.PID
	id.NativeStartTime = &wrongStart
	persistRecoveryIdentity(t, f, d, id)
	recoverUnresolvedWindows(t, f, d, nil)
	if start, err := workers.ProcessStartTime(native.PID); err != nil || !start.Equal(native.StartTime) {
		t.Fatalf("reused PID was touched: %s %v", start, err)
	}
}

func TestRecoverAfterWorkerCrashWindows(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	d, proc, wait := seedLaunch(t, f, "tree")
	dir := workers.AttemptDir(f.state, d.RunID, d.AttemptID)
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	var descendants map[string]workers.ProcessIdentity
	var handles []windows.Handle
	for descendants == nil {
		select {
		case <-deadline.C:
			t.Fatal("tree did not become live")
		case <-tick.C:
		}
		data, err := os.ReadFile(filepath.Join(d.Workdir, "tree.jsonl"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		rows := map[string]workers.ProcessIdentity{}
		breakawayDenied := false
		dec := json.NewDecoder(bytes.NewReader(data))
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
				t.Fatal(err)
			}
			if row.Role == "breakaway" {
				if row.Error == nil || *row.Error != 5 || row.PID != 0 {
					t.Fatalf("breakaway not denied: %+v", row)
				}
				breakawayDenied = true
			} else {
				rows[row.Role] = workers.ProcessIdentity{PID: row.PID, StartTime: row.StartTime}
			}
		}
		if len(rows) == 2 && breakawayDenied {
			descendants = rows
		}
	}
	id, err := workers.ReadIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if id.NativePID == nil || id.NativeStartTime == nil {
		t.Fatal("native identity missing")
	}
	descendants["native"] = workers.ProcessIdentity{PID: *id.NativePID, StartTime: *id.NativeStartTime}
	if _, err := workers.OrphanPIDs(d.AttemptID); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("Windows orphan enumeration changed: %v", err)
	}
	for role, observed := range descendants {
		h, err := pinRecoveryIdentity(t, observed)
		if err != nil {
			t.Fatalf("%s identity changed before pinning: %v", role, err)
		}
		handles = append(handles, h)
		start, err := workers.ProcessStartTime(observed.PID)
		if err != nil || !start.Equal(observed.StartTime) {
			t.Fatalf("%s was not live before worker crash: %s %v", role, start, err)
		}
	}
	if start, err := workers.ProcessStartTime(proc.Pid); err != nil || !start.Equal(id.StartTime) || id.PID != proc.Pid {
		t.Fatalf("worker identity before crash: %s %v", start, err)
	}
	if err := proc.Kill(); err != nil {
		t.Fatal(err)
	}
	wait()
	for _, h := range handles {
		if status, err := windows.WaitForSingleObject(h, 5000); err != nil || status != windows.WAIT_OBJECT_0 {
			t.Fatalf("job member survived worker crash: %d %v", status, err)
		}
	}
	for role, observed := range descendants {
		if start, err := workers.ProcessStartTime(observed.PID); !errors.Is(err, workers.ErrNoProcess) {
			t.Fatalf("%s still reported live after crash: %s %v", role, start, err)
		}
	}
	recoverUnresolvedWindows(t, f, d, nil)
	// Recovery's inability to observe descendants is distinct from the test's
	// retained-handle evidence that this actual tree died with its worker.
}

func TestRecoveryCleanupRejectsReusedIdentityWindows(t *testing.T) {
	_, id, h := suspendedRecoveryNative(t, t.TempDir())
	t.Run("mismatched creation time", func(t *testing.T) {
		wrong := id
		wrong.StartTime = wrong.StartTime.Add(time.Second)
		if _, err := pinRecoveryIdentity(t, wrong); err == nil {
			t.Error("mismatched process retained for termination cleanup")
		}
	})
	if status, err := windows.WaitForSingleObject(h, 0); err != nil || status != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("mismatched identity cleanup signalled original process: status=%d err=%v", status, err)
	}
	if start, err := workers.ProcessStartTime(id.PID); err != nil || !start.Equal(id.StartTime) {
		t.Fatalf("original process no longer live: %s %v", start, err)
	}
}

// pinRecoveryIdentity registers termination cleanup only for the identity on h.
// Creation time remains readable after exit, which is sufficient to exclude a
// reused PID here; the caller checks liveness separately before the crash.
func pinRecoveryIdentity(t *testing.T, id workers.ProcessIdentity) (windows.Handle, error) {
	t.Helper()
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(id.PID)) //nolint:gosec // G115: fixture PID.
	if err != nil {
		return 0, err
	}
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		_ = windows.CloseHandle(h)
		return 0, err
	}
	if !time.Unix(0, created.Nanoseconds()).Equal(id.StartTime) {
		_ = windows.CloseHandle(h)
		return 0, errors.New("process creation time differs from fixture identity")
	}
	t.Cleanup(func() {
		_ = windows.TerminateProcess(h, 1)
		_, _ = windows.WaitForSingleObject(h, 5000)
		_ = windows.CloseHandle(h)
	})
	return h, nil
}
