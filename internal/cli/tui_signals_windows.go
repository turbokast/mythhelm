package cli

import "os"

func tuiOSSignals() []os.Signal        { return []os.Signal{os.Interrupt} }
func tuiTerminalSignal(os.Signal) bool { return false }
