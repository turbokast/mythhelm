package journal

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

func TestAppendCanonicalTx(t *testing.T) {
	j, _ := openTemp(t)
	ctx := t.Context()
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
	if err != nil || len(events) != 1 || events[0].RunSequence != 1 || count != 1 {
		t.Fatalf("duplicate reapplied: %v %v count%d", events, err, count)
	}
}
