//go:build windows

package supervisor_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
)

// The outer test never attaches to a console or generates control events.
// Ctrl-C cannot target a process group, so broadcast is permitted only inside
// a detached helper's newly allocated console, containing test processes alone.
func TestFirstInterruptRequestsStop(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		event uint32
	}{
		{"ctrl_c", windows.CTRL_C_EVENT}, {"ctrl_break", windows.CTRL_BREAK_EVENT},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFirstInterruptConsoleHelper$", "-test.v", "--", strconv.FormatUint(uint64(tc.event), 10)) //nolint:gosec // G702: own executable and fixed test helper with a numeric console-event selector.
			cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS}
			var output bytes.Buffer
			cmd.Stdout, cmd.Stderr = &output, &output
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cmd.Wait() })
			start, err := workers.ProcessStartTime(cmd.Process.Pid)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pinRecoveryIdentity(t, workers.ProcessIdentity{PID: cmd.Process.Pid, StartTime: start}); err != nil {
				t.Fatal(err)
			}
			err = cmd.Wait()
			if err != nil {
				t.Fatalf("isolated %s console: %v\n%s", tc.name, err, output.String())
			}
			if strings.Contains(output.String(), "blocked:") {
				t.Skipf("blocked: %s runner console prerequisite: %s", tc.name, output.String())
			}
			if !strings.Contains(output.String(), "--- PASS: TestFirstInterruptConsoleHelper") {
				t.Fatalf("console helper did not report acceptance: %s", output.String())
			}
			t.Logf("%s", output.String())
		})
	}
}

func TestFirstInterruptConsoleHelper(t *testing.T) {
	if len(os.Args) != 5 || os.Args[3] != "--" {
		return
	}
	// This test changes this isolated process's console and signal registration.
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	ok, _, err := kernel.NewProc("AllocConsole").Call()
	if ok == 0 {
		t.Skipf("blocked: AllocConsole: %v", err)
	}
	defer func() { _, _, _ = kernel.NewProc("FreeConsole").Call() }()
	event, err := strconv.ParseUint(os.Args[4], 10, 32)
	if err != nil || (event != windows.CTRL_C_EVENT && event != windows.CTRL_BREAK_EVENT) {
		t.Fatalf("invalid helper console event %q", os.Args[4])
	}
	// Protect the console driver itself from the event; the actual CLI registers
	// its own handler through executeRun, without any test signal injection.
	driverInterrupts := make(chan os.Signal, 1)
	signal.Notify(driverInterrupts, os.Interrupt)
	defer signal.Stop(driverInterrupts)
	f := newFixture(t)
	r := startLive(t, f, f.fakeRun("--format", "jsonl", "--scenario", "slow")...)
	cliStart, err := workers.ProcessStartTime(r.cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pinRecoveryIdentity(t, workers.ProcessIdentity{PID: r.cmd.Process.Pid, StartTime: cliStart}); err != nil {
		t.Fatal(err)
	}
	r.waitStdout(t, "the native session", isType("attempt.native_session"))
	run := f.onlyRun(t)
	attemptID := findAttempt(t, f, run.RunID)
	dir := workers.AttemptDir(f.state, run.RunID, attemptID)
	id, err := workers.ReadIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	h, err := pinRecoveryIdentity(t, workers.ProcessIdentity{PID: id.PID, StartTime: id.StartTime})
	if err != nil {
		t.Fatal(err)
	}
	if status, err := windows.WaitForSingleObject(h, 0); err != nil || status != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("worker is not live before console interrupt: %d %v", status, err)
	}
	if err := windows.GenerateConsoleCtrlEvent(uint32(event), 0); err != nil {
		t.Skipf("blocked: GenerateConsoleCtrlEvent(%d) in isolated console: %v", event, err)
	}
	select {
	case <-driverInterrupts:
	case <-time.After(5 * time.Second):
		t.Fatal("isolated console driver did not observe the generated event")
	}
	code, _, stderr := r.wait(t)
	if code != 130 {
		t.Fatalf("console event %d: exit=%d stderr=%q, want confirmed exit 130", event, code, stderr)
	}
	// Read durable evidence independently of the CLI's rendered output. Direct
	// delivery to the worker would kill its job without a confirmed stop receipt.
	events := f.events(t, run.RunID)
	requested, stopped := typeIndex(events, "attempt.stop_requested"), typeIndex(events, "attempt.stopped")
	if requested < 0 || stopped <= requested {
		t.Fatalf("durable stop order: requested=%d stopped=%d", requested, stopped)
	}
	payload := payloadOf(t, events[stopped])
	if payload["confirmed"] != true || payload["descendant_scan"] != "job-object" {
		t.Fatalf("unconfirmed console stop: %v", payload)
	}
	counts := eventTypeCounts(t, f, run.RunID)
	if counts["attempt.stop_requested"] != 1 || counts["attempt.launched"] != 1 {
		t.Fatalf("console event changed launch/stop counts: %v", counts)
	}
	if got := f.onlyRun(t); got.State != string(supervisor.RunCancelled) {
		t.Fatalf("durable state=%s, want cancelled", got.State)
	}
	t.Logf("console event %d: durable attempt.stop_requested, confirmed job-object stop, exit 130", event)
}

func TestSecondInterruptDetaches(t *testing.T) {
	t.Parallel()
	// An ignore-sigint fake cannot hold TerminateJobObject open: StopKill is the
	// only qualified Windows rung. Queuing two interrupts would test a scheduler
	// race, not a held-open stop. Never substitute that for worker-alive exit 6.
	t.Skipf("blocked: Windows %s stop-ladder/v2-windows has only immediate job termination; no approved fake scenario holds the stop open for a second console interrupt; worker_detachment remains unknown pending Task 11 qualification", runtime.GOARCH)
}
