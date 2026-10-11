//go:build unix

package codex

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
)

// groupLauncher runs the spec for real in its own process group.
type groupLauncher struct{ t *testing.T }

func (l groupLauncher) Launch(_ context.Context, spec adapter.ProcSpec) (adapter.OwnedProc, error) {
	cmd := exec.Command(spec.Path, spec.Args...) // #nosec G204 -- Test launches the proposal's own fixture script.
	cmd.Dir, cmd.Env, cmd.Stdin = spec.Dir, spec.Env, spec.Stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &groupProc{cmd: cmd, stdout: stdout}, nil
}

type groupProc struct {
	cmd    *exec.Cmd
	stdout io.Reader
}

var sysSignals = map[adapter.StopSignal]syscall.Signal{
	adapter.StopInterrupt: syscall.SIGINT,
	adapter.StopTerminate: syscall.SIGTERM,
	adapter.StopKill:      syscall.SIGKILL,
}

func (p *groupProc) Stdout() io.Reader { return p.stdout }
func (p *groupProc) Signal(sig adapter.StopSignal) error {
	return syscall.Kill(-p.cmd.Process.Pid, sysSignals[sig])
}
func (p *groupProc) Wait() adapter.NativeExit {
	err := p.cmd.Wait()
	if ee, ok := err.(*exec.ExitError); ok { //nolint:errorlint // Direct result of cmd.Wait.
		return adapter.NativeExit{Code: ee.ExitCode()}
	}
	return adapter.NativeExit{Err: err}
}

// GroupGone ignores the zombie leader: only a group with no member left,
// zombies included once reaped, counts as gone.
func (p *groupProc) GroupGone() bool {
	return syscall.Kill(-p.cmd.Process.Pid, 0) == syscall.ESRCH
}

func TestStopLadderEscalates(t *testing.T) { // AC-5.1, AC-5.2
	in := prepareInput(t)
	ready := filepath.Join(t.TempDir(), "ready")
	in.Probe = writeNative(t, "#!/bin/sh\ntrap '' INT TERM\n: > "+ready+"\nwhile :; do sleep 0.05; done\n")
	a := testAdapter()
	lp, err := a.Prepare(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	const grace = 300 * time.Millisecond
	lp.StopLadder = []adapter.StopStep{
		{Signal: adapter.StopInterrupt, Grace: grace},
		{Signal: adapter.StopTerminate, Grace: grace},
		{Signal: adapter.StopKill, Grace: 2 * time.Second},
	}
	s, err := a.Start(context.Background(), lp, groupLauncher{t})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture never installed its signal traps")
		}
		time.Sleep(10 * time.Millisecond)
	}
	start := time.Now()
	rep := s.Interrupt(context.Background())
	elapsed := time.Since(start)

	want := []adapter.StopSignal{adapter.StopInterrupt, adapter.StopTerminate, adapter.StopKill}
	if !slices.Equal(rep.Sent, want) {
		t.Fatalf("sent = %v, want %v (a fixture ignoring INT and TERM)", rep.Sent, want)
	}
	if elapsed < 2*grace {
		t.Fatalf("ladder finished in %v, before both graces (%v) elapsed", elapsed, 2*grace)
	}
	if !rep.Confirmed {
		t.Fatalf("report = %+v, want the group confirmed gone", rep)
	}
	if rep.Acknowledged != "unknown" {
		t.Fatalf("acknowledged = %q, want unknown", rep.Acknowledged)
	}
	drain(t, s)
}
