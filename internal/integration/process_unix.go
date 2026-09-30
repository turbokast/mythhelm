//go:build !windows

package integration

import (
	"os/exec"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd)  { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func killProcessGroup(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
