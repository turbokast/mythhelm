//go:build unix

package workers

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/turbokast/mythhelm/internal/adapter"
)

func TestWorkerSurvivesParentExit(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("worker detachment is claimed on Linux and macOS only")
	}
	a := newAttempt(t)
	l := a.launch(t, "slow")
	body, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	// The helper plays the `mythhelm run` CLI, in its own process group as
	// a shell job would be.
	helper := exec.Command(os.Args[0]) //nolint:gosec // G702: re-executes this test binary
	helper.Env = append(os.Environ(), spawnHelperEnv+"="+strings.Join([]string{a.state, a.runID, a.attemptID}, "\n"))
	helper.Stdin = strings.NewReader(string(body))
	helper.Stderr = os.Stderr
	helper.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out, err := helper.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = helper.Process.Kill(); _ = helper.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatalf("reading the worker PID from the helper: %v", err)
	}
	workerPID, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Kill(workerPID, unix.SIGKILL) })
	a.waitSessionStarted(t)

	// Kill the helper's whole process group, as closing its terminal or
	// killing the CLI's job would.
	if err := unix.Kill(-helper.Process.Pid, unix.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = helper.Wait()

	first := a.heartbeat(t)
	deadline := time.Now().Add(20 * time.Second)
	for a.heartbeat(t) < first+2 {
		if time.Now().After(deadline) {
			t.Fatalf("heartbeat stuck at %d after the helper died (log %q)", first, a.workerLog())
		}
		time.Sleep(100 * time.Millisecond)
	}
	if sid, err := unix.Getsid(workerPID); err != nil || sid != workerPID {
		t.Errorf("worker session = %d, %v; want its own session %d", sid, err, workerPID)
	}

	id, err := ReadIdentity(a.dir)
	if err != nil {
		t.Fatal(err)
	}
	started, err := ProcessStartTime(workerPID)
	if err != nil {
		t.Fatal(err)
	}
	if id.PID != workerPID || !id.StartTime.Equal(started) || id.LaunchToken != l.LaunchToken {
		t.Errorf("identity = %+v, want pid %d, start %v and the launch token", id, workerPID, started)
	}
	if id.NativeLaunchIntent == nil || id.NativePID == nil || id.NativePGID == nil || *id.NativePGID != *id.NativePID {
		t.Errorf("identity = %+v, want the native launch intent, PID and its own process group", id)
	}

	// The orphaned worker still obeys a stop request.
	if err := RequestStop(a.dir, "test-stop"); err != nil {
		t.Fatal(err)
	}
	if st, _ := final(t, a.waitDone(t)); st != "stopped" {
		t.Errorf("final state = %s, want stopped", st)
	}
}

// heartbeat returns the worker's current beat count, 0 before the first.
func (a attempt) heartbeat(t *testing.T) int64 {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(a.dir, "heartbeat"))
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	var hb struct {
		Beat int64 `json:"beat"`
	}
	if json.Unmarshal(b, &hb) != nil {
		return 0 // caught mid-write; the next read sees it whole
	}
	return hb.Beat
}

// TestProcGoneMapsReadRace pins the /proc open/read race: a worker reaped
// between open and read surfaces ESRCH on the read, which must mean "gone"
// exactly like ENOENT, or verifyIdentity rejects a legitimately reaped
// worker (exit 6 worker_lost) instead of accepting it on PID and token.
func TestProcGoneMapsReadRace(t *testing.T) {
	readRace := &fs.PathError{Op: "read", Path: "/proc/9976/stat", Err: syscall.ESRCH}
	if !procGone(readRace) {
		t.Errorf("procGone(%v) = false, want true", readRace)
	}
	missing := &fs.PathError{Op: "open", Path: "/proc/9976/stat", Err: syscall.ENOENT}
	if !procGone(missing) {
		t.Errorf("procGone(%v) = false, want true", missing)
	}
	denied := &fs.PathError{Op: "open", Path: "/proc/1/stat", Err: syscall.EACCES}
	if procGone(denied) {
		t.Errorf("procGone(%v) = true, want false: undeterminable stays fail-closed", denied)
	}
}

func TestWorkerClimbsPinnedLadder(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("process-group stop semantics are claimed on Linux and macOS only")
	}
	// The scenario ignores SIGINT, so both rungs fire whatever the grace;
	// short graces keep the test fast.
	two := []adapter.StopStep{
		{Signal: adapter.StopInterrupt, Grace: time.Second},
		{Signal: adapter.StopTerminate, Grace: 5 * time.Second},
	}
	p := climbPinnedLadder(t, "ignore-sigint", two)
	if !p.Confirmed || !slices.Equal(p.Sent, two) || p.LadderVersion != "stop-ladder/v1" {
		t.Errorf("attempt.stopped = %+v, want confirmed with sent %v under stop-ladder/v1", p, two)
	}
	if len(p.UnresolvedPIDs) != 0 {
		t.Errorf("attempt.stopped unresolved_pids = %v, want none", p.UnresolvedPIDs)
	}
	// Deleting one rung from the handoff changes the recorded receipt: the
	// receipt reflects the climb, not the default ladder.
	one := []adapter.StopStep{{Signal: adapter.StopTerminate, Grace: 5 * time.Second}}
	p = climbPinnedLadder(t, "ignore-sigint", one)
	if !p.Confirmed || !slices.Equal(p.Sent, one) {
		t.Errorf("attempt.stopped = %+v, want confirmed with sent %v", p, one)
	}
}
