package supervisor_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
)

// stoppedWithoutTerminal retains a real worker's complete stopped event but
// not its following terminal state: the durable boundary between those writes.
func stoppedWithoutTerminal(t *testing.T, f fixture, d admission.Decision) map[string]any {
	t.Helper()
	path := filepath.Join(workers.AttemptDir(f.state, d.RunID, d.AttemptID), supervisor.SpoolFile)
	raw, err := os.ReadFile(path) // #nosec G304 -- test-owned temporary spool
	if err != nil {
		t.Fatal(err)
	}
	var prefix []byte
	var stopped map[string]any
	for line := range bytes.SplitSeq(raw, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var ev journal.Event
		if err := json.Unmarshal(line, &ev); err != nil {
			t.Fatal(err)
		}
		prefix = append(prefix, line...)
		prefix = append(prefix, '\n')
		if ev.Type == "attempt.stopped" {
			if err := json.Unmarshal(ev.Payload, &stopped); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if stopped == nil {
		t.Fatal("real worker emitted no complete stop")
	}
	if err := os.WriteFile(path, prefix, 0600); err != nil {
		t.Fatal(err)
	}
	return stopped
}

func appendRecoveryStop(t *testing.T, f fixture, d admission.Decision, payload map[string]any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.journal(t).Append(t.Context(), journal.Event{SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"), RunID: d.RunID, AttemptID: d.AttemptID, ProducerID: ids.New("fixture"), ProducerSequence: 1, Generation: 1, ObservedAt: time.Now().UTC(), Type: "attempt.stopped", Payload: raw}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRecoverConfirmedEmptyStop(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("quarantine resolution requires the existing Linux orphan scanner")
	}
	for _, tc := range []struct {
		name  string
		alter func(map[string]any)
	}{
		{name: "complete original stop"},
		{name: "missing confirmation", alter: func(s map[string]any) { delete(s, "confirmed") }},
		{name: "missing scan", alter: func(s map[string]any) { delete(s, "descendant_scan") }},
		{name: "missing PIDs", alter: func(s map[string]any) { delete(s, "unresolved_pids") }},
		{name: "missing identities", alter: func(s map[string]any) { delete(s, "unresolved_identities") }},
		{name: "null PIDs", alter: func(s map[string]any) { s["unresolved_pids"] = nil }},
		{name: "unconfirmed", alter: func(s map[string]any) { s["confirmed"] = false }},
		{name: "failed scan", alter: func(s map[string]any) { s["descendant_scan"] = "failed" }},
		{name: "unknown scan", alter: func(s map[string]any) { s["descendant_scan"] = "unknown" }},
		{name: "inconsistent identity", alter: func(s map[string]any) {
			s["unresolved_identities"] = []any{map[string]any{"pid": 123, "start_time": "2020-01-01T00:00:00Z"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			d, _, wait := seedLaunch(t, f, "happy")
			wait()
			stopped := stoppedWithoutTerminal(t, f, d)
			pids, ok := stopped["unresolved_pids"].([]any)
			if stopped["confirmed"] != true || stopped["descendant_scan"] != "proc-environ" || !ok || len(pids) != 0 {
				t.Fatalf("fixture stop %#v", stopped)
			}
			if code, _, stderr := f.run(t, "recover", d.RunID); code != 6 {
				t.Fatalf("first recovery exit %d: %s", code, stderr)
			}
			a, err := f.journal(t).LatestAttempt(t.Context(), d.RunID)
			if err != nil || a.State != "quarantined" {
				t.Fatalf("first recovery attempt %+v: %v", a, err)
			}
			if tc.alter != nil {
				tc.alter(stopped)
				appendRecoveryStop(t, f, d, stopped)
			}
			code, _, stderr := f.run(t, "recover", d.RunID)
			if tc.alter != nil {
				if code != 6 {
					t.Fatalf("incomplete latest report resolved: exit %d: %s", code, stderr)
				}
				if _, err := f.journal(t).Candidate(t.Context(), d.AttemptID); !errors.Is(err, journal.ErrNotFound) {
					t.Fatalf("incomplete report froze candidate: %v", err)
				}
				for _, ev := range f.events(t, d.RunID) {
					if ev.Type == "recovery.ownership_resolved" {
						t.Fatal("incomplete report resolved ownership")
					}
				}
				return
			}
			if code != 4 {
				t.Fatalf("confirmed empty stop recovery exit %d: %s; want recovered partial", code, stderr)
			}
			c, err := f.journal(t).Candidate(t.Context(), d.AttemptID)
			if err != nil || !c.Partial {
				t.Fatalf("partial candidate %+v: %v", c, err)
			}
			if got := f.onlyRun(t); got.State != "failed" || got.Reason != "recovered_partial" {
				t.Fatalf("recovered run %+v", got)
			}
			counts := map[string]int{}
			for _, ev := range f.events(t, d.RunID) {
				counts[ev.Type]++
			}
			if counts["attempt.launched"] != 1 || counts["candidate.frozen"] != 1 || counts["recovery.ownership_resolved"] != 1 {
				t.Fatalf("recovery replayed or omitted effects: %v", counts)
			}
		})
	}
}

// A historical empty report cannot replace a fresh scan. The test owns the
// scripted escaped writer and checks its recorded identity before cleanup.
func TestRecoverEmptyStopStillScansForWriters(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("quarantine resolution requires the existing Linux orphan scanner")
	}
	f := newFixture(t)
	d, _, wait := seedLaunch(t, f, "escapee")
	wait()
	stopped := stoppedWithoutTerminal(t, f, d)
	raw, err := json.Marshal(stopped["unresolved_identities"])
	if err != nil {
		t.Fatal(err)
	}
	var identities []workers.ProcessIdentity
	if err := json.Unmarshal(raw, &identities); err != nil {
		t.Fatal(err)
	}
	if len(identities) != 1 {
		t.Fatalf("escaped writer identities: %+v", identities)
	}
	escaped := identities[0]
	t.Cleanup(func() {
		start, err := workers.ProcessStartTime(escaped.PID)
		if errors.Is(err, workers.ErrNoProcess) {
			return
		}
		if err != nil || !start.Equal(escaped.StartTime) {
			t.Errorf("cleanup identity changed: %v", err)
			return
		}
		proc, err := os.FindProcess(escaped.PID)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = proc.Release() }()
		if err := proc.Kill(); err != nil {
			t.Error(err)
			return
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			if _, err := workers.ProcessStartTime(escaped.PID); errors.Is(err, workers.ErrNoProcess) {
				return
			}
			if time.Now().After(deadline) {
				t.Error("test-owned escaped writer did not exit")
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	})
	if code, _, stderr := f.run(t, "recover", d.RunID); code != 6 {
		t.Fatalf("initial quarantine exit %d: %s", code, stderr)
	}
	// Independent negative: present complete empty historical evidence while a
	// real marker-bearing writer is still observable in the current scan.
	stopped["unresolved_pids"] = []int{}
	stopped["unresolved_identities"] = []workers.ProcessIdentity{}
	appendRecoveryStop(t, f, d, stopped)
	marked, err := workers.OrphanPIDs(d.AttemptID)
	if err != nil || len(marked) != 1 || marked[0] != escaped.PID {
		t.Fatalf("fresh marked writer %v: %v", marked, err)
	}
	if code, _, stderr := f.run(t, "recover", d.RunID); code != 6 {
		t.Fatalf("live writer resolved exit %d: %s", code, stderr)
	}
	start, err := workers.ProcessStartTime(escaped.PID)
	if err != nil || !start.Equal(escaped.StartTime) {
		t.Fatalf("recovery disturbed escaped writer: %v", err)
	}
	if _, err := f.journal(t).Candidate(t.Context(), d.AttemptID); !errors.Is(err, journal.ErrNotFound) {
		t.Fatalf("live writer froze candidate: %v", err)
	}
	for _, ev := range f.events(t, d.RunID) {
		if ev.Type == "recovery.ownership_resolved" {
			t.Fatal("live writer resolved ownership")
		}
	}
}
