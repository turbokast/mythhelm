package cli

import (
	"os"
	"syscall"
)

// Go exposes close/logoff/shutdown notifications as SIGTERM. This is only
// a best-effort cleanup opportunity; Windows can still terminate the process.
func tuiOSSignals() []os.Signal          { return []os.Signal{os.Interrupt, syscall.SIGTERM} }
func tuiTerminalSignal(s os.Signal) bool { return s == syscall.SIGTERM }
