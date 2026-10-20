//go:build windows

package workers_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/turbokast/mythhelm/internal/cli"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
)

type detachConsoleReady struct {
	Window  uintptr
	Class   string
	Visible bool
}

// The selected helper runs the real CLI; the console-close variant starts
// with its own console created before Go registers the runtime handler.
// Neither the test runner nor its host console is ever attached to it.
func TestDetachConsoleHelper(t *testing.T) {
	if len(os.Args) < 5 || os.Args[2] != "--" {
		return
	}
	ready := detachConsoleReady{}
	if os.Args[3] == "console_close" {
		// The parent created this console before Go startup, preserving the
		// runtime handler table. Never allocate/attach a console after startup.
		ready.Window, _, _ = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow").Call()
		if ready.Window != 0 {
			var class [128]uint16
			n, err := windows.GetClassName(windows.HWND(ready.Window), &class[0], int32(len(class)))
			if err != nil {
				t.Fatal(err)
			}
			ready.Class = windows.UTF16ToString(class[:n])
			ready.Visible = windows.IsWindowVisible(windows.HWND(ready.Window))
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(ready); err != nil {
		t.Fatal(err)
	}
	os.Exit(cli.Main(os.Args[4:], cli.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}))
}

func TestSurvivesCLIExitAndConsoleClose(t *testing.T) {
	// Each case installs a private runtime directory for real supervisor recovery.
	t.Run("cleanup_exit_race", testDetachCleanupExitRace)
	outer := t
	for _, action := range []string{"cli_exit", "console_close", "early_setup_return"} {
		var earlyWorker, earlyNative windows.Handle
		t.Run(action, func(t *testing.T) {
			tempDir := t.TempDir
			if action == "early_setup_return" {
				// Keep files intact until the outer exit assertions. TempDir deletion
				// must not stop a mutant's live worker and mask the missing fallback.
				tempDir = outer.TempDir
			}
			state, repo, runtimeDir := tempDir(), tempDir(), tempDir()
			t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
			release, err := control.AcquireInstance(state)
			if err != nil {
				t.Fatal(err)
			}
			release() // Persist the boot generation used by the worker spawn.
			gitHome := tempDir()
			for _, args := range [][]string{
				{"init", "--quiet", "--initial-branch=main"},
				{"config", "user.name", "Test"},
				{"config", "user.email", "test@example.com"},
				{"commit", "--quiet", "--allow-empty", "-m", "synthetic fixture"},
			} {
				cmd := exec.CommandContext(t.Context(), "git", args...)
				cmd.Dir = repo
				cmd.Env = append(os.Environ(), "HOME="+gitHome, "XDG_CONFIG_HOME="+gitHome, "GIT_CONFIG_NOSYSTEM=1")
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git: %v: %s", err, out)
				}
			}
			task := filepath.Join(tempDir(), "task.md")
			if err := os.WriteFile(task, []byte("# Synthetic detach task\nWrite demo.txt.\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			args := []string{"-test.run=^TestDetachConsoleHelper$", "--", action, "run", "--task-file", task, "--non-interactive", "--adapter", "fake", "--billing", "local-scripted", "--execution-profile", "trusted-host", "--no-checks", "--scenario", "slow", "--format", "jsonl"}
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], args...) //nolint:gosec // G702: own test executable, fixed helper selector and synthetic fixture paths.
			cmd.Env = append(os.Environ(), "MYTHHELM_HOME="+state, "HOME="+tempDir(), "XDG_CONFIG_HOME="+tempDir())
			cmd.Dir = repo
			flags := uint32(windows.DETACHED_PROCESS)
			if action == "console_close" {
				flags = windows.CREATE_NEW_CONSOLE
			}
			cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
			output, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var diagnostics bytes.Buffer
			cmd.Stderr = &diagnostics
			// Registered before launch: the later helper pin and Wait cleanups run
			// first, preventing the spawning CLI from racing this private-state scan.
			t.Cleanup(func() { cleanupDetachState(t, state) })
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cmd.Wait() })
			start, err := workers.ProcessStartTime(cmd.Process.Pid)
			if err != nil {
				t.Fatal(err)
			}
			helper := pinDetachIdentity(t, workers.ProcessIdentity{PID: cmd.Process.Pid, StartTime: start})
			scan := bufio.NewScanner(output)
			scan.Buffer(make([]byte, 4096), 1<<20)
			if !scan.Scan() {
				t.Fatalf("console helper has no readiness: %v", scan.Err())
			}
			var ready detachConsoleReady
			if err := json.Unmarshal(scan.Bytes(), &ready); err != nil {
				t.Fatal(err)
			}
			if action == "early_setup_return" {
				// Intentionally return before consuming attempt.launched. Only the
				// pre-launch fallback owns teardown; parent-lifetime emergency cleanup
				// runs only after the exit assertions and keeps a failing mutant leak-free.
				// Anchor the identity deadline to launch evidence, excluding admission
				// and Git setup; leave attempt.launched unread in the output pipe.
				intentObserved := false
				for scan.Scan() {
					var ev journal.Event
					if err := json.Unmarshal(scan.Bytes(), &ev); err != nil {
						t.Fatal(err)
					}
					if ev.Type == "attempt.launch_intent_recorded" {
						intentObserved = true
						break
					}
				}
				if !intentObserved {
					t.Fatalf("early-return fixture lacks launch intent: %v", scan.Err())
				}
				id := waitDetachNativeIdentity(t, state)
				earlyWorker, err = openDetachIdentity(workers.ProcessIdentity{PID: id.PID, StartTime: id.StartTime})
				if err != nil {
					t.Fatal(err)
				}
				outer.Cleanup(func() { terminateDetachHandle(outer, earlyWorker); _ = windows.CloseHandle(earlyWorker) })
				earlyNative, err = openDetachIdentity(workers.ProcessIdentity{PID: *id.NativePID, StartTime: *id.NativeStartTime})
				if err != nil {
					t.Fatal(err)
				}
				outer.Cleanup(func() { terminateDetachHandle(outer, earlyNative); _ = windows.CloseHandle(earlyNative) })
				requireDetachedLive(t, earlyWorker, id.StartTime)
				requireDetachedLive(t, earlyNative, *id.NativeStartTime)
				return
			}
			// Install worker cleanup as soon as its first durable launch evidence is
			// rendered, before any later native-session or console assertion can fail.
			var id workers.Identity
			var dir, runID, attemptID string
			var worker, native windows.Handle
			sessionStarted := false
			var lastLine string
			for scan.Scan() {
				lastLine = scan.Text()
				var ev journal.Event
				if err := json.Unmarshal(scan.Bytes(), &ev); err != nil {
					t.Fatal(err)
				}
				if ev.Type == "attempt.launched" {
					runID, attemptID = ev.RunID, ev.AttemptID
					dir = workers.AttemptDir(state, runID, attemptID)
					id, err = workers.ReadIdentity(dir)
					if err != nil {
						t.Fatal(err)
					}
					worker = pinDetachIdentity(t, workers.ProcessIdentity{PID: id.PID, StartTime: id.StartTime})
					if id.NativePID == nil || id.NativeStartTime == nil {
						t.Fatal("missing native identity")
					}
					native = pinDetachIdentity(t, workers.ProcessIdentity{PID: *id.NativePID, StartTime: *id.NativeStartTime})
				}
				if ev.Type == "attempt.native_session" {
					sessionStarted = true
					break
				}
			}
			if !sessionStarted || worker == 0 || native == 0 {
				_ = cmd.Wait()
				t.Fatalf("CLI never established a native session: %v; last=%s; stderr=%s", scan.Err(), lastLine, diagnostics.String())
			}
			if action == "console_close" {
				t.Logf("test-owned console class=%q visible=%t", ready.Class, ready.Visible)
				if ready.Class == "PseudoConsoleWindow" {
					t.Skip("blocked: console_close: GetConsoleWindow returned a PseudoConsoleWindow message window; a physical console-window close is unavailable on this runner")
				}
				if ready.Window == 0 {
					t.Skip("blocked: console_close: allocated console has no window on this runner")
				}
				ok, _, err := windows.NewLazySystemDLL("user32.dll").NewProc("PostMessageW").Call(ready.Window, 0x0010, 0, 0) // WM_CLOSE on this helper's console only.
				if ok == 0 {
					t.Skipf("blocked: console_close: PostMessageW(WM_CLOSE): %v", err)
				}
			} else {
				if err := windows.TerminateProcess(helper, 0); err != nil {
					t.Fatal(err)
				}
			}
			status, err := windows.WaitForSingleObject(helper, 5000)
			if err != nil || status != windows.WAIT_OBJECT_0 {
				t.Fatalf("CLI did not exit after %s: status=%d err=%v console_class=%q visible=%t", action, status, err, ready.Class, ready.Visible)
			}
			requireDetachedLive(t, worker, id.StartTime)
			requireDetachedLive(t, native, *id.NativeStartTime)
			j, err := journal.Open(t.Context(), state)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = j.Close() }()
			detach := make(chan struct{}, 1)
			detach <- struct{}{}
			out, err := supervisor.RecoverWithHooks(ctx, j, runID, supervisor.Hooks{Detach: detach})
			if err != nil || !out.DetachedRunning || out.Mode != "reattached" || out.RunID != runID || out.AttemptID != attemptID || out.AttemptState != supervisor.AttemptRunning {
				t.Fatalf("reconnect after %s: %+v %v", action, out, err)
			}
			after, err := workers.ReadIdentity(dir)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(id, after) {
				t.Fatalf("reconnect changed launch identity: before=%+v after=%+v", id, after)
			}
			requireDetachedLive(t, worker, id.StartTime)
			requireDetachedLive(t, native, *id.NativeStartTime)
			events, err := j.Events(ctx, runID, 0)
			if err != nil {
				t.Fatal(err)
			}
			launches, intents, requests := 0, 0, 0
			for _, ev := range events {
				switch ev.Type {
				case "attempt.launched":
					launches++
				case "attempt.launch_intent_recorded":
					intents++
				case "attempt.stop_requested":
					requests++
				}
			}
			if launches != 1 || intents != 1 || requests != 0 {
				t.Fatalf("reconnect launch/intent/stop counts=%d/%d/%d", launches, intents, requests)
			}
			if err := workers.RequestStop(dir, "detach-test-stop"); err != nil {
				t.Fatal(err)
			}
			stopped, err := supervisor.RecoverWithHooks(ctx, j, runID, supervisor.Hooks{})
			if err != nil || stopped.AttemptState != supervisor.AttemptStopped {
				t.Fatalf("post-reconnect stop: %+v %v", stopped, err)
			}
		})
		if action == "early_setup_return" && earlyWorker != 0 && earlyNative != 0 {
			for _, h := range []windows.Handle{earlyWorker, earlyNative} {
				status, err := windows.WaitForSingleObject(h, 5000)
				if err != nil || status != windows.WAIT_OBJECT_0 {
					t.Errorf("early setup cleanup left owned process alive: status=%d err=%v", status, err)
				}
			}
		}
	}
}

// Check creation time on the same retained handle before registering any
// termination. A failed identity observation can never authorize cleanup.
var errDetachIdentityMismatch = errors.New("fixture process creation time differs")

func openDetachIdentity(id workers.ProcessIdentity) (windows.Handle, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(id.PID)) //nolint:gosec // G115: fixture PID originates from Windows.
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
		return 0, errDetachIdentityMismatch
	}
	return h, nil
}

func pinDetachIdentity(t *testing.T, id workers.ProcessIdentity) windows.Handle {
	t.Helper()
	h, err := openDetachIdentity(id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { terminateDetachHandle(t, h); _ = windows.CloseHandle(h) })
	return h
}

func terminateDetachHandle(t *testing.T, h windows.Handle) bool {
	t.Helper()
	if err := terminateDetachProcess(h, windows.TerminateProcess); err != nil {
		t.Error(err)
		return false
	}
	return true
}

// A matching retained object may exit between the initial wait and terminate.
// ERROR_ACCESS_DENIED then means exit only if this same object is signalled.
func terminateDetachProcess(h windows.Handle, terminate func(windows.Handle, uint32) error) error {
	status, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		return fmt.Errorf("fixture exit observation unknown: %w", err)
	}
	if status == windows.WAIT_OBJECT_0 {
		return nil
	}
	if status != uint32(windows.WAIT_TIMEOUT) {
		return fmt.Errorf("fixture exit observation unknown: status=%d", status)
	}
	if err := terminate(h, 1); err != nil {
		status, waitErr := windows.WaitForSingleObject(h, 0)
		if waitErr == nil && status == windows.WAIT_OBJECT_0 {
			return nil
		}
		return fmt.Errorf("terminate fixture identity: %w; exit status=%d wait_error=%v", err, status, waitErr)
	}
	status, err = windows.WaitForSingleObject(h, 5000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("fixture identity cleanup unconfirmed: status=%d err=%v", status, err)
	}
	return nil
}

func testDetachCleanupExitRace(t *testing.T) {
	t.Helper()
	for _, exitDuringTermination := range []bool{true, false} {
		t.Run(fmt.Sprintf("exit_before_denial_%t", exitDuringTermination), func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^$") //nolint:gosec // G702: own test executable with a fixed empty test selector.
			cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED | windows.DETACHED_PROCESS}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cmd.Wait() })
			start, err := workers.ProcessStartTime(cmd.Process.Pid)
			if err != nil {
				t.Fatal(err)
			}
			h := pinDetachIdentity(t, workers.ProcessIdentity{PID: cmd.Process.Pid, StartTime: start})
			denial := func(h windows.Handle, _ uint32) error {
				if exitDuringTermination {
					if err := windows.TerminateProcess(h, 0); err != nil {
						return err
					}
					status, err := windows.WaitForSingleObject(h, 5000)
					if err != nil || status != windows.WAIT_OBJECT_0 {
						return fmt.Errorf("fixture concurrent exit unconfirmed: status=%d err=%v", status, err)
					}
				}
				return windows.ERROR_ACCESS_DENIED
			}
			err = terminateDetachProcess(h, denial)
			if exitDuringTermination {
				status, waitErr := windows.WaitForSingleObject(h, 0)
				if err != nil || waitErr != nil || status != windows.WAIT_OBJECT_0 {
					t.Fatalf("exited retained object not confirmed: err=%v status=%d wait_error=%v", err, status, waitErr)
				}
			} else {
				requireDetachedLive(t, h, start)
				if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
					t.Fatalf("live denied process reported gone: %v", err)
				}
			}
		})
	}
}

func waitDetachNativeIdentity(t *testing.T, state string) workers.Identity {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		files, err := filepath.Glob(filepath.Join(state, "runs", "*", "attempts", "*", "worker.json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			id, err := workers.ReadIdentity(filepath.Dir(file))
			if err == nil && id.NativePID != nil && id.NativeStartTime != nil {
				return id
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("early-return fixture never persisted native identity")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The spawning CLI is already quiescent when this cleanup runs. Scan only
// this test's private durable identities, never OS process names or PID ranges.
func cleanupDetachState(t *testing.T, state string) {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join(state, "runs", "*", "attempts", "*"))
	if err != nil {
		t.Error(err)
		return
	}
	for _, dir := range dirs {
		id, err := workers.ReadIdentity(dir)
		if err != nil {
			t.Errorf("cleanup worker identity unknown: %v", err)
			continue
		}
		h, err := openDetachIdentity(workers.ProcessIdentity{PID: id.PID, StartTime: id.StartTime})
		switch {
		case err == nil:
			gone := terminateDetachHandle(t, h)
			_ = windows.CloseHandle(h)
			if !gone {
				continue
			}
		case errors.Is(err, windows.ERROR_INVALID_PARAMETER), errors.Is(err, errDetachIdentityMismatch):
			// The recorded worker is gone; a reused PID grants no termination authority.
		default:
			t.Errorf("cleanup worker observation unknown: %v", err)
			continue
		}
		// A worker may have persisted its native after the first read. Re-read only
		// after its exact process has exited so it cannot create another native.
		id, err = workers.ReadIdentity(dir)
		if err != nil {
			t.Errorf("cleanup final identity unknown: %v", err)
			continue
		}
		if id.NativePID == nil || id.NativeStartTime == nil {
			if id.NativeLaunchIntent != nil {
				t.Error("cleanup native identity unknown after launch intent")
			}
			continue
		}
		h, err = openDetachIdentity(workers.ProcessIdentity{PID: *id.NativePID, StartTime: *id.NativeStartTime})
		switch {
		case err == nil:
			terminateDetachHandle(t, h)
			_ = windows.CloseHandle(h)
		case errors.Is(err, windows.ERROR_INVALID_PARAMETER), errors.Is(err, errDetachIdentityMismatch):
		default:
			t.Errorf("cleanup native observation unknown: %v", err)
		}
	}
}

func requireDetachedLive(t *testing.T, h windows.Handle, start time.Time) {
	t.Helper()
	status, err := windows.WaitForSingleObject(h, 0)
	if err != nil || status != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("detached process not live: status=%d err=%v", status, err)
	}
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		t.Fatal(err)
	}
	if !time.Unix(0, created.Nanoseconds()).Equal(start) {
		t.Fatal("detached process creation time changed")
	}
}
