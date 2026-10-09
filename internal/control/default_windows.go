package control

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// DefaultTransport is this platform's control transport.
func DefaultTransport() (Transport, error) { return NewPipeTransport() }

// detach starts cmd with no console, in its own process group, so the
// client's exit or Ctrl+C does not take the supervisor with it (I06).
func detach(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}
	return nil
}
