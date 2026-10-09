//go:build linux || darwin

package control

import (
	"os/exec"
	"syscall"
)

// DefaultTransport is this platform's control transport.
func DefaultTransport() (Transport, error) { return NewUnixTransport(), nil }

// detach starts cmd in its own session with no terminal, so the client's
// exit does not take the supervisor with it (I06).
func detach(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return nil
}
