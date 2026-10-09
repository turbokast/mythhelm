//go:build !linux

package contain

import (
	"runtime"
	"syscall"
)

// ProbeLinux reports the missing capability: the boundary needs Linux user and
// mount namespaces.
func ProbeLinux() Availability {
	return Availability{Reason: "mythhelm-restricted/1 needs Linux user and mount namespaces; this OS is " + runtime.GOOS}
}

// EnterLinux always refuses off Linux.
func EnterLinux(ContainSpec) error { return ErrUnsupported }

// RunProbeChild has no probe to run off Linux.
func RunProbeChild() int { return 1 }

func nsSysProcAttr() *syscall.SysProcAttr { return nil }
