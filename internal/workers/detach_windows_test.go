//go:build windows

package workers_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
	Blocked string
}

// The selected helper runs the real CLI with a newly allocated console.
// Neither the test runner nor its host console is ever attached to it.
func TestDetachConsoleHelper(t *testing.T) {
	if len(os.Args) < 5 || os.Args[2] != "--" {
		return
	}
	ready := detachConsoleReady{}
	if os.Args[3] == "console_close" {
		kernel := windows.NewLazySystemDLL("kernel32.dll")
		ok, _, err := kernel.NewProc("AllocConsole").Call()
		if ok == 0 {
			_ = json.NewEncoder(os.Stdout).Encode(detachConsoleReady{Blocked: fmt.Sprintf("AllocConsole: %v", err)})
			return
		}
		ready.Window, _, _ = kernel.NewProc("GetConsoleWindow").Call()
	}
	if err := json.NewEncoder(os.Stdout).Encode(ready); err != nil {
		t.Fatal(err)
	}
	os.Exit(cli.Main(os.Args[4:], cli.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}))
}

func TestSurvivesCLIExitAndConsoleClose(t *testing.T) {
	// Each case installs a private runtime directory for real supervisor recovery.
	for _, action := range []string{"cli_exit", "console_close"} {
		t.Run(action, func(t *testing.T) {
			state, repo, runtimeDir := t.TempDir(), t.TempDir(), t.TempDir()
			t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
			release, err := control.AcquireInstance(state)
			if err != nil {
				t.Fatal(err)
			}
			release() // Persist the boot generation used by the worker spawn.
			for _, args := range [][]string{
				{"init", "--quiet", "--initial-branch=main"},
				{"config", "user.name", "Test"},
				{"config", "user.email", "test@example.com"},
				{"-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--quiet", "--allow-empty", "-m", "synthetic fixture"},
			} {
				cmd := exec.CommandContext(t.Context(), "git", args...)
				cmd.Dir = repo
				cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "GIT_CONFIG_NOSYSTEM=1")
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git: %v: %s", err, out)
				}
			}
			task := filepath.Join(t.TempDir(), "task.md")
			if err := os.WriteFile(task, []byte("# Synthetic detach task\nWrite demo.txt.\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			args := []string{"-test.run=^TestDetachConsoleHelper$", "--", action, "run", "--task-file", task, "--non-interactive", "--adapter", "fake", "--billing", "local-scripted", "--execution-profile", "trusted-host", "--no-checks", "--scenario", "slow", "--format", "jsonl"}
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], args...) //nolint:gosec // G702: own test executable, fixed helper selector and synthetic fixture paths.
			cmd.Env = append(os.Environ(), "MYTHHELM_HOME="+state, "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
			cmd.Dir = repo
			cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS}
			output, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var diagnostics bytes.Buffer
			cmd.Stderr = &diagnostics
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
			if ready.Blocked != "" {
				t.Skipf("blocked: %s: %s", action, ready.Blocked)
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
				t.Fatalf("CLI did not exit after %s: status=%d err=%v", action, status, err)
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
	}
}

// Check creation time on the same retained handle before registering any
// termination. A failed identity observation can never authorize cleanup.
func pinDetachIdentity(t *testing.T, id workers.ProcessIdentity) windows.Handle {
	t.Helper()
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(id.PID)) //nolint:gosec // G115: fixture PID originates from Windows.
	if err != nil {
		t.Fatal(err)
	}
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		_ = windows.CloseHandle(h)
		t.Fatal(err)
	}
	if !time.Unix(0, created.Nanoseconds()).Equal(id.StartTime) {
		_ = windows.CloseHandle(h)
		t.Fatal("fixture process creation time differs")
	}
	t.Cleanup(func() {
		_ = windows.TerminateProcess(h, 1)
		_, _ = windows.WaitForSingleObject(h, 5000)
		_ = windows.CloseHandle(h)
	})
	return h
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
