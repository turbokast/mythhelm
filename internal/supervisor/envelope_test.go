package supervisor

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
)

type envelopeWorld struct {
	j     *journal.Journal
	runID string
	prod  *Producer
}

// blockedRun is a run journaled through admission to blocked with reason,
// carrying a stored envelope of 30 minutes, 3 repairs, 2 replans and 5
// transport retries.
func blockedRun(t *testing.T, reason string) envelopeWorld {
	t.Helper()
	j, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	w := envelopeWorld{j: j, runID: ids.New("run"), prod: NewProducer(ids.New("sup"), 1)}
	run := journal.RunRow{RunID: w.runID, AdapterID: "builtin/fake", SourceRepo: "/tmp/repo", TaskSHA256: "t",
		BillingPosture: "local-scripted", ExecutionProfile: "trusted-host"}
	if err := CreateRun(t.Context(), j, run, w.prod); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		to     RunState
		reason string
	}{{RunAdmission, ""}, {RunBlocked, reason}} {
		if err := TransitionRun(t.Context(), j, w.runID, step.to, step.reason, w.prod); err != nil {
			t.Fatal(err)
		}
	}
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.UpsertRunEnvelope(t.Context(), tx, journal.EnvelopeRow{RunID: w.runID, ExecutionSeconds: 1800,
			Repairs: 3, Replans: 2, TransportRetries: 5, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	}); err != nil {
		t.Fatal(err)
	}
	return w
}

func (w envelopeWorld) extensions(t *testing.T) []journal.Event {
	t.Helper()
	evs, err := w.j.Events(t.Context(), w.runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []journal.Event
	for _, ev := range evs {
		if ev.Type == EventExtensionGranted {
			out = append(out, ev)
		}
	}
	return out
}

func TestExtensionGrantsRecordedDecision(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		kind, reason string
		raisedTo     int64
		check        func(journal.EnvelopeRow) int64
	}{
		{"execution", "envelope_deadline_exceeded", 3600, func(e journal.EnvelopeRow) int64 { return e.ExecutionSeconds }},
		{"repairs", "envelope_repairs_exhausted", 6, func(e journal.EnvelopeRow) int64 { return e.Repairs }},
		{"replans", "envelope_replans_exhausted", 4, func(e journal.EnvelopeRow) int64 { return e.Replans }},
		{"transport_retries", "envelope_transport_retries_exhausted", 9, func(e journal.EnvelopeRow) int64 { return e.TransportRetries }},
	} {
		t.Run(c.kind, func(t *testing.T) {
			t.Parallel()
			w := blockedRun(t, c.reason)
			before, err := w.j.RunEnvelope(t.Context(), w.runID)
			if err != nil {
				t.Fatal(err)
			}
			if err := RequestExtension(t.Context(), w.j, w.runID, c.kind, c.raisedTo, "operator"); err != nil {
				t.Fatal(err)
			}
			evs := w.extensions(t)
			if len(evs) != 1 {
				t.Fatalf("run.extension_granted events = %d, want 1", len(evs))
			}
			var p struct {
				Kind      string `json:"kind"`
				RaisedTo  int64  `json:"raised_to"`
				DecidedBy string `json:"decided_by"`
			}
			if err := json.Unmarshal(evs[0].Payload, &p); err != nil {
				t.Fatal(err)
			}
			if p.Kind != c.kind || p.RaisedTo != c.raisedTo || p.DecidedBy != "operator" {
				t.Fatalf("payload = %+v, want kind %s raised_to %d decided_by operator", p, c.kind, c.raisedTo)
			}
			after, err := w.j.RunEnvelope(t.Context(), w.runID)
			if err != nil {
				t.Fatal(err)
			}
			if got := c.check(after); got != c.raisedTo {
				t.Fatalf("envelope %s = %d, want %d", c.kind, got, c.raisedTo)
			}
			// Only the extended ceiling moves.
			if c.check(before) == c.check(after) {
				t.Fatalf("envelope %s did not change", c.kind)
			}
			after.UpdatedAt, before.UpdatedAt = "", ""
			switch c.kind {
			case "execution":
				after.ExecutionSeconds = before.ExecutionSeconds
			case "repairs":
				after.Repairs = before.Repairs
			case "replans":
				after.Replans = before.Replans
			case "transport_retries":
				after.TransportRetries = before.TransportRetries
			}
			if after != before {
				t.Fatalf("envelope changed beyond %s: %+v -> %+v", c.kind, before, after)
			}
			ok, err := CheckExtension(t.Context(), w.j, w.runID, c.kind)
			if err != nil || !ok {
				t.Fatalf("CheckExtension after grant = %v, %v; want true", ok, err)
			}
		})
	}
}

func TestCheckExtensionFailsClosed(t *testing.T) {
	t.Parallel()
	w := blockedRun(t, "envelope_repairs_exhausted")
	if ok, err := CheckExtension(t.Context(), w.j, w.runID, "repairs"); err != nil || ok {
		t.Fatalf("no event: CheckExtension = %v, %v; want false, nil", ok, err)
	}
	if err := RequestExtension(t.Context(), w.j, w.runID, "repairs", 4, "operator"); err != nil {
		t.Fatal(err)
	}
	// A grant for one kind never covers another.
	if ok, err := CheckExtension(t.Context(), w.j, w.runID, "replans"); err != nil || ok {
		t.Fatalf("other kind: CheckExtension = %v, %v; want false, nil", ok, err)
	}
	// A later block for the same kind needs a fresh decision: the grant
	// binds the block it answered.
	ev, err := newEvent(w.runID, "", "", "run.state_changed", stateChange(string(RunBlocked), "envelope_repairs_exhausted"), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := w.prod.append(t.Context(), w.j, ev, nil); err != nil {
		t.Fatal(err)
	}
	if ok, err := CheckExtension(t.Context(), w.j, w.runID, "repairs"); err != nil || ok {
		t.Fatalf("after a second block: CheckExtension = %v, %v; want false, nil", ok, err)
	}
	// A read failure is an error, never a grant.
	if err := w.j.Close(); err != nil {
		t.Fatal(err)
	}
	if ok, err := CheckExtension(t.Context(), w.j, w.runID, "repairs"); err == nil || ok {
		t.Fatalf("closed journal: CheckExtension = %v, %v; want an error and false", ok, err)
	}
}

func TestExtensionRejectsUnknownKind(t *testing.T) {
	t.Parallel()
	w := blockedRun(t, "envelope_repairs_exhausted")
	for name, c := range map[string]struct {
		kind      string
		raisedTo  int64
		decidedBy string
		want      string
	}{
		"unknown kind":           {"budget", 9, "operator", "unknown extension kind"},
		"empty kind":             {"", 9, "operator", "unknown extension kind"},
		"other kind not blocked": {"replans", 9, "operator", "not blocked for replans"},
		"not above the ceiling":  {"repairs", 3, "operator", "must exceed"},
		"lower than the ceiling": {"repairs", 1, "operator", "must exceed"},
		"no decider":             {"repairs", 9, "", "decided_by"},
	} {
		err := RequestExtension(t.Context(), w.j, w.runID, c.kind, c.raisedTo, c.decidedBy)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, c.want)
		}
	}
	if got := len(w.extensions(t)); got != 0 {
		t.Fatalf("rejected requests journaled %d events", got)
	}
	if env, err := w.j.RunEnvelope(t.Context(), w.runID); err != nil || env.Repairs != 3 {
		t.Fatalf("envelope after rejections = %+v, %v; want repairs still 3", env, err)
	}

	// A run that is not blocked at all cannot be extended.
	j, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	prod := NewProducer(ids.New("sup"), 1)
	runID := ids.New("run")
	if err := CreateRun(t.Context(), j, journal.RunRow{RunID: runID, AdapterID: "builtin/fake", SourceRepo: "/tmp/repo", TaskSHA256: "t",
		BillingPosture: "local-scripted", ExecutionProfile: "trusted-host"}, prod); err != nil {
		t.Fatal(err)
	}
	if err := RequestExtension(t.Context(), j, runID, "repairs", 9, "operator"); err == nil || !strings.Contains(err.Error(), "not blocked") {
		t.Fatalf("unblocked run: err = %v, want a not blocked error", err)
	}
}
