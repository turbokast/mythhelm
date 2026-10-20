//go:build unix

package cli

import (
	"os"
	"syscall"
)

func tuiOSSignals() []os.Signal          { return []os.Signal{os.Interrupt, syscall.SIGHUP, syscall.SIGTERM} }
func tuiTerminalSignal(s os.Signal) bool { return s == syscall.SIGHUP || s == syscall.SIGTERM }
