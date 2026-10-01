package workers

import (
	"errors"
	"runtime"
	"testing"
)

// The orphan scan either runs for real or reports unsupported; it never
// reports an empty result without looking. Callers that only print PIDs
// ignore the error, while ownership decisions fail closed on it.
func TestOrphanPIDsContract(t *testing.T) {
	t.Parallel()
	pids, err := OrphanPIDs("orphan-contract-probe-nonexistent")
	if err != nil {
		if !errors.Is(err, errors.ErrUnsupported) {
			t.Fatalf("OrphanPIDs error = %v, want ErrUnsupported", err)
		}
		return
	}
	if runtime.GOOS != "linux" {
		t.Fatal("only the Linux scanner may return a scanned result")
	}
	if len(pids) != 0 {
		t.Fatalf("unmatched marker scan = %v, want empty", pids)
	}
}
