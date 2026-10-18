package workers

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/turbokast/mythhelm/internal/adapter"
)

// descendantScan remains unavailable until Windows ownership Task 4 wires
// job-wide stop confirmation and descendant enumeration into the worker.
const descendantScan = "unavailable"

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

// Signal supports only StopKill (Process.Kill); the Windows stop ladder has
// no other rung.
func (p *ownedProc) Signal(sig adapter.StopSignal) error {
	if sig != adapter.StopKill {
		return fmt.Errorf("stop signal %q is not supported on Windows", sig)
	}
	if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

// GroupGone reports whether the native process has been waited for.
func (p *ownedProc) GroupGone() bool { return p.waited.Load() }

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
