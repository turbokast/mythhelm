package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/workers"
)

// watchPipeline builds the minimal pipeline a watch test drives: journal,
// producer, decision identity and discarding hooks.
func watchPipeline(runID, attemptID string, j *journal.Journal, prod *Producer) *pipeline {
	return &pipeline{
		d:    admission.Decision{RunID: runID, AttemptID: attemptID},
		h:    Hooks{Event: func(journal.Event) {}, Notice: func(string) {}},
		j:    j,
		prod: prod,
	}
}

// appendSpoolLine appends one journal.Event envelope line to the attempt's
// spool.
func appendSpoolLine(t *testing.T, dir string, ev journal.Event) {
	t.Helper()
	line, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	spool, err := os.OpenFile(filepath.Join(dir, "spool.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600) //nolint:gosec // G304: fixed spool name in a test temp dir
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = spool.Close() }()
	if _, err := spool.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}
}

func spoolProgress(runID, attemptID string, seq int64) journal.Event {
	return journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: "evt_spool_progress",
		RunID: runID, AttemptID: attemptID, ProducerID: "wrk_" + attemptID,
		ProducerSequence: seq, Generation: 1, ObservedAt: time.Now().UTC(),
		Type: "attempt.progress", Payload: json.RawMessage(`{"assistant_turns":1,"tool_uses":{},"retries":0}`),
	}
}

// TestWatchWritesSupervisorBeatAfterIngest pins the watch-tick beat: every
// successful Ingest writes supervisor.beat with the boot generation and
// an increasing counter.
func TestWatchWritesSupervisorBeatAfterIngest(t *testing.T) {
	// Serial: holdLockFor isolates the runtime dir through the environment.
	stateDir := t.TempDir()
	wantGen := holdLockFor(t, stateDir)
	j := openPinJournal(t, stateDir)
	prod := NewProducer(ids.New("sup"), 1)
	runID := newRun(t, j, prod)
	attemptID := newAttempt(t, j, prod, runID)
	dir := workers.AttemptDir(stateDir, runID, attemptID)
	appendSpoolLine(t, dir, spoolProgress(runID, attemptID, 1))

	p := watchPipeline(runID, attemptID, j, prod)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	ref := AttemptRef{StateDir: stateDir, RunID: runID, AttemptID: attemptID}
	go func() { done <- p.watch(ctx, ref, nil) }()

	// Poll for the second beat: repetition proves the tick, not a single write.
	deadline := time.Now().Add(30 * time.Second)
	var beat control.SupervisorBeat
	for {
		raw, err := os.ReadFile(filepath.Join(dir, "supervisor.beat")) //nolint:gosec // G304: fixed beat name in a test temp dir
		if err == nil {
			if uerr := json.Unmarshal(raw, &beat); uerr != nil {
				cancel()
				t.Fatalf("supervisor.beat is not a SupervisorBeat: %v", uerr)
			}
			if beat.Beat >= 2 {
				break
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			cancel()
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("timed out waiting for the second supervisor beat (last %+v)", beat)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if beat.Generation != wantGen {
		t.Errorf("beat generation = %d, want the boot generation %d", beat.Generation, wantGen)
	}
	at, err := time.Parse(time.RFC3339Nano, beat.At)
	if err != nil {
		t.Errorf("beat at = %q, want RFC3339Nano: %v", beat.At, err)
	} else if age := time.Since(at); age < 0 || age > time.Minute {
		t.Errorf("beat age = %s, want a fresh beat", age)
	}
	// The worker reads what the watch writes.
	if _, gen, ok := workers.ReadSupervisorBeat(dir); !ok || gen != wantGen {
		t.Errorf("workers.ReadSupervisorBeat = gen %d ok %v, want gen %d ok true", gen, ok, wantGen)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("watch = %v, want context.Canceled", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("watch did not return after cancel")
	}
}

// TestWatchWritesNoBeatWhenIngestFails pins "after Ingest only": a spool
// the watch cannot ingest writes no beat at all.
func TestWatchWritesNoBeatWhenIngestFails(t *testing.T) {
	// Serial: holdLockFor isolates the runtime dir through the environment.
	stateDir := t.TempDir()
	holdLockFor(t, stateDir)
	j := openPinJournal(t, stateDir)
	prod := NewProducer(ids.New("sup"), 1)
	runID := newRun(t, j, prod)
	attemptID := newAttempt(t, j, prod, runID)
	// The run is executing by the time a watch runs: walk it there
	// through production transitions so the ingest_failed path can
	// interrupt it.
	for _, to := range []RunState{RunAdmission, RunExecuting} {
		if err := TransitionRun(context.Background(), j, runID, to, "", prod); err != nil {
			t.Fatal(err)
		}
	}
	dir := workers.AttemptDir(stateDir, runID, attemptID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spool.jsonl"), []byte("{corrupt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := watchPipeline(runID, attemptID, j, prod)
	ref := AttemptRef{StateDir: stateDir, RunID: runID, AttemptID: attemptID}
	if err := p.watch(context.Background(), ref, nil); err != nil {
		t.Fatalf("watch on a corrupt spool = %v, want the ingest_failed path", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "supervisor.beat")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("supervisor.beat stat = %v, want no beat without a successful Ingest", err)
	}
}
