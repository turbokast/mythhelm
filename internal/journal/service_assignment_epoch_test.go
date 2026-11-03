package journal

import (
	"database/sql"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

// All fixtures in this file are synthetic, with no native credentials.

func epochObserved(gen int64) string {
	return fmt.Sprintf("2026-11-02T00:00:%02d.000000001Z", gen)
}

func epochSeed(t *testing.T, j *Journal) {
	t.Helper()
	seedRun(t, j, "run_a")
	seedRun(t, j, "run_b")
	attachmentTx(t, j, func(tx *sql.Tx) {
		for index, suffix := range []string{"a", "b"} {
			if err := InsertAttempt(t.Context(), tx, AttemptRow{AttemptID: "att_" + suffix, RunID: "run_" + suffix, TaskID: "task", AttemptNumber: int64(index + 1), State: "running", LaunchTokenSHA256: "hash_" + suffix, WorkspacePath: "synthetic"}); err != nil {
				t.Fatal(err)
			}
		}
	})
}

// epochEvent writes an event whose envelope generation equals the payload's
// control generation, as an adoption producer must.
func epochEvent(t *testing.T, tx *sql.Tx, run, attempt, eventID, typ string, schema int, payload string) {
	t.Helper()
	var gen int64
	if err := tx.QueryRowContext(t.Context(), `SELECT json_extract(?, '$.control_generation')`, payload).Scan(&gen); err != nil {
		t.Fatal(err)
	}
	epochEventAt(t, tx, run, attempt, eventID, typ, schema, gen, payload)
}

func epochEventAt(t *testing.T, tx *sql.Tx, run, attempt, eventID, typ string, schema int, envelopeGen int64, payload string) {
	t.Helper()
	attachmentSQL(t, tx, `INSERT INTO journal (event_id, schema_version, run_id, task_id, attempt_id, producer_id, producer_sequence,
		run_sequence, generation, caused_by, observed_at, type, payload)
		VALUES (?, ?, ?, 'task', ?, ?, 1, (SELECT COALESCE(MAX(run_sequence), 0) + 1 FROM journal WHERE run_id = ?), ?, NULL, ?, ?, ?)`,
		eventID, schema, run, attempt, "producer_"+eventID, run, envelopeGen, epochObserved(int64(len(eventID))), typ, payload)
}

func epochPayload(op string, gen int64) string {
	return fmt.Sprintf(`{"operation_id":%q,"control_generation":%d}`, op, gen)
}

// epochEvidence writes a claimed internal service.adopt operation and its
// schema-2 service.adopted event for e.
func epochEvidence(t *testing.T, tx *sql.Tx, e AssignmentEpoch) {
	t.Helper()
	attachmentSQL(t, tx, `INSERT INTO operations VALUES (?, 'service.adopt', ?, 'digest', 'claimed', 0, NULL, 'now', NULL)`, e.OperationID, e.RunID)
	epochEvent(t, tx, e.RunID, e.AttemptID, e.AdoptionEventID, "service.adopted", 2, epochPayload(e.OperationID, e.ControlGeneration))
}

// epochComplete records the operation result with the actual revision read
// after every projection, never a guessed revision+1.
func epochComplete(t *testing.T, tx *sql.Tx, e AssignmentEpoch) uint64 {
	t.Helper()
	revision := attachmentRevision(t, tx, e.RunID)
	attachmentSQL(t, tx, `UPDATE operations SET state='done', revision=?, result=?, finished_at='now' WHERE operation_id=?`,
		revision, fmt.Sprintf(`{"operation_id":%q,"revision":%d}`, e.OperationID, revision), e.OperationID)
	return revision
}

func epochOf(run, attempt string, gen int64) AssignmentEpoch {
	suffix := run + "_" + strconv.FormatInt(gen, 10)
	return AssignmentEpoch{RunID: run, AttemptID: attempt, AdoptionEventID: "evt_" + suffix, OperationID: "op_" + suffix, ControlGeneration: gen}
}

func epochAdopt(t *testing.T, j *Journal, e AssignmentEpoch) {
	t.Helper()
	attachmentTx(t, j, func(tx *sql.Tx) {
		epochEvidence(t, tx, e)
		if err := RecordAssignmentEpoch(t.Context(), tx, e); err != nil {
			t.Fatal(err)
		}
		epochComplete(t, tx, e)
	})
}

func requireEpochError(t *testing.T, err error, code v2contract.Code, reason, op string) {
	t.Helper()
	var ce *v2contract.ControlError
	if !errors.As(err, &ce) {
		t.Fatalf("error = %v, want *v2contract.ControlError %s", err, code)
	}
	if ce.Code != code || ce.Owner != "journal" || ce.OperationID != op || ce.Detail["mythhelm.assignment-epoch/reason"] != reason {
		t.Fatalf("control error = %+v, want code %s owner journal operation %s reason %s", ce, code, op, reason)
	}
	if len(ce.Detail) != 1 {
		t.Fatalf("detail = %v, want only the fixed reason", ce.Detail)
	}
	if verr := ce.Validate(); verr != nil {
		t.Fatalf("invalid control error: %v", verr)
	}
}

type epochSnapshot struct {
	epochs, assignment, result string
	revision                   uint64
}

func takeEpochSnapshot(t *testing.T, j *Journal, run string) epochSnapshot {
	t.Helper()
	var s epochSnapshot
	attachmentTx(t, j, func(tx *sql.Tx) {
		q := func(query string) string {
			var out sql.NullString
			if err := tx.QueryRowContext(t.Context(), query, run).Scan(&out); err != nil {
				t.Fatal(err)
			}
			return out.String
		}
		s.epochs = q(`SELECT group_concat(run_id||'|'||control_generation||'|'||attempt_id||'|'||adoption_event_id||'|'||operation_id||'|'||rowid, ';') FROM (SELECT rowid, * FROM service_assignment_epochs WHERE run_id=? ORDER BY control_generation)`)
		s.assignment = q(`SELECT group_concat(owner||'|'||assigned_at||'|'||operation_id) FROM run_assignments WHERE run_id=?`)
		s.result = q(`SELECT group_concat(operation_id||'|'||state||'|'||revision||'|'||COALESCE(result,''), ';') FROM (SELECT * FROM operations WHERE object=? ORDER BY operation_id)`)
		s.revision = attachmentRevision(t, tx, run)
	})
	return s
}

func TestAssignmentEpochIdentityAndRevision(t *testing.T) { // I03/I07/I09/I23 (v2 sections 3/5/6): canonical epoch evidence, historical unknowns.
	t.Parallel()
	t.Run("record projects assignment and actual revision", func(t *testing.T) {
		t.Parallel()
		j, _ := openTemp(t)
		epochSeed(t, j)
		first := epochOf("run_a", "att_a", 1)
		attachmentTx(t, j, func(tx *sql.Tx) {
			epochEvidence(t, tx, first)
			before := attachmentRevision(t, tx, "run_a")
			if err := RecordAssignmentEpoch(t.Context(), tx, first); err != nil {
				t.Fatal(err)
			}
			afterRecord := attachmentRevision(t, tx, "run_a")
			if afterRecord <= before {
				t.Fatalf("revision %d after record, want > %d", afterRecord, before)
			}
			var owner, assignedAt, op string
			if err := tx.QueryRowContext(t.Context(), `SELECT owner, assigned_at, operation_id FROM run_assignments WHERE run_id='run_a'`).Scan(&owner, &assignedAt, &op); err != nil {
				t.Fatal(err)
			}
			if owner != "supervisor" || assignedAt != epochObserved(int64(len(first.AdoptionEventID))) || op != first.OperationID {
				t.Fatalf("assignment = %s %s %s, want supervisor at event time bound to %s", owner, assignedAt, op, first.OperationID)
			}
			recorded := epochComplete(t, tx, first)
			if recorded != afterRecord {
				t.Fatalf("operation revision %d, want actual %d", recorded, afterRecord)
			}
			got, ok, err := ReadAssignmentEpoch(t.Context(), tx, "run_a")
			if err != nil || !ok || got != first {
				t.Fatalf("read = %+v %t %v, want %+v", got, ok, err, first)
			}
		})
		second := epochOf("run_a", "att_a", 2)
		epochAdopt(t, j, second)
		attachmentTx(t, j, func(tx *sql.Tx) {
			got, ok, err := ReadAssignmentEpoch(t.Context(), tx, "run_a")
			if err != nil || !ok || got != second {
				t.Fatalf("read = %+v %t %v, want greatest %+v", got, ok, err, second)
			}
			var op string
			if err := tx.QueryRowContext(t.Context(), `SELECT operation_id FROM run_assignments WHERE run_id='run_a'`).Scan(&op); err != nil {
				t.Fatal(err)
			}
			if op != second.OperationID {
				t.Fatalf("assignment operation %s, want advanced to %s", op, second.OperationID)
			}
		})
	})
	t.Run("historical unknowns read false", func(t *testing.T) {
		t.Parallel()
		j, _ := openTemp(t)
		epochSeed(t, j)
		seedRun(t, j, "run_c")
		attachmentTx(t, j, func(tx *sql.Tx) {
			attachmentSQL(t, tx, `INSERT INTO operations VALUES ('op_hist', 'service.adopt', 'run_b', 'digest', 'done', 1, '{}', 'then', 'then')`)
			// An epoch packed into the owner string is not epoch evidence.
			attachmentSQL(t, tx, `INSERT INTO run_assignments VALUES ('run_b', 'supervisor#epoch=7', 'then', 'op_hist')`)
		})
		attachmentTx(t, j, func(tx *sql.Tx) {
			for _, run := range []string{"run_a", "run_b", "run_c"} {
				got, ok, err := ReadAssignmentEpoch(t.Context(), tx, run)
				if err != nil || ok || got != (AssignmentEpoch{}) {
					t.Errorf("%s read = %+v %t %v, want unknown false", run, got, ok, err)
				}
			}
			_, ok, err := ReadAssignmentEpoch(t.Context(), tx, "run_missing")
			if ok {
				t.Fatal("missing run read as known")
			}
			requireEpochError(t, err, v2contract.CodeInvalidContract, "missing", "assignment-epoch.read")
		})
	})
	t.Run("claimed only operation is unavailable", func(t *testing.T) {
		t.Parallel()
		j, _ := openTemp(t)
		epochSeed(t, j)
		e := epochOf("run_a", "att_a", 1)
		attachmentTx(t, j, func(tx *sql.Tx) {
			epochEvidence(t, tx, e)
			if err := RecordAssignmentEpoch(t.Context(), tx, e); err != nil {
				t.Fatal(err)
			}
		})
		attachmentTx(t, j, func(tx *sql.Tx) {
			got, ok, err := ReadAssignmentEpoch(t.Context(), tx, "run_a")
			if ok || got != (AssignmentEpoch{}) {
				t.Fatalf("claimed-only read = %+v %t, want unavailable", got, ok)
			}
			requireEpochError(t, err, v2contract.CodeOwnershipUnresolved, "missing", "assignment-epoch.read")
		})
	})
	readControls := []struct {
		name     string
		complete string
	}{
		{name: "result changed method", complete: `UPDATE operations SET method='run.stop', state='done', revision=(SELECT revision FROM run_control_revisions WHERE run_id='run_a'), result='{}', finished_at='now' WHERE operation_id=?`},
		{name: "result changed object", complete: `UPDATE operations SET object='run_b', state='done', revision=(SELECT revision FROM run_control_revisions WHERE run_id='run_a'), result='{}', finished_at='now' WHERE operation_id=?`},
		{name: "result revision zero", complete: `UPDATE operations SET state='done', revision=0, result='{}', finished_at='now' WHERE operation_id=?`},
		{name: "result revision ahead of run", complete: `UPDATE operations SET state='done', revision=(SELECT revision + 5 FROM run_control_revisions WHERE run_id='run_a'), result='{}', finished_at='now' WHERE operation_id=?`},
		{name: "result without finish time", complete: `UPDATE operations SET state='done', revision=(SELECT revision FROM run_control_revisions WHERE run_id='run_a'), result='{}' WHERE operation_id=?`},
		{name: "wrapper with another outer operation_id", complete: `UPDATE operations SET state='done', revision=(SELECT revision FROM run_control_revisions WHERE run_id='run_a'), result=json_object('operation_id', 'op_other', 'revision', (SELECT revision FROM run_control_revisions WHERE run_id='run_a')), finished_at='now' WHERE operation_id=?1`},
		{name: "wrapper revision differs from stored revision", complete: `UPDATE operations SET state='done', revision=(SELECT revision FROM run_control_revisions WHERE run_id='run_a'), result=json_object('operation_id', ?1, 'revision', (SELECT revision - 1 FROM run_control_revisions WHERE run_id='run_a')), finished_at='now' WHERE operation_id=?1`},
		{name: "malformed result JSON", complete: `UPDATE operations SET state='done', revision=(SELECT revision FROM run_control_revisions WHERE run_id='run_a'), result='{}', finished_at='now' WHERE operation_id=?1`},
		{name: "result JSON array not wrapper", complete: `UPDATE operations SET state='done', revision=(SELECT revision FROM run_control_revisions WHERE run_id='run_a'), result='[]', finished_at='now' WHERE operation_id=?1`},
		{name: "completed wrapper carrying refusal error", complete: `UPDATE operations SET state='done', revision=(SELECT revision FROM run_control_revisions WHERE run_id='run_a'), result=json_object('operation_id', ?1, 'revision', (SELECT revision FROM run_control_revisions WHERE run_id='run_a'), 'error', json_object('code', 'invalid_contract')), finished_at='now' WHERE operation_id=?1`},
	}
	acceptedWrappers := []struct {
		name     string
		complete string
	}{
		{name: "wrapper with opaque body", complete: `UPDATE operations SET state='done', revision=(SELECT revision FROM run_control_revisions WHERE run_id='run_a'), result=json_object('operation_id', ?1, 'revision', (SELECT revision FROM run_control_revisions WHERE run_id='run_a'), 'body', json_object('anything', json('[1]'))), finished_at='now' WHERE operation_id=?1`},
		{name: "wrapper with null error", complete: `UPDATE operations SET state='done', revision=(SELECT revision FROM run_control_revisions WHERE run_id='run_a'), result=json_object('operation_id', ?1, 'revision', (SELECT revision FROM run_control_revisions WHERE run_id='run_a'), 'error', json('null')), finished_at='now' WHERE operation_id=?1`},
	}
	for _, tc := range acceptedWrappers {
		t.Run("read accepts "+tc.name, func(t *testing.T) {
			t.Parallel()
			j, _ := openTemp(t)
			epochSeed(t, j)
			e := epochOf("run_a", "att_a", 1)
			attachmentTx(t, j, func(tx *sql.Tx) {
				epochEvidence(t, tx, e)
				if err := RecordAssignmentEpoch(t.Context(), tx, e); err != nil {
					t.Fatal(err)
				}
				attachmentSQL(t, tx, tc.complete, e.OperationID)
			})
			attachmentTx(t, j, func(tx *sql.Tx) {
				got, ok, err := ReadAssignmentEpoch(t.Context(), tx, "run_a")
				if err != nil || !ok || got != e {
					t.Fatalf("read = %+v %t %v, want %+v known", got, ok, err, e)
				}
			})
		})
	}
	t.Run("read refuses attempt rebound to another run after commit", func(t *testing.T) {
		t.Parallel()
		j, _ := openTemp(t)
		epochSeed(t, j)
		e := epochOf("run_a", "att_a", 1)
		epochAdopt(t, j, e)
		attachmentTx(t, j, func(tx *sql.Tx) {
			if _, ok, err := ReadAssignmentEpoch(t.Context(), tx, "run_a"); err != nil || !ok {
				t.Fatalf("baseline read = %t %v, want known", ok, err)
			}
			attachmentSQL(t, tx, `UPDATE attempts SET run_id='run_b' WHERE attempt_id='att_a'`)
		})
		attachmentTx(t, j, func(tx *sql.Tx) {
			got, ok, err := ReadAssignmentEpoch(t.Context(), tx, "run_a")
			if ok || got != (AssignmentEpoch{}) {
				t.Fatalf("read = %+v %t, want foreign link refused", got, ok)
			}
			requireEpochError(t, err, v2contract.CodeInvalidContract, "identity_mismatch", "assignment-epoch.read")
		})
	})
	for _, tc := range readControls {
		t.Run("read refuses "+tc.name, func(t *testing.T) {
			t.Parallel()
			j, _ := openTemp(t)
			epochSeed(t, j)
			e := epochOf("run_a", "att_a", 1)
			attachmentTx(t, j, func(tx *sql.Tx) {
				epochEvidence(t, tx, e)
				if err := RecordAssignmentEpoch(t.Context(), tx, e); err != nil {
					t.Fatal(err)
				}
				attachmentSQL(t, tx, tc.complete, e.OperationID)
			})
			attachmentTx(t, j, func(tx *sql.Tx) {
				got, ok, err := ReadAssignmentEpoch(t.Context(), tx, "run_a")
				if ok || got != (AssignmentEpoch{}) {
					t.Fatalf("read = %+v %t, want refused", got, ok)
				}
				requireEpochError(t, err, v2contract.CodeInvalidContract, "identity_mismatch", "assignment-epoch.read")
			})
		})
	}
	refusals := []struct {
		name   string
		prior  bool
		code   v2contract.Code
		reason string
		mutate func(t *testing.T, tx *sql.Tx, e *AssignmentEpoch)
	}{
		{name: "missing operation", code: v2contract.CodeInvalidContract, reason: "identity_mismatch", mutate: func(t *testing.T, tx *sql.Tx, e *AssignmentEpoch) {
			epochEvent(t, tx, e.RunID, e.AttemptID, e.AdoptionEventID, "service.adopted", 2, epochPayload(e.OperationID, e.ControlGeneration))
		}},
		{name: "foreign run operation", code: v2contract.CodeInvalidContract, reason: "identity_mismatch", mutate: func(t *testing.T, tx *sql.Tx, e *AssignmentEpoch) {
			attachmentSQL(t, tx, `INSERT INTO operations VALUES (?, 'service.adopt', 'run_b', 'digest', 'claimed', 0, NULL, 'now', NULL)`, e.OperationID)
			epochEvent(t, tx, e.RunID, e.AttemptID, e.AdoptionEventID, "service.adopted", 2, epochPayload(e.OperationID, e.ControlGeneration))
		}},
		{name: "public method operation", code: v2contract.CodeInvalidContract, reason: "identity_mismatch", mutate: func(t *testing.T, tx *sql.Tx, e *AssignmentEpoch) {
			attachmentSQL(t, tx, `INSERT INTO operations VALUES (?, 'run.stop', ?, 'digest', 'claimed', 0, NULL, 'now', NULL)`, e.OperationID, e.RunID)
			epochEvent(t, tx, e.RunID, e.AttemptID, e.AdoptionEventID, "service.adopted", 2, epochPayload(e.OperationID, e.ControlGeneration))
		}},
		{name: "event id as operation id", code: v2contract.CodeInvalidContract, reason: "identity_mismatch", mutate: func(t *testing.T, tx *sql.Tx, e *AssignmentEpoch) {
			epochEvidence(t, tx, *e)
			e.OperationID = e.AdoptionEventID
		}},
		{name: "payload operation mismatch", code: v2contract.CodeInvalidContract, reason: "identity_mismatch", mutate: func(t *testing.T, tx *sql.Tx, e *AssignmentEpoch) {
			attachmentSQL(t, tx, `INSERT INTO operations VALUES (?, 'service.adopt', ?, 'digest', 'claimed', 0, NULL, 'now', NULL)`, e.OperationID, e.RunID)
			epochEvent(t, tx, e.RunID, e.AttemptID, e.AdoptionEventID, "service.adopted", 2, epochPayload("op_other", e.ControlGeneration))
		}},
		{name: "payload generation mismatch", code: v2contract.CodeInvalidContract, reason: "identity_mismatch", mutate: func(t *testing.T, tx *sql.Tx, e *AssignmentEpoch) {
			attachmentSQL(t, tx, `INSERT INTO operations VALUES (?, 'service.adopt', ?, 'digest', 'claimed', 0, NULL, 'now', NULL)`, e.OperationID, e.RunID)
			epochEvent(t, tx, e.RunID, e.AttemptID, e.AdoptionEventID, "service.adopted", 2, epochPayload(e.OperationID, e.ControlGeneration+1))
		}},
		{name: "envelope generation mismatch", code: v2contract.CodeInvalidContract, reason: "identity_mismatch", mutate: func(t *testing.T, tx *sql.Tx, e *AssignmentEpoch) {
			e.ControlGeneration = 2
			attachmentSQL(t, tx, `INSERT INTO operations VALUES (?, 'service.adopt', ?, 'digest', 'claimed', 0, NULL, 'now', NULL)`, e.OperationID, e.RunID)
			epochEventAt(t, tx, e.RunID, e.AttemptID, e.AdoptionEventID, "service.adopted", 2, 1, epochPayload(e.OperationID, 2))
		}},
		{name: "schema one event", code: v2contract.CodeInvalidContract, reason: "identity_mismatch", mutate: func(t *testing.T, tx *sql.Tx, e *AssignmentEpoch) {
			attachmentSQL(t, tx, `INSERT INTO operations VALUES (?, 'service.adopt', ?, 'digest', 'claimed', 0, NULL, 'now', NULL)`, e.OperationID, e.RunID)
			epochEvent(t, tx, e.RunID, e.AttemptID, e.AdoptionEventID, "service.adopted", 1, epochPayload(e.OperationID, e.ControlGeneration))
		}},
		{name: "foreign attempt", code: v2contract.CodeInvalidContract, reason: "identity_mismatch", mutate: func(t *testing.T, tx *sql.Tx, e *AssignmentEpoch) {
			epochEvidence(t, tx, *e)
			e.AttemptID = "att_b"
		}},
		{name: "non positive generation", code: v2contract.CodeInvalidContract, reason: "identity_mismatch", mutate: func(t *testing.T, tx *sql.Tx, e *AssignmentEpoch) {
			e.ControlGeneration = 0
			epochEvidence(t, tx, *e)
		}},
		{name: "missing run", code: v2contract.CodeInvalidContract, reason: "missing", mutate: func(_ *testing.T, _ *sql.Tx, e *AssignmentEpoch) {
			e.RunID = "run_missing"
		}},
		{name: "stale generation", prior: true, code: v2contract.CodeRevisionConflict, reason: "stale", mutate: func(t *testing.T, tx *sql.Tx, e *AssignmentEpoch) {
			*e = AssignmentEpoch{RunID: "run_a", AttemptID: "att_a", AdoptionEventID: "evt_stale", OperationID: "op_stale", ControlGeneration: 1}
			epochEvidence(t, tx, *e)
		}},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			j, _ := openTemp(t)
			epochSeed(t, j)
			e := epochOf("run_a", "att_a", 1)
			if tc.prior {
				epochAdopt(t, j, epochOf("run_a", "att_a", 2))
				e = epochOf("run_a", "att_a", 3)
			}
			before := takeEpochSnapshot(t, j, "run_a")
			err := j.Transact(t.Context(), func(tx *sql.Tx) error {
				tc.mutate(t, tx, &e)
				return RecordAssignmentEpoch(t.Context(), tx, e)
			})
			requireEpochError(t, err, tc.code, tc.reason, "assignment-epoch.record")
			if after := takeEpochSnapshot(t, j, "run_a"); after != before {
				t.Fatalf("refused record changed state: %+v -> %+v", before, after)
			}
		})
	}
}

func TestAssignmentEpochAtomicReplay(t *testing.T) { // I12/I20/I21 (v2 sections 5/6): one atomic transaction, immutable replay.
	t.Parallel()
	t.Run("exact committed replay changes nothing", func(t *testing.T) {
		t.Parallel()
		j, _ := openTemp(t)
		epochSeed(t, j)
		e := epochOf("run_a", "att_a", 1)
		epochAdopt(t, j, e)
		before := takeEpochSnapshot(t, j, "run_a")
		if before.epochs == "" || before.assignment == "" {
			t.Fatalf("adoption did not commit: %+v", before)
		}
		attachmentTx(t, j, func(tx *sql.Tx) {
			if err := RecordAssignmentEpoch(t.Context(), tx, e); err != nil {
				t.Fatalf("exact replay: %v", err)
			}
		})
		if after := takeEpochSnapshot(t, j, "run_a"); after != before {
			t.Fatalf("replay changed state: %+v -> %+v", before, after)
		}
	})
	conflicts := []struct {
		name   string
		mutate func(t *testing.T, tx *sql.Tx, e *AssignmentEpoch)
	}{
		{name: "changed operation same event", mutate: func(t *testing.T, tx *sql.Tx, e *AssignmentEpoch) {
			attachmentSQL(t, tx, `INSERT INTO operations VALUES ('op_changed', 'service.adopt', 'run_a', 'digest', 'claimed', 0, NULL, 'now', NULL)`)
			e.OperationID = "op_changed"
		}},
		{name: "changed arguments same operation", mutate: func(t *testing.T, tx *sql.Tx, e *AssignmentEpoch) {
			epochEvent(t, tx, "run_a", "att_a", "evt_changed", "service.adopted", 2, epochPayload(e.OperationID, 2))
			e.AdoptionEventID, e.ControlGeneration = "evt_changed", 2
		}},
		{name: "changed attempt same identity", mutate: func(_ *testing.T, _ *sql.Tx, e *AssignmentEpoch) {
			e.AttemptID = "att_b"
		}},
	}
	for _, tc := range conflicts {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			j, _ := openTemp(t)
			epochSeed(t, j)
			e := epochOf("run_a", "att_a", 1)
			epochAdopt(t, j, e)
			before := takeEpochSnapshot(t, j, "run_a")
			err := j.Transact(t.Context(), func(tx *sql.Tx) error {
				tc.mutate(t, tx, &e)
				return RecordAssignmentEpoch(t.Context(), tx, e)
			})
			requireEpochError(t, err, v2contract.CodeRevisionConflict, "conflicting", "assignment-epoch.record")
			if after := takeEpochSnapshot(t, j, "run_a"); after != before {
				t.Fatalf("conflict changed state: %+v -> %+v", before, after)
			}
		})
	}
	failAt := []string{"after evidence", "projection", "after record", "after completion"}
	for _, step := range failAt {
		t.Run("rollback "+step, func(t *testing.T) {
			t.Parallel()
			j, _ := openTemp(t)
			epochSeed(t, j)
			e := epochOf("run_a", "att_a", 1)
			before := takeEpochSnapshot(t, j, "run_a")
			synthetic := errors.New("synthetic failure")
			err := j.Transact(t.Context(), func(tx *sql.Tx) error {
				epochEvidence(t, tx, e)
				if step == "after evidence" {
					return synthetic
				}
				if step == "projection" {
					attachmentSQL(t, tx, `CREATE TRIGGER synthetic_projection_failure BEFORE INSERT ON run_assignments BEGIN SELECT RAISE(ABORT, 'synthetic'); END`)
				}
				if err := RecordAssignmentEpoch(t.Context(), tx, e); err != nil {
					return err
				}
				if step == "after record" {
					return synthetic
				}
				epochComplete(t, tx, e)
				return synthetic
			})
			if step == "projection" {
				requireEpochError(t, err, v2contract.CodePersistenceUnavailable, "storage_unavailable", "assignment-epoch.record")
			} else if !errors.Is(err, synthetic) {
				t.Fatalf("transaction error = %v, want synthetic", err)
			}
			if after := takeEpochSnapshot(t, j, "run_a"); after != before {
				t.Fatalf("failed adoption left state: %+v -> %+v", before, after)
			}
			var events int
			if err := j.db.QueryRowContext(t.Context(), `SELECT count(*) FROM journal WHERE event_id=?`, e.AdoptionEventID).Scan(&events); err != nil {
				t.Fatal(err)
			}
			if events != 0 {
				t.Fatalf("rolled back event persisted %d times", events)
			}
		})
	}
	t.Run("committed claimed only is unavailable after reopen", func(t *testing.T) {
		t.Parallel()
		j, dir := openTemp(t)
		epochSeed(t, j)
		e := epochOf("run_a", "att_a", 1)
		attachmentTx(t, j, func(tx *sql.Tx) {
			epochEvidence(t, tx, e)
			if err := RecordAssignmentEpoch(t.Context(), tx, e); err != nil {
				t.Fatal(err)
			}
		})
		if err := j.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(t.Context(), dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = reopened.Close() }()
		attachmentTx(t, reopened, func(tx *sql.Tx) {
			_, ok, err := ReadAssignmentEpoch(t.Context(), tx, "run_a")
			if ok {
				t.Fatal("claimed-only adoption read as known")
			}
			requireEpochError(t, err, v2contract.CodeOwnershipUnresolved, "missing", "assignment-epoch.read")
		})
	})
}

func TestAssignmentEpochSQLGuards(t *testing.T) { // I20/I23 (v2 sections 5/6): append-only epoch history, no implicit victim deletion.
	t.Parallel()
	// Each case runs against a committed epoch generation 1 for run_a; prep
	// writes valid evidence so only the guard under test can refuse.
	guards := []struct {
		name, prep, attack string
		// control drops the named triggers in a rolled-back transaction and
		// shows the attack then really changes the victim.
		control []string
	}{
		{name: "insert or replace primary key", prep: "gen1", attack: `INSERT OR REPLACE INTO service_assignment_epochs VALUES ('run_a', 1, 'att_a', 'evt_x', 'op_x')`,
			control: []string{"service_assignment_epochs_identity", "service_assignment_epochs_no_replace", "service_assignment_epochs_stale"}},
		{name: "insert or replace adoption event", prep: "gen2", attack: `INSERT OR REPLACE INTO service_assignment_epochs VALUES ('run_a', 2, 'att_a', 'evt_run_a_1', 'op_x')`,
			control: []string{"service_assignment_epochs_identity", "service_assignment_epochs_no_replace"}},
		{name: "insert or replace operation", prep: "gen2", attack: `INSERT OR REPLACE INTO service_assignment_epochs VALUES ('run_a', 2, 'att_a', 'evt_x', 'op_run_a_1')`,
			control: []string{"service_assignment_epochs_identity", "service_assignment_epochs_no_replace"}},
		{name: "insert or replace rowid", prep: "gen2", attack: `INSERT OR REPLACE INTO service_assignment_epochs (rowid, run_id, control_generation, attempt_id, adoption_event_id, operation_id) VALUES (1, 'run_a', 2, 'att_a', 'evt_x', 'op_x')`,
			control: []string{"service_assignment_epochs_no_replace"}},
		{name: "update or replace", prep: "record2", attack: `UPDATE OR REPLACE service_assignment_epochs SET control_generation = 1 WHERE control_generation = 2`,
			control: []string{"service_assignment_epochs_no_update"}},
		{name: "update immutable", attack: `UPDATE service_assignment_epochs SET attempt_id = 'att_b'`, control: []string{"service_assignment_epochs_no_update"}},
		{name: "delete immutable", attack: `DELETE FROM service_assignment_epochs`, control: []string{"service_assignment_epochs_no_delete"}},
		{name: "inconsistent attempt link", prep: "gen2", attack: `INSERT INTO service_assignment_epochs VALUES ('run_a', 2, 'att_b', 'evt_x', 'op_x')`,
			control: []string{"service_assignment_epochs_identity"}},
		{name: "assignment owner divergence", attack: `UPDATE run_assignments SET owner = 'other' WHERE run_id = 'run_a'`, control: []string{"run_assignments_epoch_update"}},
		{name: "assignment operation divergence", prep: "gen2", attack: `UPDATE run_assignments SET operation_id = 'op_x' WHERE run_id = 'run_a'`, control: []string{"run_assignments_epoch_update"}},
		{name: "assignment delete", attack: `DELETE FROM run_assignments WHERE run_id = 'run_a'`, control: []string{"run_assignments_epoch_delete"}},
	}
	for _, tc := range guards {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			j, _ := openTemp(t)
			epochSeed(t, j)
			epochAdopt(t, j, epochOf("run_a", "att_a", 1))
			prep := func(tx *sql.Tx) {
				switch tc.prep {
				case "gen1", "gen2":
					gen := int64(1)
					if tc.prep == "gen2" {
						gen = 2
					}
					epochEvidence(t, tx, AssignmentEpoch{RunID: "run_a", AttemptID: "att_a", AdoptionEventID: "evt_x", OperationID: "op_x", ControlGeneration: gen})
				case "record2":
					e := AssignmentEpoch{RunID: "run_a", AttemptID: "att_a", AdoptionEventID: "evt_x", OperationID: "op_x", ControlGeneration: 2}
					epochEvidence(t, tx, e)
					if err := RecordAssignmentEpoch(t.Context(), tx, e); err != nil {
						t.Fatal(err)
					}
				}
			}
			before := takeEpochSnapshot(t, j, "run_a")
			err := j.Transact(t.Context(), func(tx *sql.Tx) error {
				prep(tx)
				_, err := tx.ExecContext(t.Context(), tc.attack)
				return err
			})
			if err == nil {
				t.Fatal("hostile SQL accepted")
			}
			if after := takeEpochSnapshot(t, j, "run_a"); after != before {
				t.Fatalf("refused SQL changed state: %+v -> %+v", before, after)
			}
			// Removed-guard control: the same statement does reach the victim.
			control := errors.New("control rollback")
			err = j.Transact(t.Context(), func(tx *sql.Tx) error {
				for _, trigger := range tc.control {
					attachmentSQL(t, tx, "DROP TRIGGER "+trigger)
				}
				prep(tx)
				if _, err := tx.ExecContext(t.Context(), tc.attack); err != nil {
					t.Fatalf("control without %v still refused: %v", tc.control, err)
				}
				var victim string
				if err := tx.QueryRowContext(t.Context(), `SELECT COALESCE((SELECT group_concat(run_id||'|'||control_generation||'|'||attempt_id||'|'||adoption_event_id||'|'||operation_id||'|'||rowid, ';') FROM (SELECT rowid, * FROM service_assignment_epochs ORDER BY control_generation)), '')||'#'||COALESCE((SELECT owner||'|'||operation_id FROM run_assignments WHERE run_id='run_a'), '')`).Scan(&victim); err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(victim, "run_a|1|att_a|evt_run_a_1|op_run_a_1|1") && strings.HasSuffix(victim, "#supervisor|op_run_a_1") {
					t.Fatalf("control left victim unchanged (%s); attack proves nothing", victim)
				}
				return control
			})
			if !errors.Is(err, control) {
				t.Fatalf("control transaction = %v", err)
			}
		})
	}
	t.Run("historical assignment keeps released mutability", func(t *testing.T) {
		t.Parallel()
		j, _ := openTemp(t)
		epochSeed(t, j)
		attachmentTx(t, j, func(tx *sql.Tx) {
			attachmentSQL(t, tx, `INSERT INTO operations VALUES ('op_hist', 'run.adopt', 'run_b', 'digest', 'done', 1, '{}', 'then', 'then')`)
			attachmentSQL(t, tx, `INSERT INTO run_assignments VALUES ('run_b', 'supervisor', 'then', 'op_hist')`)
			attachmentSQL(t, tx, `UPDATE run_assignments SET owner = 'tui' WHERE run_id = 'run_b'`)
			attachmentSQL(t, tx, `DELETE FROM run_assignments WHERE run_id = 'run_b'`)
		})
	})
	t.Run("storage evidence imports no authority packages", func(t *testing.T) {
		t.Parallel()
		file, err := parser.ParseFile(token.NewFileSet(), "service_assignment_epoch.go", nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			path := strings.Trim(spec.Path.Value, `"`)
			for _, banned := range []string{"/control", "/supervisor", "/service", "/workers", "/adapters"} {
				if strings.Contains(path, "mythhelm/internal"+banned) {
					t.Errorf("storage reader imports %s", path)
				}
			}
		}
	})
}
