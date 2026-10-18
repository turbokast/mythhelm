package workers

import (
	"errors"
	"fmt"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/turbokast/mythhelm/internal/adapter"
)

const descendantScan = "job-object"

// detachedAttr gives the worker its own process group and no console, so a
// console Ctrl-C or close aimed at the CLI does not reach it.
func detachedAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
}

// nativeAttr creates the native suspended, in a new process group without a
// console window. The launch hook assigns the owned job before resuming it.
func nativeAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW | windows.CREATE_SUSPENDED}
}

// nativeGroup is nil: Windows ownership uses a Job Object, not a Unix PGID.
func nativeGroup(int) *int { return nil }

// Signal terminates the owned job; Windows has no qualified graceful rung.
func (p *ownedProc) Signal(sig adapter.StopSignal) error {
	if sig != adapter.StopKill {
		return fmt.Errorf("stop signal %q is not supported on Windows", sig)
	}
	if p.job == nil {
		return errors.New("native has no owned job")
	}
	terminate := p.jobOps.terminate
	if terminate == nil {
		terminate = (*jobObject).terminate
	}
	return terminate(p.job)
}

// GroupGone requires the native to be reaped and an observed empty job.
func (p *ownedProc) GroupGone() bool {
	if !p.waited.Load() || p.job == nil {
		return false
	}
	members, err := p.jobMembers()
	return err == nil && len(members) == 0
}

func (p *ownedProc) jobMembers() ([]ProcessIdentity, error) {
	members := p.jobOps.members
	if members == nil {
		members = (*jobObject).members
	}
	return members(p.job)
}

func descendants(p *ownedProc, _ string) ([]int, []ProcessIdentity, error) {
	if p == nil || p.job == nil {
		return []int{}, []ProcessIdentity{}, nil
	}
	identities, err := p.jobMembers()
	if err != nil {
		return nil, nil, err
	}
	pids := make([]int, 0, len(identities))
	for _, id := range identities {
		pids = append(pids, id.PID)
	}
	if identities == nil {
		identities = []ProcessIdentity{}
	}
	return pids, identities, nil
}

// ProcessStartTime returns pid's creation time (GetProcessTimes).
func ProcessStartTime(pid int) (time.Time, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)) //nolint:gosec // G115: PIDs fit in 32 bits on Windows
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return time.Time{}, fmt.Errorf("%w: pid %d", ErrNoProcess, pid)
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("pid %d: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return time.Time{}, fmt.Errorf("pid %d: %w", pid, err)
	}
	return time.Unix(0, created.Nanoseconds()).UTC(), nil
}

func markedPIDs(string) ([]int, error) {
	return nil, errors.ErrUnsupported
}
