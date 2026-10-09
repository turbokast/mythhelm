//go:build !linux

package workers

import (
	"fmt"
	"runtime"
	"syscall"

	"github.com/turbokast/mythhelm/internal/contain"
)

// containAttr refuses: v1 qualifies the Linux boundary only (design D4).
func containAttr() (*syscall.SysProcAttr, error) {
	return nil, fmt.Errorf("contained launch on %s: %w", runtime.GOOS, contain.ErrUnsupported)
}
