//go:build unix

package fake

import (
	"os"
	"syscall"
	"time"
)

// spawnEscapee starts a child that leaves the process group and session
// (setsid), as a descendant escaping the stop ladder would. It inherits the
// environment, including the MYTHHELM_ATTEMPT_ID marker, and lingers.
func spawnEscapee(linger time.Duration) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() { _ = null.Close() }()
	p, err := os.StartProcess(exe, []string{exe, AgentCommand, "--linger", linger.String()}, &os.ProcAttr{
		Env:   os.Environ(),
		Files: []*os.File{null, null, null},
		Sys:   &syscall.SysProcAttr{Setsid: true},
	})
	if err != nil {
		return err
	}
	return p.Release()
}
