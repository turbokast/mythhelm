package journal

import (
	"bytes"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

func TestCanonicalAdoptionV2Transaction(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	legacy, err := open(ctx, dir, migrations[:9])
	if err != nil {
		t.Fatal(err)
	}
	historical := Event{SchemaVersion: 1, EventID: "evt_historical", RunID: "run_canonical", ProducerID: "legacy", ProducerSequence: 1, Generation: 1, ObservedAt: time.Now().UTC(), Type: "legacy.fixture", Payload: []byte(`{ "original" : "bytes" }`)}
	if err := legacy.Append(ctx, historical, nil); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	j, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	before, err := j.Events(ctx, historical.RunID, 0)
	if err != nil || len(before) != 1 || before[0].SchemaVersion != 1 || before[0].Generation != 1 || before[0].ProducerSequence != 1 || !bytes.Equal(before[0].Payload, historical.Payload) {
		t.Fatalf("migration rewrote historical bytes: %v %v", before, err)
	}
	event := v2contract.Envelope{SchemaVersion: 2, EventID: "evt_canonical", RunID: "run_canonical", ProducerID: "service.canonical", ProducerSequence: 1, Generation: 2, ObservedAt: time.Now().UTC(), Type: "service.adopted", Payload: []byte(`{"schema_version":2}`)}
	for _, version := range []int{1, 3} {
		e := event
		e.SchemaVersion = version
		if err := j.Transact(ctx, func(tx *sql.Tx) error { return AppendCanonicalTx(ctx, tx, e, nil) }); err == nil {
			t.Fatal("noncanonical schema accepted")
		}
	}
	if err := j.Transact(ctx, func(tx *sql.Tx) error { return AppendCanonicalTx(ctx, tx, event, nil) }); err == nil {
		t.Fatal("pre-adoption phase accepted")
	}
	if err := j.Transact(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE migration_state SET phase='adopted' WHERE id=1`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("projection")
	if err := j.Transact(ctx, func(tx *sql.Tx) error { return AppendCanonicalTx(ctx, tx, event, func(*sql.Tx) error { return boom }) }); !errors.Is(err, boom) {
		t.Fatalf("rollback %v", err)
	}
	count := 0
	if err := j.Transact(ctx, func(tx *sql.Tx) error {
		return AppendCanonicalTx(ctx, tx, event, func(*sql.Tx) error { count++; return nil })
	}); err != nil {
		t.Fatal(err)
	}
	if err := j.Transact(ctx, func(tx *sql.Tx) error {
		return AppendCanonicalTx(ctx, tx, event, func(*sql.Tx) error { count++; return nil })
	}); err != nil {
		t.Fatal(err)
	}
	events, err := j.Events(ctx, event.RunID, 0)
	if err != nil || len(events) != 2 || events[1].RunSequence != 2 || count != 1 || !bytes.Equal(events[0].Payload, historical.Payload) {
		t.Fatalf("duplicate reapplied: %v %v count%d", events, err, count)
	}
}
