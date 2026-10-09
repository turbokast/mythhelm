package supervisor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
)

type ledgerFixture struct {
	j     *journal.Journal
	runID string
	seq   map[string]int64
}

func newLedgerFixture(t *testing.T) *ledgerFixture {
	t.Helper()
	j, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	f := &ledgerFixture{j: j, runID: ids.New("run"), seq: map[string]int64{}}
	now := time.Now()
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.InsertRun(t.Context(), tx, journal.RunRow{RunID: f.runID, State: "created", AdapterID: "builtin/fake", SourceRepo: "/tmp/repo",
			TaskSHA256: "t", BillingPosture: "local-scripted", ExecutionProfile: "trusted-host", CreatedAt: now, UpdatedAt: now})
	}); err != nil {
		t.Fatal(err)
	}
	return f
}

// ingest appends one event of type typ from producer through the projection
// the supervisor uses for spooled worker events.
func (f *ledgerFixture) event(producer, typ, payload string) journal.Event {
	f.seq[producer]++
	return journal.Event{SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"), RunID: f.runID, ProducerID: producer,
		ProducerSequence: f.seq[producer], Generation: 1, ObservedAt: time.Now().UTC(), Type: typ, Payload: json.RawMessage(payload)}
}

func (f *ledgerFixture) append(t *testing.T, ev journal.Event) {
	t.Helper()
	err := f.j.Append(t.Context(), ev, func(tx *sql.Tx) error { return projectWorkerEvent(t.Context(), tx, ev) })
	if err != nil {
		t.Fatalf("Append %s: %v", ev.Type, err)
	}
}

func (f *ledgerFixture) rows(t *testing.T) []journal.UsageRow {
	t.Helper()
	rows, err := f.j.UsageObservations(t.Context(), f.runID)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func nativeResult(usage, cost string) string {
	return fmt.Sprintf(`{"result_observed":true,"usage_native_reported":%s,"retail_equivalent_estimate_usd":%s}`, usage, cost)
}

func rowFor(t *testing.T, rows []journal.UsageRow, scope, unit string) journal.UsageRow {
	t.Helper()
	var found []journal.UsageRow
	for _, r := range rows {
		if r.Scope == scope && r.Unit == unit {
			found = append(found, r)
		}
	}
	if len(found) != 1 {
		t.Fatalf("rows for %s/%s = %+v, want exactly one in %+v", scope, unit, found, rows)
	}
	return found[0]
}

func TestNativeResultProjectsUsageRows(t *testing.T) { // I09 (v2 §7.3)
	f := newLedgerFixture(t)
	f.append(t, f.event("sup_x", "admission.decided", `{"adapter":{"harness":"claude-code"}}`))
	ev := f.event("wrk_a", "attempt.native_result",
		nativeResult(`{"claude-synthetic":{"input":100,"output":50,"cache_read":10,"cache_creation":5}}`, `"0.0123"`))
	f.append(t, ev)

	rows := f.rows(t)
	if len(rows) != 2 {
		t.Fatalf("got %d rows %+v, want one token row and one retail row", len(rows), rows)
	}
	tok := rowFor(t, rows, "claude-synthetic", "tokens")
	if tok.Label != "reported" || tok.Quantity != "165" || tok.Source != "native-reported" ||
		tok.ProducerID != "wrk_a" || tok.ProducerSequence != ev.ProducerSequence {
		t.Errorf("token row = %+v", tok)
	}
	usd := rowFor(t, rows, "retail-equivalent", "USD")
	if usd.Label != "estimated" || usd.Quantity != "0.0123" || usd.ProducerID != "wrk_a" || usd.ProducerSequence != ev.ProducerSequence {
		t.Errorf("retail row = %+v", usd)
	}

	// An identity-less delta handed straight to the writer keeps NULL identity.
	err := f.j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.InsertUsageObservation(t.Context(), tx, journal.UsageRow{ObservationID: "obs_anon", RunID: f.runID, Scope: "claude-synthetic",
			Unit: "tokens", Source: "native-reported", Label: "estimated", Quantity: "4", ObservedAt: "2026-01-01T00:00:00Z"})
	})
	if err != nil {
		t.Fatal(err)
	}
	anon := f.rows(t)[2]
	if anon.ObservationID != "obs_anon" || anon.ProducerID != "" || anon.ProducerSequence != 0 {
		t.Errorf("identity-less row = %+v, want empty producer", anon)
	}
}

func TestNativeResultAttemptsAccumulate(t *testing.T) {
	f := newLedgerFixture(t)
	f.append(t, f.event("wrk_a", "attempt.native_result", nativeResult(`{"m":{"input":1,"output":2,"cache_read":3,"cache_creation":4}}`, `"0.10"`)))
	f.append(t, f.event("wrk_b", "attempt.native_result", nativeResult(`{"m":{"input":10,"output":20,"cache_read":30,"cache_creation":40}}`, `"0.27"`)))

	rows := f.rows(t)
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want an increment row per attempt and scope: %+v", len(rows), rows)
	}
	var readings []billing.Reading
	for i, r := range rows {
		readings = append(readings, billing.Reading{Scope: r.Scope, Unit: r.Unit, Source: r.Source, At: time.Unix(int64(i), 0),
			Delta: &rows[i].Quantity, Producer: r.ProducerID, Sequence: r.ProducerSequence, HasIdentity: true})
	}
	n, err := billing.Normalize(readings, nil, nil) //nolint:misspell // the spec names billing.Normalize
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[billing.ScopeKey]string{
		{Scope: "m", Unit: "tokens", Source: "native-reported"}:              "110",
		{Scope: "retail-equivalent", Unit: "USD", Source: "native-reported"}: "0.37",
	} {
		if got := n.Scopes[key].Total; got == nil || *got != want {
			t.Errorf("accumulated %+v = %v, want %s", key, got, want)
		}
	}
}

func TestNativeResultReplayIgnored(t *testing.T) {
	f := newLedgerFixture(t)
	ev := f.event("wrk_a", "attempt.native_result", nativeResult(`{"m":{"input":1,"output":2,"cache_read":3,"cache_creation":4}}`, `"0.10"`))
	f.append(t, ev)
	before := len(f.rows(t))
	f.append(t, ev) // same event_id: Append journals nothing and runs no projection
	if got := len(f.rows(t)); got != before || before != 2 {
		t.Fatalf("rows after replay = %d, before = %d, want 2 and 2", got, before)
	}
	evs, err := f.j.Events(t.Context(), f.runID, 0)
	if err != nil || len(evs) != 1 {
		t.Fatalf("journal holds %d events (%v), want 1", len(evs), err)
	}
}

func TestNativeResultRepresentedDeltaDropped(t *testing.T) {
	f := newLedgerFixture(t)
	// A row already carries (wrk_a, 1) although the event itself was never journaled.
	err := f.j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.InsertUsageObservation(t.Context(), tx, journal.UsageRow{ObservationID: "obs_prior", RunID: f.runID, Scope: "m", Unit: "tokens",
			Source: "native-reported", Label: "reported", Quantity: "10", ProducerID: "wrk_a", ProducerSequence: 1, ObservedAt: "2026-01-01T00:00:00Z"})
	})
	if err != nil {
		t.Fatal(err)
	}
	f.append(t, f.event("wrk_a", "attempt.native_result", nativeResult(`{"m":{"input":1,"output":2,"cache_read":3,"cache_creation":4}}`, `null`)))
	rows := f.rows(t)
	if got := rowFor(t, rows, "m", "tokens"); got.ObservationID != "obs_prior" {
		t.Fatalf("token rows = %+v, want only the prior row", rows)
	}
}

func TestNativeResultNullsProjectUnknown(t *testing.T) { // I09 (v2 §7.3)
	cases := []struct{ name, usage, cost string }{
		{"null usage and cost", `null`, `null`},
		{"partial counts and non-decimal cost", `{"m":{"input":1,"output":null,"cache_read":3,"cache_creation":4}}`, `"1e-3"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newLedgerFixture(t)
			f.append(t, f.event("wrk_a", "attempt.native_result", nativeResult(c.usage, c.cost)))
			rows := f.rows(t)
			want := 1
			if c.usage != `null` {
				want = 2
			}
			if len(rows) != want {
				t.Fatalf("got %d rows %+v, want %d unknown markers", len(rows), rows, want)
			}
			for _, r := range rows {
				if r.Quantity != "unknown" || r.Label != "unknown" || r.ProducerID != "" || r.Unit != "" || r.Source != "" {
					t.Errorf("row %+v is not an unknown marker", r)
				}
			}
			rowFor(t, rows, "retail-equivalent", "")
		})
	}
}

func TestProjectUsageFailureRollsBack(t *testing.T) {
	// A native_result for a run without a runs row cannot store rows: the
	// foreign key refuses it and the event is not journaled either.
	f := newLedgerFixture(t)
	ev := f.event("wrk_a", "attempt.native_result", nativeResult(`null`, `"0.10"`))
	ev.RunID = "run_missing"
	err := f.j.Append(t.Context(), ev, func(tx *sql.Tx) error { return projectWorkerEvent(context.Background(), tx, ev) })
	if err == nil {
		t.Fatal("projection for an unknown run succeeded")
	}
	if evs, _ := f.j.Events(t.Context(), "run_missing", 0); len(evs) != 0 {
		t.Fatalf("event journaled despite failed projection: %+v", evs)
	}
}
