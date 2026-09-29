//go:build unix

package workers

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
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
