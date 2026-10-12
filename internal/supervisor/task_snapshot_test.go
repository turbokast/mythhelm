package supervisor

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/turbokast/mythhelm/internal/admission"
)

// A reader that polls the run directory while snapshot writes the task (the
// live run viewer, which digest-checks task.md) must see the file absent or
// complete, never partial (I07).
func TestSnapshotNeverExposesPartialTask(t *testing.T) {
	t.Parallel()
	runDir, workdir := t.TempDir(), t.TempDir()
	content := bytes.Repeat([]byte("0123456789abcdef"), 2<<20)
	p := &pipeline{d: admission.Decision{
		RunDir: runDir, Workdir: workdir, Task: admission.Task{Content: content},
	}}
	path := filepath.Join(runDir, taskFile)

	var partial atomic.Int64
	started, done := make(chan struct{}), make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		close(started)
		for {
			select {
			case <-done:
				return
			default:
			}
			if fi, err := os.Stat(path); err == nil && fi.Size() != int64(len(content)) {
				partial.Store(fi.Size() + 1)
				return
			}
			runtime.Gosched()
		}
	})
	<-started

	// The clone destination exists, so snapshot fails after the task write.
	err := p.snapshot(t.Context())
	close(done)
	wg.Wait()
	if err == nil {
		t.Fatal("snapshot succeeded; expected the existing clone destination to refuse")
	}
	if n := partial.Load(); n != 0 {
		t.Fatalf("task.md was visible with %d bytes, want absent or all %d", n-1, len(content))
	}
	got, err := os.ReadFile(path) // #nosec G304 -- path is under the test's temp dir
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("task.md is %d bytes, want the admitted %d", len(got), len(content))
	}
	entries, err := os.ReadDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("run dir holds %v, want only %s (no leftover temp file)", names, taskFile)
	}
}

func TestSnapshotReportsTaskWriteFailure(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "gone")
	p := &pipeline{d: admission.Decision{RunDir: missing, Task: admission.Task{Content: []byte("# t\n")}}}
	err := p.snapshot(t.Context())
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot error = %v, want one wrapping os.ErrNotExist", err)
	}
}
