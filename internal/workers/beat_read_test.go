package workers

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

// beatReadFake returns a scripted sequence of read results and counts the
// reads; the last result repeats.
type beatReadFake struct {
	results []error
	body    []byte
	calls   int
}

func (f *beatReadFake) read(string) ([]byte, error) {
	i := min(f.calls, len(f.results)-1)
	f.calls++
	if f.results[i] != nil {
		return nil, f.results[i]
	}
	return f.body, nil
}

func beatBody(t *testing.T, at time.Time, generation int64) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"beat": 1, "at": at.UTC().Format(time.RFC3339Nano), "generation": generation})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

var errBeatSharing = errors.New("open supervisor.beat: sharing violation")

func TestReadBeatRetriesTransientReadErrors(t *testing.T) {
	t.Parallel()
	at := time.Now().UTC().Truncate(time.Microsecond)
	transient := func(err error) bool { return errors.Is(err, errBeatSharing) }

	t.Run("a sharing violation twice then a beat reads the beat", func(t *testing.T) {
		t.Parallel()
		f := &beatReadFake{results: []error{errBeatSharing, errBeatSharing, nil}, body: beatBody(t, at, 4)}
		gotAt, gen, ok := readBeat("supervisor.beat", f.read, transient, 5, time.Millisecond)
		if !ok || gen != 4 || !gotAt.Equal(at) {
			t.Fatalf("readBeat = (%s, %d, %v) after %d reads, want the generation-4 beat", gotAt, gen, ok, f.calls)
		}
		if f.calls != 3 {
			t.Fatalf("reads = %d, want 3", f.calls)
		}
	})
	t.Run("a persistent sharing violation reads stale after the budget", func(t *testing.T) {
		t.Parallel()
		f := &beatReadFake{results: []error{errBeatSharing}}
		if _, _, ok := readBeat("supervisor.beat", f.read, transient, 5, time.Millisecond); ok {
			t.Fatal("a persistent sharing violation reads ok, want stale (fail closed)")
		}
		if f.calls != 6 {
			t.Fatalf("reads = %d, want 6 (the first plus 5 retries)", f.calls)
		}
	})
	t.Run("a permanent read error reads stale without retrying", func(t *testing.T) {
		t.Parallel()
		f := &beatReadFake{results: []error{os.ErrNotExist}}
		if _, _, ok := readBeat("supervisor.beat", f.read, transient, 5, time.Millisecond); ok {
			t.Fatal("a missing beat reads ok, want stale")
		}
		if f.calls != 1 {
			t.Fatalf("reads = %d, want 1 (a permanent error is not retried)", f.calls)
		}
	})
}
