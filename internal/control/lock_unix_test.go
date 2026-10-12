//go:build unix

package control

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestReadMetadataRereadsTornRecordReturnedPastSettle pins issue #449: a
// read can stall past metadataSettle, so a read started
// within metadataSettle may return a torn record only after the budget has
// elapsed. That record was observed mid-rewrite, not for the whole budget,
// so it must be re-read rather than returned. The lock path is a FIFO whose
// writer stalls past metadataSettle, emits a short record stitched onto a
// long record's tail, and swaps a complete record into the path before
// closing.
func TestReadMetadataRereadsTornRecordReturnedPastSettle(t *testing.T) { // I02
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "supervisor.lock")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	want := lockMetadata{Root: "/r", PID: 1, Generation: 2}
	short, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	complete := filepath.Join(dir, "complete")
	if err := os.WriteFile(complete, append(short, '\n'), 0o600); err != nil {
		t.Fatalf("writing complete record: %v", err)
	}
	torn := append(append(short, '\n'), strings.Repeat("x", 200)...)

	writerErr := make(chan error, 1)
	go func() {
		time.Sleep(metadataSettle + 100*time.Millisecond)
		w, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			writerErr <- err
			return
		}
		if _, err := w.Write(torn); err != nil {
			_ = w.Close()
			writerErr <- err
			return
		}
		if err := os.Rename(complete, path); err != nil {
			_ = w.Close()
			writerErr <- err
			return
		}
		writerErr <- w.Close()
	}()

	type result struct {
		meta lockMetadata
		err  error
	}
	read := make(chan result, 1)
	go func() {
		meta, err := readMetadataErr(path)
		read <- result{meta, err}
	}()
	var got lockMetadata
	select {
	case r := <-read:
		got, err = r.meta, r.err
	case <-time.After(10 * time.Second):
		t.Fatal("readMetadataErr still blocked on the FIFO after 10s; the stalled writer never opened it")
	}
	if werr := <-writerErr; werr != nil {
		t.Fatalf("stalled writer: %v", werr)
	}
	if err != nil {
		t.Fatalf("readMetadataErr after a read straddling the settle deadline: %v, want the completed record", err)
	}
	if got != want {
		t.Fatalf("readMetadataErr = %+v, want %+v", got, want)
	}
}
