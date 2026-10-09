//go:build unix

package workers

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestHashFileRejectsFIFO pins the hash open against a FIFO swapped in after
// admission: it must fail fast, never hang. Identity precedes verification,
// so a hang here would hold the run to its execution deadline.
func TestHashFileRejectsFIFO(t *testing.T) {
	t.Parallel()
	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := hashFile(fifo)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("hashFile(fifo) succeeded; want a fast rejection")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("hashFile(fifo) hung; want a fast rejection")
	}
}
