package control

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

func adoptionJournal(t *testing.T) (*journal.Journal, ServiceAdoption) {
	t.Helper()
	j := openPinJournal(t)
	now := time.Now().UTC()
	a := ServiceAdoption{SchemaVersion: 2, RunID: "run_adopt", AttemptID: "att_adopt", LaunchSHA256: strings.Repeat("a", 64), OperationID: "op_adopt", WorkerPID: 123, WorkerStartedAt: now, LaunchGeneration: 1, ControlGeneration: 2, ReconciliationEventID: "evt_adopt"}
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(t.Context(), `UPDATE migration_state SET phase='adopted' WHERE id=1`); err != nil {
			return err
		}
		if err := journal.InsertRun(t.Context(), tx, journal.RunRow{RunID: a.RunID, State: "executing", AdapterID: "builtin/fake", SourceRepo: "fixture", TaskSHA256: "fixture", BillingPosture: "local-scripted", ExecutionProfile: "trusted-host", CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		return journal.InsertAttempt(t.Context(), tx, journal.AttemptRow{AttemptID: a.AttemptID, RunID: a.RunID, TaskID: "task_adopt", AttemptNumber: 1, State: "executing", LaunchTokenSHA256: a.LaunchSHA256, WorkspacePath: "fixture"})
	}); err != nil {
		t.Fatal(err)
	}
	return j, a
}

func TestCanonicalAdoptionV2Transaction(t *testing.T) {
	j, a := adoptionJournal(t)
	ctx := t.Context()
	record := func(a ServiceAdoption) error {
		return j.Transact(ctx, func(tx *sql.Tx) error { return RecordServiceAdoption(ctx, tx, a) })
	}
	if err := record(a); err != nil {
		t.Fatal(err)
	}
	events, err := j.Events(ctx, a.RunID, 0)
	if err != nil || len(events) != 1 {
		t.Fatalf("events %v %v", events, err)
	}
	decoded, err := DecodeServiceAdoption(events[0])
	if err != nil || decoded != a {
		t.Fatalf("decode %+v %v", decoded, err)
	}
	if err := record(a); err != nil {
		t.Fatalf("replay %v", err)
	}
	changed := a
	changed.WorkerPID++
	if err := record(changed); err == nil {
		t.Fatal("changed operation accepted")
	}
	changed = a
	changed.OperationID = "op_collision"
	if err := record(changed); err == nil {
		t.Fatal("existing event accepted under new operation")
	}
	if err := j.Transact(ctx, func(tx *sql.Tx) error {
		var revision uint64
		var raw string
		if err := tx.QueryRowContext(ctx, `SELECT revision,result FROM operations WHERE operation_id=?`, a.OperationID).Scan(&revision, &raw); err != nil {
			return err
		}
		current, err := journal.RunRevision(ctx, tx, a.RunID)
		if err != nil {
			return err
		}
		var result Result
		if json.Unmarshal([]byte(raw), &result) != nil || result.OperationID != a.OperationID || result.Revision < 0 || uint64(result.Revision) != current || revision != current {
			return errors.New("result does not bind actual final revision")
		}
		epoch, ok, err := journal.ReadAssignmentEpoch(ctx, tx, a.RunID)
		if err != nil {
			return err
		}
		if !ok || epoch.AdoptionEventID != a.ReconciliationEventID {
			return errors.New("epoch missing")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*journal.Event){
		func(e *journal.Event) { e.SchemaVersion = 1 }, func(e *journal.Event) { e.SchemaVersion = 3 }, func(e *journal.Event) { e.Generation++ }, func(e *journal.Event) { e.RunID = "foreign" }, func(e *journal.Event) { e.EventID = "foreign" },
		func(e *journal.Event) { e.Payload = append([]byte(`{"unknown":1,`), e.Payload[1:]...) },
		func(e *journal.Event) { e.Payload = append([]byte(`{"schema_version":2,`), e.Payload[1:]...) },
		func(e *journal.Event) {
			e.Payload = []byte(strings.Replace(string(e.Payload), `"schema_version":2`, `"schema_version":1`, 1))
		},
		func(e *journal.Event) {
			e.Payload = []byte(strings.Replace(string(e.Payload), `"schema_version":2`, `"schema_version":3`, 1))
		},
	} {
		e := events[0]
		mutate(&e)
		if _, err := DecodeServiceAdoption(e); err == nil {
			t.Fatalf("bad event accepted: %s", e.Payload)
		}
	}
}

func TestAdoptionRollbackAndClaimedOnly(t *testing.T) {
	j, a := adoptionJournal(t)
	ctx := t.Context()
	boom := errors.New("rollback")
	if err := j.Transact(ctx, func(tx *sql.Tx) error {
		if err := RecordServiceAdoption(ctx, tx, a); err != nil {
			return err
		}
		return boom
	}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if events, err := j.Events(ctx, a.RunID, 0); err != nil || len(events) != 0 {
		t.Fatalf("rollback events %v %v", events, err)
	}
	raw, _ := json.Marshal(a)
	if err := j.Transact(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO operations(operation_id,method,object,digest,state,claimed_at) VALUES (?,'service.adopt',?,?,'claimed','now')`, a.OperationID, a.RunID, adoptionDigest(raw))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err := j.Transact(ctx, func(tx *sql.Tx) error { return RecordServiceAdoption(ctx, tx, a) })
	var ce *v2contract.ControlError
	if !errors.As(err, &ce) || ce.Code != v2contract.CodeOwnershipUnresolved {
		t.Fatalf("claimed-only %v", err)
	}
}

// Keep the context used in these transactions independent of external clients.
func adoptionDigest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
