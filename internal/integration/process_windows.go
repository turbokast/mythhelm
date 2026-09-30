//go:build windows

package integration

import (
	"os/exec"
	"strconv"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func killProcessGroup(cmd *exec.Cmd) {
	// taskkill /T terminates children as well as the direct check process.
	if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run(); err != nil { // #nosec G204 -- fixed system tool and numeric pid
		_ = cmd.Process.Kill()
	}
}
