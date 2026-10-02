//go:build unix

package workers

import (
	"errors"
	"fmt"
	"io/fs"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/turbokast/mythhelm/internal/adapter"
)

// procGone reports whether err means "no such process": ENOENT (already
// reaped when looked up) or ESRCH (reaped mid-lookup: on Linux a worker
// reaped between the open and the read of /proc/<pid>/stat surfaces ESRCH
// on the read). It checks both syscall and x/sys errnos, which are distinct
// types. Anything else stays fail-closed: undeterminable is not gone.
func procGone(err error) bool {
	return errors.Is(err, fs.ErrNotExist) ||
		errors.Is(err, syscall.ESRCH) ||
		errors.Is(err, unix.ESRCH)
}

// detachedAttr puts the worker in a new session, so a terminal hangup or a
// signal to the CLI's process group does not reach it (AC-5.2).
func detachedAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// nativeAttr makes the native process the leader of its own process group,
// which the stop ladder signals as a whole.
func nativeAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func nativeGroup(pid int) *int { return &pid }

var unixSignals = map[adapter.StopSignal]unix.Signal{
	adapter.StopInterrupt: unix.SIGINT,
	adapter.StopTerminate: unix.SIGTERM,
	adapter.StopKill:      unix.SIGKILL,
}

// Signal delivers sig to every process in the native's group. A group that
// is already gone is not an error.
func (p *ownedProc) Signal(sig adapter.StopSignal) error {
	s, ok := unixSignals[sig]
	if !ok {
		return fmt.Errorf("unknown stop signal %q", sig)
	}
	if err := unix.Kill(-*p.pgid, s); err != nil && !errors.Is(err, unix.ESRCH) {
		return err
	}
	return nil
}

// GroupGone is the stop confirmation: kill(-pgid, 0) fails with ESRCH only
// once no process of the group remains, zombies included.
func (p *ownedProc) GroupGone() bool {
	return errors.Is(unix.Kill(-*p.pgid, 0), unix.ESRCH)
}
