//go:build !linux

package workers

import (
	"fmt"
	"os"
	"runtime"
	"syscall"

	"github.com/turbokast/mythhelm/internal/contain"
)

// containAttr refuses: v1 qualifies the Linux boundary only (design D4).
func containAttr() (*syscall.SysProcAttr, error) {
	return nil, fmt.Errorf("contained launch on %s: %w", runtime.GOOS, contain.ErrUnsupported)
}

// dupFile is unreachable off Linux: containAttr refuses first.
func dupFile(*os.File) (*os.File, error) { return nil, contain.ErrUnsupported }
