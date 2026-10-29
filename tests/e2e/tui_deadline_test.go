//go:build unix

package e2e

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/workers"
)

// Unix only: the run is detached by killing its attached client, and only
// Unix records worker detachment as supported (I14 (v2 §2)).

// TestDetachedDeadlineRecoveryBinary gives a packaged run a two-second
// execution envelope, removes its only client once the worker has launched,
// and requires the worker alone to enforce the pinned deadline (AC-6.11):
// the spool records the deadline request and reason, the worker actually
// exits, and recovery from the journal concludes blocked /
// envelope_deadline_exceeded with no second launch. The fenced case runs a
// per-user supervisor for the state root, so the worker is pinned to its
// boot generation and must still exit rather than await reconnection. A
// user stop requested before the deadline stays cancelled. I21 (v2 §2).
func TestDetachedDeadlineRecoveryBinary(t *testing.T) {
	for _, tc := range []struct {
		name       string
		fenced     bool
		userFirst  bool
		scenario   string
		requester  string
		reason     any
		runState   string
		runReason  string
		recoverRet int
	}{
		{"unfenced deadline", false, false, "slow", workers.DeadlineRequester, workers.DeadlineReason, "blocked", workers.DeadlineReason, 3},
		{"fenced deadline", true, false, "slow", workers.DeadlineRequester, workers.DeadlineReason, "blocked", workers.DeadlineReason, 3},
		// ignore-term keeps the user's ladder running past the deadline,
		// so the deadline timer meets an already accepted request.
		{"user stop first", false, true, "ignore-term", "user-interrupt", nil, "cancelled", "", 130},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := canonicalEnv(t)
			runtimeDir := shortRuntimeDir(t)
			e.vars = append(e.vars, "XDG_RUNTIME_DIR="+runtimeDir)
			if tc.fenced {
				startService(t, e, runtimeDir)
			}
			repo, digest := mkRepo(t, e, canonicalDir(t, t.TempDir()), "pass")
			r := startLive(t, e, repo, append(baseRunArgs(digest), "--format", "jsonl",
				"--scenario", tc.scenario, "--envelope-execution", "2s")...)
			var launched journal.Event
			if err := json.Unmarshal([]byte(r.waitLine(t, "the launch", func(line string) bool {
				return strings.Contains(line, `"type":"attempt.launched"`)
			})), &launched); err != nil {
				t.Fatal(err)
			}
			state, runID := e.state, launched.RunID
			dir := workers.AttemptDir(state, runID, launched.AttemptID)
			worker := workerOf(t, state, runID, launched.AttemptID)
			reapDetached(t, worker)
			generation := waitBeat(t, dir)
			if tc.fenced != (generation > 0) {
				t.Fatalf("launch generation %d, fenced %v", generation, tc.fenced)
			}
			if tc.userFirst {
				if err := r.cmd.Process.Signal(os.Interrupt); err != nil {
					t.Fatal(err)
				}
				r.waitLine(t, "the worker's stop request", func(line string) bool {
					return strings.Contains(line, `"type":"attempt.stop_requested"`)
				})
			}
			// The client goes away without another word: from here only the
			// worker can enforce the envelope.
			if err := r.cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			r.wait(t)

			waitSpool(t, dir, "the worker's terminal state", func(events []journal.Event) bool {
				return finalSpoolState(t, events) != nil
			})
			waitProcessesGone(t, worker)
			events := spoolEvents(t, dir)
			final := finalSpoolState(t, events)
			if got := stopRequesters(t, events); !slices.Equal(got, []string{tc.requester}) || final["state"] != "stopped" || final["reason"] != tc.reason {
				t.Fatalf("spooled requesters %v, final state %v; want one %s stop ending stopped with reason %v", got, final, tc.requester, tc.reason)
			}
			row, _, counts := runSnapshot(t, state, runID)
			if counts["attempt.stopped"] != 0 || (row.State != "executing" && row.State != "stopping") {
				t.Fatalf("before recovery run %s, events %v; the client must not have concluded the attempt", row.State, counts)
			}

			code, stdout, stderr := run(t, e, repo, "recover", runID)
			if code != tc.recoverRet {
				t.Fatalf("recover exit %d, want %d:\n%s\n%s", code, tc.recoverRet, stdout, stderr)
			}
			row, a, counts := runSnapshot(t, state, runID)
			if row.State != tc.runState || row.Reason != tc.runReason || a.State != "stopped" ||
				counts["attempt.launched"] != 1 || counts["attempt.stop_requested"] != 1 {
				t.Fatalf("after recovery run %s (%s), attempt %s, events %v; want %s (%s) from one launch and one stop",
					row.State, row.Reason, a.State, counts, tc.runState, tc.runReason)
			}
		})
	}
}

// finalSpoolState is the payload of the worker's terminal state change, or
// nil while the attempt is still live in its spool.
func finalSpoolState(t *testing.T, events []journal.Event) map[string]any {
	t.Helper()
	for _, ev := range slices.Backward(events) {
		if ev.Type != "attempt.state_changed" {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatal(err)
		}
		switch p["state"] {
		case "launching", "running", "stop_requested":
			return nil
		}
		return p
	}
	return nil
}

// waitBeat waits for the supervisor beat the attached client writes after
// each ingest and returns the boot generation it carries.
func waitBeat(t *testing.T, dir string) int64 {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for {
		if _, generation, ok := workers.ReadSupervisorBeat(dir); ok {
			return generation
		}
		if time.Now().After(deadline) {
			t.Fatal("no supervisor beat was written")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// shortRuntimeDir is a private XDG_RUNTIME_DIR short enough for a Unix
// socket path, so no host supervisor is consulted or disturbed.
func shortRuntimeDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "mhrt") //nolint:usetesting // t.TempDir() is too long for sun_path on macOS
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return canonicalDir(t, dir)
}

// startService runs the per-user supervisor for e's state root and waits
// for its control socket; cleanup terminates and joins it.
func startService(t *testing.T, e env, runtimeDir string) {
	t.Helper()
	cmd := exec.Command(mythhelmBin, control.SupervisorCommand)
	cmd.Env = e.withParent(t)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		killed, err := joinOwned(exited, func() { _ = cmd.Process.Signal(syscall.SIGTERM) }, func() { _ = cmd.Process.Kill() }, ownedJoinGrace)
		switch {
		case err != nil:
			t.Errorf("supervisor %d was never joined: %v", cmd.Process.Pid, err)
		case killed:
			t.Errorf("supervisor ignored SIGTERM and was killed: %s", out.String())
		}
	})
	socket := filepath.Join(runtimeDir, "mythhelm-uid"+strconv.Itoa(os.Getuid()), "control.sock")
	deadline := time.Now().Add(time.Minute)
	for {
		if _, err := os.Stat(socket); err == nil {
			return
		}
		select {
		case <-exited:
			t.Fatalf("supervisor exited before serving: %s", out.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("supervisor never served %s: %s", socket, out.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
