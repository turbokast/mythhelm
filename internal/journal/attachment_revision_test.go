package journal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// All fixtures in this file are synthetic, with no native credentials.
func attachmentSQL(t *testing.T, tx *sql.Tx, query string, args ...any) {
	t.Helper()
	if _, err := tx.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func attachmentRevision(t *testing.T, tx *sql.Tx, run string) uint64 {
	t.Helper()
	revision, err := RunRevision(t.Context(), tx, run)
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func attachmentTx(t *testing.T, j *Journal, fn func(*sql.Tx)) {
	t.Helper()
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error { fn(tx); return nil }); err != nil {
		t.Fatal(err)
	}
}

func attachmentParents(t *testing.T, j *Journal) {
	t.Helper()
	for _, run := range []string{"run_a", "run_b"} {
		seedRun(t, j, run)
	}
	attachmentTx(t, j, func(tx *sql.Tx) {
		for index, suffix := range []string{"a", "b"} {
			if err := InsertAttempt(t.Context(), tx, AttemptRow{AttemptID: "att_" + suffix, RunID: "run_" + suffix, TaskID: "task", AttemptNumber: int64(index + 1), State: "running", LaunchTokenSHA256: "hash_" + suffix, WorkspacePath: "synthetic"}); err != nil {
				t.Fatal(err)
			}
			attachmentSQL(t, tx, `UPDATE attempts SET worker_pid=123, worker_start_time='synthetic-start' WHERE attempt_id=?`, "att_"+suffix)
			attachmentSQL(t, tx, `INSERT INTO verifications VALUES (?, ?, 'candidate', 'config', 'unknown', 'start', NULL)`, "ver_"+suffix, "run_"+suffix)
			attachmentSQL(t, tx, `INSERT INTO operations VALUES (?, 'attachment.stop', ?, 'digest', 'done', 1, '{"accepted":true}', 'now','now')`, "op_"+suffix, "run_"+suffix)
		}
	})
}

func TestAttachmentMigration(t *testing.T) { // I09/I23 (v2 sections 5/6): preserve historical unknowns and one canonical store.
	t.Parallel()
	t.Run("legacy values and immutable results", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		legacy, err := open(t.Context(), dir, migrations[:8])
		if err != nil {
			t.Fatal(err)
		}
		seedRun(t, legacy, "legacy")
		attachmentTx(t, legacy, func(tx *sql.Tx) {
			attachmentSQL(t, tx, `UPDATE runs SET reason=NULL WHERE run_id='legacy'`)
			attachmentSQL(t, tx, `INSERT INTO operations VALUES ('old','stop','legacy','digest','done',3,'{"unknown":null}','then','then')`)
		})
		before := schemaObjects(t, legacy.db)
		if err := legacy.Close(); err != nil {
			t.Fatal(err)
		}
		j, err := Open(t.Context(), dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = j.Close() }()
		if SchemaVersion != 9 {
			t.Fatalf("schema=%d, want9", SchemaVersion)
		}
		after := schemaObjects(t, j.db)
		for name, ddl := range before {
			if after[name] != ddl {
				t.Errorf("released schema object %s changed", name)
			}
		}
		for _, table := range []string{"attachment_grants", "run_control_revisions", "attachment_projections", "stop_outbox", "stop_results", "prepared_runs", "dispatch_outbox"} {
			if after[table] == "" {
				t.Errorf("missing table %s", table)
			}
		}
		var reason sql.NullString
		var result string
		if err := j.db.QueryRowContext(t.Context(), `SELECT reason FROM runs WHERE run_id='legacy'`).Scan(&reason); err != nil {
			t.Fatal(err)
		}
		if reason.Valid {
			t.Fatalf("unknown reason became %q", reason.String)
		}
		if err := j.db.QueryRowContext(t.Context(), `SELECT result FROM operations WHERE operation_id='old'`).Scan(&result); err != nil {
			t.Fatal(err)
		}
		if result != `{"unknown":null}` {
			t.Fatalf("legacy result changed: %s", result)
		}
		attachmentTx(t, j, func(tx *sql.Tx) {
			if got := attachmentRevision(t, tx, "legacy"); got != 1 {
				t.Errorf("seeded revision=%d, want1", got)
			}
		})
		// New producers get hash-only authority storage; no legacy unknown is populated.
		var stateSchema int
		if err := j.db.QueryRowContext(t.Context(), `SELECT schema_version FROM migration_state`).Scan(&stateSchema); err != nil {
			t.Fatal(err)
		}
		if stateSchema != 9 {
			t.Fatalf("migration schema=%d, want9", stateSchema)
		}
		for _, table := range []string{"attachment_grants", "stop_outbox", "dispatch_outbox", "prepared_runs"} {
			var ddl string
			if err := j.db.QueryRowContext(t.Context(), `SELECT sql FROM sqlite_schema WHERE name=?`, table).Scan(&ddl); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(ddl, "raw_token") || strings.Contains(ddl, "environment") {
				t.Errorf("sensitive storage in %s", table)
			}
		}
	})
	t.Run("failed upgrade rolls back tables triggers and version", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		old, err := open(t.Context(), dir, migrations[:8])
		if err != nil {
			t.Fatal(err)
		}
		seedRun(t, old, "legacy")
		before := schemaObjects(t, old.db)
		if err := old.Close(); err != nil {
			t.Fatal(err)
		}
		bad := append([]string{}, migrations[:8]...)
		bad = append(bad, migrations[8]+"\nINSERT INTO missing_table VALUES (1);")
		if j, err := open(t.Context(), dir, bad); err == nil {
			_ = j.Close()
			t.Fatal("invalid migration accepted")
		}
		restored, err := open(t.Context(), dir, migrations[:8])
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = restored.Close() }()
		after := schemaObjects(t, restored.db)
		if len(after) != len(before) {
			t.Fatalf("schema objects=%d, want%d", len(after), len(before))
		}
		for name, ddl := range before {
			if after[name] != ddl {
				t.Errorf("rollback changed %s", name)
			}
		}
		if run, err := restored.Run(t.Context(), "legacy"); err != nil || run.State != "created" {
			t.Fatalf("legacy after rollback=%+v, %v", run, err)
		}
	})
	t.Run("newer schema refused without touching bytes", func(t *testing.T) {
		t.Parallel()
		j, dir := openTemp(t)
		seedRun(t, j, "future")
		if _, err := j.db.ExecContext(t.Context(), `PRAGMA user_version=10`); err != nil {
			t.Fatal(err)
		}
		if err := j.Close(); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, DBName)
		before := fileSHA256(t, path)
		if newer, err := Open(t.Context(), dir); !errors.Is(err, ErrSchemaTooNew) {
			if newer != nil {
				_ = newer.Close()
			}
			t.Fatalf("newer open=%v", err)
		}
		if got := fileSHA256(t, path); got != before {
			t.Fatal("newer database bytes changed")
		}
		if _, err := os.Stat(path + ".bak-v10"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("newer refusal created backup: %v", err)
		}
	})
}

func TestRunRevisionCoversProjection(t *testing.T) { // I07/I12/I23 (v2 sections 5/6): evidence and projection share revision transaction.
	t.Parallel()
	// Keep a deliberately incomplete trigger inventory as a counterfactual.
	for _, omit := range []bool{false, true} {
		t.Run(fmt.Sprintf("omitted_dependency_%t", omit), func(t *testing.T) {
			t.Parallel()
			j, _ := openTemp(t)
			attachmentParents(t, j)
			attachmentTx(t, j, func(tx *sql.Tx) {
				if omit {
					attachmentSQL(t, tx, `DROP TRIGGER check_results_revision_insert`)
				}
				before := attachmentRevision(t, tx, "run_a")
				attachmentSQL(t, tx, `INSERT INTO check_results VALUES ('ver_a','unit','[]','unknown',NULL,NULL,NULL,NULL)`)
				changed := attachmentRevision(t, tx, "run_a") > before
				if changed == omit {
					t.Fatalf("omitted=%t changed=%t", omit, changed)
				}
			})
		})
	}
	t.Run("actual journal projection rollback dedup and offsets", func(t *testing.T) {
		t.Parallel()
		j, _ := openTemp(t)
		seedRun(t, j, "run_a")
		var initial uint64
		attachmentTx(t, j, func(tx *sql.Tx) {
			initial = attachmentRevision(t, tx, "run_a")
			if initial != 1 {
				t.Fatalf("initial=%d", initial)
			}
		})
		ev := event("run_a", "producer", 1, 0)
		if err := j.Append(t.Context(), ev, func(tx *sql.Tx) error {
			if got := attachmentRevision(t, tx, "run_a"); got != initial+1 {
				t.Fatalf("event revision inside callback=%d", got)
			}
			return InsertAttempt(t.Context(), tx, AttemptRow{AttemptID: "ingested", RunID: "run_a", TaskID: "task", AttemptNumber: 1, State: "running", LaunchTokenSHA256: "digest", WorkspacePath: "synthetic"})
		}); err != nil {
			t.Fatal(err)
		}
		attachmentTx(t, j, func(tx *sql.Tx) {
			if got := attachmentRevision(t, tx, "run_a"); got != initial+2 {
				t.Fatalf("committed event+attempt revision=%d", got)
			}
		})
		if err := j.Append(t.Context(), ev, func(*sql.Tx) error { t.Fatal("duplicate projected"); return nil }); err != nil {
			t.Fatal(err)
		}
		if err := j.AdvanceSpoolOffset(t.Context(), "ingested", 123); err != nil {
			t.Fatal(err)
		}
		attachmentTx(t, j, func(tx *sql.Tx) {
			if got := attachmentRevision(t, tx, "run_a"); got != initial+2 {
				t.Fatalf("duplicate/offset changed revision=%d", got)
			}
		})
		sentinel := errors.New("rollback")
		if err := j.Append(t.Context(), event("run_a", "producer", 2, 0), func(tx *sql.Tx) error {
			if err := SetAttemptState(t.Context(), tx, "ingested", "stopped", ""); err != nil {
				return err
			}
			if got := attachmentRevision(t, tx, "run_a"); got != initial+4 {
				t.Fatalf("uncommitted revisions=%d", got)
			}
			return sentinel
		}); !errors.Is(err, sentinel) {
			t.Fatalf("rollback=%v", err)
		}
		attachmentTx(t, j, func(tx *sql.Tx) {
			if got := attachmentRevision(t, tx, "run_a"); got != initial+2 {
				t.Fatalf("rolled back revision=%d", got)
			}
		})
	})
	t.Run("every source mutation and indirect rebinding", testAttachmentSourceInventory)
	t.Run("snapshot sees neither uncommitted revision nor partial source", testAttachmentSnapshot)
}

// The inventory is deliberately explicit: changing the source set or selected
// column list must update this test and migration together. Immutable source
// tables retain their stronger rejection of UPDATE/DELETE, including no-ops.
func testAttachmentSourceInventory(t *testing.T) {
	t.Helper()
	cases := []struct {
		table, insert, update, noOp, rebind, delete string
		immutable                                   bool
	}{
		{"runs", `INSERT INTO runs SELECT 'run_new',state,reason,adapter_id,source_repo,source_branch,base_rev,task_sha256,billing_posture,execution_profile,created_at,updated_at FROM runs WHERE run_id='run_a'`, `UPDATE runs SET reason='changed' WHERE run_id='run_a'`, `UPDATE runs SET state=state,reason=reason,updated_at='later' WHERE run_id='run_a'`, "", `DELETE FROM runs WHERE run_id='run_new'`, false},
		{"attempts", `INSERT INTO attempts (attempt_id,run_id,task_id,attempt_number,state,launch_token_sha256,workspace_path) VALUES ('extra','run_a','other',2,'running','digest','workspace')`, `UPDATE attempts SET native_session_id='session' WHERE attempt_id='extra'`, `UPDATE attempts SET native_session_id=native_session_id,spool_offset=100 WHERE attempt_id='extra'`, `UPDATE attempts SET run_id='run_b' WHERE attempt_id='extra'`, `DELETE FROM attempts WHERE attempt_id='extra'`, false},
		{"journal", `INSERT INTO journal VALUES (1,'evt',1,'run_a',NULL,NULL,'p',1,1,0,NULL,'now','attempt.native_session','{"session_id":"native"}')`, `UPDATE journal SET type='other' WHERE event_id='evt'`, `UPDATE journal SET type=type WHERE event_id='evt'`, "", `DELETE FROM journal WHERE event_id='evt'`, true},
		{"candidates", `INSERT INTO candidates VALUES ('att_a','base','commit','tree','hash','[]','[]',0)`, `UPDATE candidates SET partial=1 WHERE attempt_id='att_a'`, `UPDATE candidates SET flags=flags WHERE attempt_id='att_a'`, `UPDATE candidates SET attempt_id='att_b' WHERE attempt_id='att_a'`, `DELETE FROM candidates`, false},
		{"verifications", `INSERT INTO verifications VALUES ('extra','run_a','c','cfg','unknown','now',NULL)`, `UPDATE verifications SET result='failed' WHERE verification_id='extra'`, `UPDATE verifications SET finished_at=finished_at WHERE verification_id='extra'`, `UPDATE verifications SET run_id='run_b' WHERE verification_id='extra'`, `DELETE FROM verifications WHERE verification_id='extra'`, false},
		{"verification_bindings", `INSERT INTO verification_bindings VALUES ('ver_a','unknown',NULL,NULL,NULL)`, `UPDATE verification_bindings SET reason='changed'`, `UPDATE verification_bindings SET reason=reason`, "", `DELETE FROM verification_bindings`, true},
		{"check_results", `INSERT INTO check_results VALUES ('ver_a','unit','[]','unknown',NULL,NULL,NULL,NULL)`, `UPDATE check_results SET status='failed'`, `UPDATE check_results SET exit_code=exit_code`, `UPDATE check_results SET verification_id='ver_b'`, `DELETE FROM check_results`, false},
		{"applies", `INSERT INTO applies VALUES ('apply','run_a','repo','branch','candidate','pending',NULL,'now')`, `UPDATE applies SET state='applied'`, `UPDATE applies SET reason=reason`, `UPDATE applies SET run_id='run_b'`, `DELETE FROM applies`, false},
		{"run_assignments", `INSERT INTO run_assignments VALUES ('run_a','supervisor','now','op_a')`, `UPDATE run_assignments SET owner='other'`, `UPDATE run_assignments SET owner=owner`, `UPDATE run_assignments SET run_id='run_b'`, `DELETE FROM run_assignments`, false},
		{"stop_results", `INSERT INTO stop_results VALUES ('op_a','requested','',NULL,1)`, `UPDATE stop_results SET state='unconfirmed',reason='stop_unconfirmed',result_revision=2`, `UPDATE stop_results SET reason=reason`, "", `DELETE FROM stop_results`, false},
	}
	for _, tc := range cases {
		t.Run(tc.table, func(t *testing.T) {
			t.Parallel()
			j, _ := openTemp(t)
			attachmentParents(t, j)
			attachmentTx(t, j, func(tx *sql.Tx) {
				if tc.table == "stop_results" {
					attachmentSQL(t, tx, `INSERT INTO stop_outbox VALUES ('op_a','run_a','att_a','hash_a',123,'synthetic-start',1,'pending')`)
				}
				initial := attachmentRevision(t, tx, "run_a")
				attachmentSQL(t, tx, tc.insert)
				// A newly inserted run initialises its own revision, not another run's.
				target := "run_a"
				if tc.table == "runs" {
					target = "run_new"
				}
				got := attachmentRevision(t, tx, target)
				if tc.table == "runs" {
					if got != 1 {
						t.Fatalf("new run revision=%d", got)
					}
				} else if got != initial+1 {
					t.Fatalf("insert revision=%d want%d", got, initial+1)
				}
				// runs.update intentionally targets the original run; all other fixtures
				// operate on the just-inserted dependent row.
				target = "run_a"
				before := attachmentRevision(t, tx, target)
				_, err := tx.ExecContext(t.Context(), tc.update)
				want := before + 1
				if tc.immutable {
					if err == nil {
						t.Fatal("immutable update accepted")
					}
					want = before
				} else if err != nil {
					t.Fatal(err)
				}
				if got := attachmentRevision(t, tx, target); got != want {
					t.Fatalf("update revision=%d want%d", got, want)
				}
				_, err = tx.ExecContext(t.Context(), tc.noOp)
				if tc.immutable {
					if err == nil {
						t.Fatal("immutable no-op accepted")
					}
				} else if err != nil {
					t.Fatal(err)
				}
				if got := attachmentRevision(t, tx, target); got != want {
					t.Fatalf("no-op revision=%d want%d", got, want)
				}
				if tc.rebind != "" {
					other := attachmentRevision(t, tx, "run_b")
					attachmentSQL(t, tx, tc.rebind)
					if got := attachmentRevision(t, tx, "run_a"); got != want+1 {
						t.Fatalf("old binding revision=%d want%d", got, want+1)
					}
					if got := attachmentRevision(t, tx, "run_b"); got != other+1 {
						t.Fatalf("new binding revision=%d want%d", got, other+1)
					}
					target = "run_b"
				}
				before = attachmentRevision(t, tx, target)
				_, err = tx.ExecContext(t.Context(), tc.delete)
				switch {
				case tc.immutable || tc.table == "stop_results":
					if err == nil {
						t.Fatal("retained source deleted")
					}
				case err != nil:
					t.Fatal(err)
				case tc.table != "runs":
					if got := attachmentRevision(t, tx, target); got != before+1 {
						t.Fatalf("delete revision=%d want%d", got, before+1)
					}
				}
			})
		})
	}
}

func testAttachmentSnapshot(t *testing.T) {
	t.Helper()
	j, dir := openTemp(t)
	seedRun(t, j, "run")
	// A deferred read connection, unlike the journal's immediate writer, can
	// retain a WAL snapshot while a writer commits. No timing-based rendezvous.
	db, err := sql.Open("sqlite", dsn(filepath.Join(dir, DBName), "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	reader, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Rollback() }()
	old := attachmentRevision(t, reader, "run")
	attachmentTx(t, j, func(tx *sql.Tx) {
		if err := SetRunState(t.Context(), tx, "run", "executing", "", time.Unix(1, 0)); err != nil {
			t.Fatal(err)
		}
		if got := attachmentRevision(t, tx, "run"); got != old+1 {
			t.Fatalf("writer revision=%d", got)
		}
		if got := attachmentRevision(t, reader, "run"); got != old {
			t.Fatalf("uncommitted revision visible=%d", got)
		}
	})
	if got := attachmentRevision(t, reader, "run"); got != old {
		t.Fatalf("snapshot revision moved=%d", got)
	}
	state, err := CurrentRunState(t.Context(), reader, "run")
	if err != nil || state != "created" {
		t.Fatalf("snapshot state=%s %v", state, err)
	}
	if err := reader.Commit(); err != nil {
		t.Fatal(err)
	}
	attachmentTx(t, j, func(tx *sql.Tx) {
		if got := attachmentRevision(t, tx, "run"); got != old+1 {
			t.Fatalf("fresh revision=%d", got)
		}
	})
}

func TestRunRevisionErrors(t *testing.T) {
	t.Parallel()
	j, _ := openTemp(t)
	if _, err := RunRevision(t.Context(), nil, "run"); err == nil {
		t.Fatal("nil transaction accepted")
	}
	attachmentTx(t, j, func(tx *sql.Tx) {
		if got, err := RunRevision(t.Context(), tx, "missing"); !errors.Is(err, ErrNotFound) || got != 0 {
			t.Fatalf("missing=%d %v", got, err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := RunRevision(ctx, tx, "missing"); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled=%v", err)
		}
	})
}

func TestImmutableStopAck(t *testing.T) { // I12/I23 (v2 sections 5/6): immutable acceptance, separately reconciled result.
	t.Parallel()
	j, _ := openTemp(t)
	attachmentParents(t, j)
	attachmentTx(t, j, func(tx *sql.Tx) {
		attachmentSQL(t, tx, `INSERT INTO stop_outbox VALUES ('op_a','run_a','att_a','hash_a',123,'synthetic-start',1,'pending')`)
		attachmentSQL(t, tx, `INSERT INTO stop_results VALUES ('op_a','requested','',NULL,1)`)
		var ack string
		if err := tx.QueryRowContext(t.Context(), `SELECT result FROM operations WHERE operation_id='op_a'`).Scan(&ack); err != nil {
			t.Fatal(err)
		}
		for _, stmt := range []string{`UPDATE operations SET result='{"accepted":false}' WHERE operation_id='op_a'`, `DELETE FROM operations WHERE operation_id='op_a'`} {
			if _, err := tx.ExecContext(t.Context(), stmt); err == nil {
				t.Fatalf("immutable acknowledgment accepted %s", stmt)
			}
		}
		before := attachmentRevision(t, tx, "run_a")
		attachmentSQL(t, tx, `UPDATE stop_results SET state='unconfirmed',reason='stop_unconfirmed',result_revision=2 WHERE operation_id='op_a'`)
		if got := attachmentRevision(t, tx, "run_a"); got != before+1 {
			t.Fatalf("stop result revision=%d", got)
		}
		evidence := []struct{ id, run, attempt, kind, payload string }{
			{"wrong_run", "run_b", "att_a", "attempt.stopped", `{"confirmed":true,"unresolved_pids":[]}`},
			{"wrong_attempt", "run_a", "att_b", "attempt.stopped", `{"confirmed":true,"unresolved_pids":[]}`},
			{"wrong_type", "run_a", "att_a", "attempt.native_result", `{"confirmed":true,"unresolved_pids":[]}`},
			{"unconfirmed", "run_a", "att_a", "attempt.stopped", `{"confirmed":false,"unresolved_pids":[]}`},
			{"wrong_shape", "run_a", "att_a", "attempt.stopped", `{"confirmed":1,"unresolved_pids":[]}`},
			{"missing_scan", "run_a", "att_a", "attempt.stopped", `{"confirmed":true}`},
			{"survivor", "run_a", "att_a", "attempt.stopped", `{"confirmed":true,"unresolved_pids":[123]}`},
			{"confirmed", "run_a", "att_a", "attempt.stopped", `{"confirmed":true,"unresolved_pids":[]}`},
		}
		for i, e := range evidence {
			ev := event(e.run, "evidence", int64(i+1), 0)
			ev.EventID = e.id
			ev.AttemptID = e.attempt
			ev.Type = e.kind
			ev.Payload = json.RawMessage(e.payload)
			if err := appendTx(t.Context(), tx, ev, nil); err != nil {
				t.Fatal(err)
			}
			if e.id != "confirmed" {
				if _, err := tx.ExecContext(t.Context(), `UPDATE stop_results SET state='confirmed',reason='',confirming_event_id=?,result_revision=3 WHERE operation_id='op_a'`, e.id); err == nil {
					t.Fatalf("invalid confirming evidence accepted: %s", e.id)
				}
			}
		}
		attachmentSQL(t, tx, `UPDATE stop_results SET state='confirmed',reason='',confirming_event_id='confirmed',result_revision=3 WHERE operation_id='op_a'`)
		for _, stmt := range []string{
			`UPDATE stop_results SET state='requested',confirming_event_id=NULL,result_revision=4`,
			`UPDATE stop_results SET reason='process_lost',result_revision=4`,
			`UPDATE stop_results SET result_revision=4`,
			`INSERT OR REPLACE INTO stop_results VALUES ('op_a','requested','',NULL,1)`,
			`UPDATE stop_outbox SET attempt_id='att_b' WHERE operation_id='op_a'`,
		} {
			if _, err := tx.ExecContext(t.Context(), stmt); err == nil {
				t.Fatalf("invalid evolution accepted: %s", stmt)
			}
		}
		var unchanged string
		if err := tx.QueryRowContext(t.Context(), `SELECT result FROM operations WHERE operation_id='op_a'`).Scan(&unchanged); err != nil {
			t.Fatal(err)
		}
		if unchanged != ack {
			t.Fatalf("ack changed: %s -> %s", ack, unchanged)
		}
	})
}

func TestRunRevisionSelectedColumns(t *testing.T) {
	t.Parallel()
	// Every mutable selected column gets an actual unequal-value write, and a
	// second equal-value write. Savepoints isolate the FK/identity variations.
	cases := []struct {
		table, where, insert string
		values               map[string]string
	}{
		{"runs", "run_id='run_extra'", `INSERT INTO runs SELECT 'run_extra',state,reason,adapter_id,source_repo,source_branch,base_rev,task_sha256,billing_posture,execution_profile,created_at,updated_at FROM runs WHERE run_id='run_a'`, map[string]string{
			"run_id": "'renamed'", "state": "'executing'", "reason": "'changed'", "adapter_id": "'other'", "source_repo": "'other'", "source_branch": "'other'", "base_rev": "'other'", "task_sha256": "'other'", "billing_posture": "'other'", "execution_profile": "'other'",
		}},
		{"attempts", "attempt_id='att_a'", "", map[string]string{
			"attempt_id": "'renamed'", "run_id": "'run_b'", "task_id": "'other'", "attempt_number": "2", "state": "'stopped'", "reason": "'changed'", "launch_token_sha256": "'other'", "worker_pid": "456", "worker_start_time": "'other'", "native_pid": "456", "native_pgid": "456", "native_session_id": "'other'", "workspace_path": "'other'",
		}},
		{"candidates", "attempt_id='att_a'", `INSERT INTO candidates VALUES ('att_a','base','commit','tree','hash','[]','[]',0)`, map[string]string{
			"attempt_id": "'att_b'", "base_rev": "'other'", "candidate_commit": "'other'", "tree_id": "'other'", "patch_sha256": "'other'", "changed_paths": "'[\"file\"]'", "flags": "'[\"flag\"]'", "partial": "1",
		}},
		{"verifications", "verification_id='ver_a'", "", map[string]string{
			"verification_id": "'renamed'", "run_id": "'run_b'", "candidate_commit": "'other'", "config_sha256": "'other'", "result": "'failed'", "started_at": "'other'", "finished_at": "'other'",
		}},
		{"check_results", "verification_id='ver_a'", `INSERT INTO check_results VALUES ('ver_a','unit','[]','unknown',NULL,NULL,NULL,NULL)`, map[string]string{
			"verification_id": "'ver_b'", "name": "'other'", "argv": "'[\"other\"]'", "status": "'failed'", "exit_code": "1", "duration_ms": "10", "evidence_path": "'other'", "evidence_sha256": "'other'",
		}},
		{"applies", "apply_id='apply'", `INSERT INTO applies VALUES ('apply','run_a','repo','branch','candidate','pending',NULL,'now')`, map[string]string{
			"apply_id": "'other'", "run_id": "'run_b'", "target_repo": "'other'", "branch": "'other'", "candidate_commit": "'other'", "state": "'applied'", "reason": "'changed'", "created_at": "'other'",
		}},
		{"run_assignments", "run_id='run_a'", `INSERT INTO run_assignments VALUES ('run_a','supervisor','now','op_a')`, map[string]string{
			"run_id": "'run_b'", "owner": "'other'", "assigned_at": "'other'", "operation_id": "'op_b'",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.table, func(t *testing.T) {
			t.Parallel()
			j, _ := openTemp(t)
			attachmentParents(t, j)
			attachmentTx(t, j, func(tx *sql.Tx) {
				run := "run_a"
				if tc.table == "runs" {
					run = "run_extra"
				}
				if tc.insert != "" {
					attachmentSQL(t, tx, tc.insert)
				}
				for column, value := range tc.values {
					attachmentSQL(t, tx, `SAVEPOINT selected_column`)
					old := attachmentRevision(t, tx, run)
					attachmentSQL(t, tx, "UPDATE "+tc.table+" SET "+column+"="+value+" WHERE "+tc.where)
					target := run
					if tc.table == "runs" && column == "run_id" {
						target = "renamed"
					}
					if got := attachmentRevision(t, tx, target); got != old+1 {
						t.Fatalf("%s.%s changed revision=%d want%d", tc.table, column, got, old+1)
					}
					if target != run {
						attachmentSQL(t, tx, "UPDATE "+tc.table+" SET "+column+"="+column+" WHERE run_id='renamed'")
					} else {
						// Original key can move for an identity-column variation, so apply the
						// equal-value write to the whole isolated fixture table.
						attachmentSQL(t, tx, "UPDATE "+tc.table+" SET "+column+"="+column)
					}
					if got := attachmentRevision(t, tx, target); got != old+1 {
						t.Fatalf("%s.%s equal write advanced revision=%d", tc.table, column, got)
					}
					attachmentSQL(t, tx, `ROLLBACK TO selected_column`)
					attachmentSQL(t, tx, `RELEASE selected_column`)
				}
			})
		})
	}
}

func TestAttachmentAuthorityAndDispatchStorage(t *testing.T) {
	t.Parallel()
	j, _ := openTemp(t)
	attachmentParents(t, j)
	attachmentTx(t, j, func(tx *sql.Tx) {
		initial := attachmentRevision(t, tx, "run_a")
		attachmentSQL(t, tx, `INSERT INTO attachment_grants VALUES ('attachment','token-digest','client','run_a','repository',1,0,0)`)
		attachmentSQL(t, tx, `INSERT INTO attachment_projections VALUES ('attachment',1,1,?,'content-digest','{}')`, initial)
		attachmentSQL(t, tx, `UPDATE attachment_grants SET revoked=1,revocation_revision=1 WHERE attachment_id='attachment'`)
		attachmentSQL(t, tx, `INSERT INTO prepared_runs VALUES ('preparation','peer',1,'binding',0,'{"schema_version":2}','[]','[]',1,0,'expires')`)
		attachmentSQL(t, tx, `INSERT INTO dispatch_outbox VALUES ('op_a','preparation','peer',1,'binding','evidence','run_a','att_a','hash_a','pending')`)
		attachmentSQL(t, tx, `UPDATE dispatch_outbox SET delivery_state='dispatched' WHERE operation_id='op_a'`)
		attachmentSQL(t, tx, `INSERT INTO prepared_runs VALUES ('unused-preparation','peer',1,'binding',0,'{"schema_version":2}','[]','[]',1,0,'expires')`)
		if got := attachmentRevision(t, tx, "run_a"); got != initial {
			t.Fatalf("authority/cache/dispatch bookkeeping changed execution revision=%d", got)
		}
		for _, stmt := range []string{
			`INSERT INTO attachment_grants VALUES ('duplicate','token-digest','client','run_a','repository',1,0,0)`,
			`INSERT INTO attachment_grants VALUES ('bad-generation','other-digest','client','run_a','repository',0,0,0)`,
			`INSERT INTO attachment_projections VALUES ('attachment',1,2,1,'other','{}')`,
			`INSERT INTO stop_outbox VALUES ('op_b','run_a','att_b','hash_b',123,'synthetic-start',1,'pending')`,
			`INSERT INTO stop_outbox VALUES ('op_b','run_b','att_b','wrong-hash',123,'synthetic-start',1,'pending')`,
			`INSERT INTO stop_outbox VALUES ('op_b','run_b','att_b','hash_b',999,'synthetic-start',1,'pending')`,
			`INSERT INTO stop_outbox VALUES ('op_b','run_b','att_b','hash_b',123,'wrong-start',1,'pending')`,
			// Each binding probe uses an unused preparation, operation and attempt;
			// an unrelated UNIQUE conflict must not make these cases pass.
			`INSERT INTO dispatch_outbox VALUES ('op_b','unused-preparation','foreign-peer',1,'binding','evidence','run_b','att_b','hash_b','pending')`,
			`INSERT INTO dispatch_outbox VALUES ('op_b','unused-preparation','peer',2,'binding','evidence','run_b','att_b','hash_b','pending')`,
			`INSERT INTO dispatch_outbox VALUES ('op_b','unused-preparation','peer',1,'other-binding','evidence','run_b','att_b','hash_b','pending')`,
			`INSERT INTO dispatch_outbox VALUES ('op_b','unused-preparation','peer',1,'binding','evidence','run_a','att_b','hash_b','pending')`,
			`INSERT INTO dispatch_outbox VALUES ('op_b','unused-preparation','peer',1,'binding','evidence','run_b','att_b','other-hash','pending')`,
			`UPDATE dispatch_outbox SET launch_token_sha256='other'`,
			`DELETE FROM dispatch_outbox`,
		} {
			if _, err := tx.ExecContext(t.Context(), stmt); err == nil {
				t.Fatalf("invalid authority/binding accepted: %s", stmt)
			}
		}
		attachmentSQL(t, tx, `INSERT INTO dispatch_outbox VALUES ('op_b','unused-preparation','peer',1,'binding','evidence','run_b','att_b','hash_b','pending')`)
		attachmentSQL(t, tx, `INSERT INTO stop_outbox VALUES ('op_b','run_b','att_b','hash_b',123,'synthetic-start',1,'pending')`)
		for _, stmt := range []string{
			`INSERT INTO stop_results VALUES ('op_b','confirmed','','missing',1)`,
			`INSERT INTO stop_results VALUES ('op_b','unconfirmed','stop_unconfirmed',NULL,1)`,
			`INSERT INTO stop_results VALUES ('op_b','requested','raw native message',NULL,1)`,
			`INSERT INTO stop_results VALUES ('op_b','requested','',NULL,0)`,
			`INSERT OR REPLACE INTO stop_outbox VALUES ('op_b','run_b','att_b','hash_b',123,'synthetic-start',1,'pending')`,
			`DELETE FROM stop_outbox WHERE operation_id='op_b'`,
		} {
			if _, err := tx.ExecContext(t.Context(), stmt); err == nil {
				t.Fatalf("invalid initial stop/identity accepted: %s", stmt)
			}
		}
		attachmentSQL(t, tx, `INSERT INTO stop_results VALUES ('op_b','requested','',NULL,1)`)
		for _, stmt := range []string{
			`UPDATE stop_results SET state='unconfirmed',reason='stop_unconfirmed',result_revision=1 WHERE operation_id='op_b'`,
			`UPDATE stop_results SET state='unconfirmed',reason='stop_unconfirmed',result_revision=3 WHERE operation_id='op_b'`,
			`UPDATE run_control_revisions SET revision=0 WHERE run_id='run_a'`,
		} {
			if _, err := tx.ExecContext(t.Context(), stmt); err == nil {
				t.Fatalf("invalid revision accepted: %s", stmt)
			}
		}
	})
}

func TestRunRevisionActualProjectionWriters(t *testing.T) {
	t.Parallel()
	j, _ := openTemp(t)
	seedRun(t, j, "writers")
	attachmentTx(t, j, func(tx *sql.Tx) {
		expect := func(delta uint64, write func() error) {
			t.Helper()
			before := attachmentRevision(t, tx, "writers")
			if err := write(); err != nil {
				t.Fatal(err)
			}
			if got := attachmentRevision(t, tx, "writers"); got != before+delta {
				t.Fatalf("writer revision=%d want%d", got, before+delta)
			}
		}
		expect(1, func() error { return SetRunState(t.Context(), tx, "writers", "executing", "", time.Unix(1, 0)) })
		expect(1, func() error {
			return InsertAttempt(t.Context(), tx, AttemptRow{AttemptID: "actual", RunID: "writers", TaskID: "task", AttemptNumber: 1, State: "running", LaunchTokenSHA256: "digest", WorkspacePath: "synthetic"})
		})
		expect(1, func() error { return SetAttemptState(t.Context(), tx, "actual", "stopped", "") })
		expect(1, func() error {
			return SetAttemptProcesses(t.Context(), tx, "actual", LaunchedProcesses{WorkerPID: 123, WorkerStartTime: time.Unix(1, 0)})
		})
		expect(1, func() error { return SetNativeSession(t.Context(), tx, "actual", "native-session") })
		expect(0, func() error { return SetSpoolOffset(t.Context(), tx, "actual", 99) })
		expect(1, func() error {
			return InsertCandidate(t.Context(), tx, CandidateRow{AttemptID: "actual", BaseRev: "base", Commit: "candidate", Tree: "tree", PatchSHA256: "patch", ChangedPaths: json.RawMessage(`[]`), Flags: json.RawMessage(`[]`)})
		})
		// InsertVerification writes its row, immutable binding and every check in
		// this transaction. Evaluator side data is not a separate projection input;
		// protected acceptance depends on the immutable binding/attestation.
		verification := verificationRow("writers", "actual-verification")
		verification.Checks = []CheckRow{{Name: "unit", Status: "unknown", Argv: []string{}}}
		expect(3, func() error { return InsertVerification(t.Context(), tx, verification) })
	})
}

func TestRunRevisionRejectsReplacementAliases(t *testing.T) {
	t.Parallel()
	// Both primary-key and secondary UNIQUE collisions can implicitly delete
	// an old run's selected data while inserting into a different run.
	cases := []struct{ name, setup, replace string }{
		{"run", `INSERT INTO runs SELECT 'run_extra',state,reason,adapter_id,source_repo,source_branch,base_rev,task_sha256,billing_posture,execution_profile,created_at,updated_at FROM runs WHERE run_id='run_a'`, `INSERT OR REPLACE INTO runs SELECT run_id,'failed',reason,adapter_id,source_repo,source_branch,base_rev,task_sha256,billing_posture,execution_profile,created_at,updated_at FROM runs WHERE run_id='run_extra'`},
		{"attempt primary key", "", `INSERT OR REPLACE INTO attempts (attempt_id,run_id,task_id,attempt_number,state,launch_token_sha256,workspace_path) VALUES ('att_a','run_b','other',3,'running','hash_a','synthetic')`},
		{"attempt compound key", "", `INSERT OR REPLACE INTO attempts (attempt_id,run_id,task_id,attempt_number,state,launch_token_sha256,workspace_path) VALUES ('replacement','run_a','task',1,'running','hash_a','synthetic')`},
		{"candidate", `INSERT INTO candidates VALUES ('att_a','base','commit','tree','hash','[]','[]',0)`, `INSERT OR REPLACE INTO candidates VALUES ('att_a','other','other','other','other','[]','[]',1)`},
		{"verification", "", `INSERT OR REPLACE INTO verifications VALUES ('ver_a','run_b','other','other','failed','now',NULL)`},
		{"verification binding", `INSERT INTO verification_bindings VALUES ('ver_a','',NULL,NULL,NULL)`, `INSERT OR REPLACE INTO verification_bindings VALUES ('ver_a','other',NULL,NULL,NULL)`},
		{"check result", `INSERT INTO check_results VALUES ('ver_a','unit','[]','unknown',NULL,NULL,NULL,NULL)`, `INSERT OR REPLACE INTO check_results VALUES ('ver_a','unit','[]','failed',1,2,'path','hash')`},
		{"apply", `INSERT INTO applies VALUES ('apply','run_a','repo','branch','candidate','pending',NULL,'now')`, `INSERT OR REPLACE INTO applies VALUES ('apply','run_b','repo','branch','other','applied',NULL,'now')`},
		{"assignment", `INSERT INTO run_assignments VALUES ('run_a','supervisor','now','op_a')`, `INSERT OR REPLACE INTO run_assignments VALUES ('run_a','other','later','op_b')`},
		{"journal row key", `INSERT INTO journal VALUES (1,'event',1,'run_a',NULL,NULL,'p',1,1,0,NULL,'now','observed','{}')`, `INSERT OR REPLACE INTO journal VALUES (1,'other',1,'run_b',NULL,NULL,'other',2,2,0,NULL,'now','observed','{}')`},
		{"journal event key", `INSERT INTO journal VALUES (1,'event',1,'run_a',NULL,NULL,'p',1,1,0,NULL,'now','observed','{}')`, `INSERT OR REPLACE INTO journal VALUES (2,'event',1,'run_b',NULL,NULL,'other',2,2,0,NULL,'now','observed','{}')`},
		{"journal producer key", `INSERT INTO journal VALUES (1,'event',1,'run_a',NULL,NULL,'p',1,1,0,NULL,'now','observed','{}')`, `INSERT OR REPLACE INTO journal VALUES (2,'other',1,'run_b',NULL,NULL,'p',1,2,0,NULL,'now','observed','{}')`},
		{"journal run sequence key", `INSERT INTO journal VALUES (1,'event',1,'run_a',NULL,NULL,'p',1,1,0,NULL,'now','observed','{}')`, `INSERT OR REPLACE INTO journal VALUES (2,'other',1,'run_a',NULL,NULL,'other',2,1,0,NULL,'now','observed','{}')`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			j, _ := openTemp(t)
			attachmentParents(t, j)
			attachmentTx(t, j, func(tx *sql.Tx) {
				attachmentSQL(t, tx, `PRAGMA recursive_triggers=OFF`)
				if tc.setup != "" {
					attachmentSQL(t, tx, tc.setup)
				}
				oldA, oldB := attachmentRevision(t, tx, "run_a"), attachmentRevision(t, tx, "run_b")
				if _, err := tx.ExecContext(t.Context(), tc.replace); err == nil {
					t.Fatalf("implicit replacement accepted: %s", tc.name)
				}
				if got := attachmentRevision(t, tx, "run_a"); got != oldA {
					t.Fatalf("failed replacement changed old run revision=%d", got)
				}
				if got := attachmentRevision(t, tx, "run_b"); got != oldB {
					t.Fatalf("failed replacement changed new run revision=%d", got)
				}
			})
		})
	}
}

func TestImmutableStopAckDuplicateInsertKeepsWholeRow(t *testing.T) {
	t.Parallel()
	j, _ := openTemp(t)
	attachmentParents(t, j)
	attachmentTx(t, j, func(tx *sql.Tx) {
		attachmentSQL(t, tx, `INSERT INTO stop_outbox VALUES ('op_a','run_a','att_a','hash_a',123,'synthetic-start',1,'pending')`)
		// Compare all persisted operation fields, including identities/timestamps;
		// preserving result alone could still corrupt replay or revision authority.
		read := func() string {
			t.Helper()
			var row string
			if err := tx.QueryRowContext(t.Context(), `SELECT json_array(operation_id,method,object,digest,state,revision,result,claimed_at,finished_at) FROM operations WHERE operation_id='op_a'`).Scan(&row); err != nil {
				t.Fatal(err)
			}
			return row
		}
		before := read()
		for _, stmt := range []string{
			`INSERT OR REPLACE INTO operations VALUES ('op_a','other','run_b','other','done',99,'{"accepted":false}','later','later')`,
			`INSERT OR REPLACE INTO operations VALUES ('op_a','other','run_b','other','claimed',0,NULL,'later',NULL)`,
			`INSERT INTO operations (operation_id,method,object,digest,state,claimed_at) VALUES ('op_a','other','run_b','other','claimed','later') ON CONFLICT(operation_id) DO NOTHING`,
		} {
			result, err := tx.ExecContext(t.Context(), stmt)
			if err != nil {
				t.Fatalf("duplicate must be ignored: %v", err)
			}
			count, err := result.RowsAffected()
			if err != nil || count != 0 {
				t.Fatalf("duplicate rowsaffected=%d %v", count, err)
			}
			if got := read(); got != before {
				t.Fatalf("completed operation changed: %s -> %s", before, got)
			}
		}
	})
}

func TestRunRevisionRejectsUpdateReplacementAliases(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, setup, update string }{
		{"runs", `INSERT INTO runs SELECT 'run_new',state,reason,adapter_id,source_repo,source_branch,base_rev,task_sha256,billing_posture,execution_profile,created_at,updated_at FROM runs WHERE run_id='run_a'; INSERT INTO runs SELECT 'run_other',state,reason,adapter_id,source_repo,source_branch,base_rev,task_sha256,billing_posture,execution_profile,created_at,updated_at FROM runs WHERE run_id='run_a'`, `UPDATE OR REPLACE runs SET run_id='run_other' WHERE run_id='run_new'`},
		{"attempt primary key", "", `UPDATE OR REPLACE attempts SET attempt_id='att_b' WHERE attempt_id='att_a'`},
		{"attempt compound key", "", `UPDATE OR REPLACE attempts SET run_id='run_b',attempt_number=2 WHERE attempt_id='att_a'`},
		{"candidate", `INSERT INTO candidates VALUES ('att_a','base','commit','tree','hash','[]','[]',0),('att_b','base','other','tree','hash','[]','[]',0)`, `UPDATE OR REPLACE candidates SET attempt_id='att_b' WHERE attempt_id='att_a'`},
		{"verification", "", `UPDATE OR REPLACE verifications SET verification_id='ver_b' WHERE verification_id='ver_a'`},
		{"verification binding", `INSERT INTO verification_bindings VALUES ('ver_a','',NULL,NULL,NULL),('ver_b','',NULL,NULL,NULL)`, `UPDATE OR REPLACE verification_bindings SET verification_id='ver_b' WHERE verification_id='ver_a'`},
		{"check result", `INSERT INTO check_results VALUES ('ver_a','unit','[]','unknown',NULL,NULL,NULL,NULL),('ver_b','unit','[]','unknown',NULL,NULL,NULL,NULL)`, `UPDATE OR REPLACE check_results SET verification_id='ver_b' WHERE verification_id='ver_a'`},
		{"apply", `INSERT INTO applies VALUES ('apply_a','run_a','repo','branch','candidate','pending',NULL,'now'),('apply_b','run_b','repo','branch','other','pending',NULL,'now')`, `UPDATE OR REPLACE applies SET apply_id='apply_b' WHERE apply_id='apply_a'`},
		{"assignment", `INSERT INTO run_assignments VALUES ('run_a','supervisor','now','op_a'),('run_b','supervisor','now','op_b')`, `UPDATE OR REPLACE run_assignments SET run_id='run_b' WHERE run_id='run_a'`},
		{"journal", `INSERT INTO journal VALUES (1,'event',1,'run_a',NULL,NULL,'p',1,1,0,NULL,'now','observed','{}'),(2,'other',1,'run_b',NULL,NULL,'q',1,1,0,NULL,'now','observed','{}')`, `UPDATE OR REPLACE journal SET seq=2 WHERE seq=1`},
		{"stop result", `INSERT INTO stop_outbox VALUES ('op_a','run_a','att_a','hash_a',123,'synthetic-start',1,'pending'),('op_b','run_b','att_b','hash_b',123,'synthetic-start',1,'pending'); INSERT INTO stop_results VALUES ('op_a','requested','',NULL,1),('op_b','requested','',NULL,1)`, `UPDATE OR REPLACE stop_results SET operation_id='op_b' WHERE operation_id='op_a'`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			j, _ := openTemp(t)
			attachmentParents(t, j)
			attachmentTx(t, j, func(tx *sql.Tx) {
				attachmentSQL(t, tx, `PRAGMA recursive_triggers=OFF`)
				if tc.setup != "" {
					attachmentSQL(t, tx, tc.setup)
				}
				var before string
				// Capture every selected row as serialised SQL values, not just the
				// revision: rejection must preserve the victim and the source.
				table := strings.Fields(tc.update)[3]
				snapshot := func() string { return attachmentTableSnapshot(t, tx, table) }
				before = snapshot()
				a, b := attachmentRevision(t, tx, "run_a"), attachmentRevision(t, tx, "run_b")
				if _, err := tx.ExecContext(t.Context(), tc.update); err == nil {
					t.Fatalf("UPDATE replacement accepted: %s", tc.name)
				}
				if after := snapshot(); after != before {
					t.Fatalf("rejected replacement changed source/victim: %s -> %s", before, after)
				}
				if attachmentRevision(t, tx, "run_a") != a || attachmentRevision(t, tx, "run_b") != b {
					t.Fatal("rejected replacement advanced revisions")
				}
			})
		})
	}
}

func TestImmutableStopAckUpdateReplacementKeepsWholeRow(t *testing.T) {
	t.Parallel()
	j, _ := openTemp(t)
	attachmentParents(t, j)
	attachmentTx(t, j, func(tx *sql.Tx) {
		attachmentSQL(t, tx, `PRAGMA recursive_triggers=OFF`)
		attachmentSQL(t, tx, `INSERT INTO stop_outbox VALUES ('op_a','run_a','att_a','hash_a',123,'synthetic-start',1,'pending')`)
		attachmentSQL(t, tx, `INSERT INTO operations(operation_id,method,object,digest,state,claimed_at) VALUES ('fresh','probe','','fresh-digest','claimed','fresh')`)
		read := func() string {
			t.Helper()
			var row string
			if err := tx.QueryRowContext(t.Context(), `SELECT json_group_array(json_array(operation_id,method,object,digest,state,revision,result,claimed_at,finished_at)) FROM (SELECT * FROM operations ORDER BY operation_id)`).Scan(&row); err != nil {
				t.Fatal(err)
			}
			return row
		}
		before := read()
		if _, err := tx.ExecContext(t.Context(), `UPDATE OR REPLACE operations SET operation_id='op_a' WHERE operation_id='fresh'`); err == nil {
			t.Fatal("claimed row replaced completed acknowledgment")
		}
		if after := read(); after != before {
			t.Fatalf("UPDATE replacement changed operation rows: %s -> %s", before, after)
		}
		// Renaming a fresh claim to an unused key and completing it remain valid.
		attachmentSQL(t, tx, `UPDATE operations SET operation_id='unused' WHERE operation_id='fresh'`)
		attachmentSQL(t, tx, `UPDATE operations SET state='done',result='{}',finished_at='finished' WHERE operation_id='unused'`)
	})
}

func attachmentTableSnapshot(t *testing.T, tx *sql.Tx, table string) string {
	t.Helper()
	rows, err := tx.QueryContext(t.Context(), "SELECT rowid,* FROM "+table+" ORDER BY rowid") //nolint:gosec // G202: table comes only from the closed synthetic fixture inventory.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var result []string
	for rows.Next() {
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			t.Fatal(err)
		}
		result = append(result, fmt.Sprint(values))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(result, "\n")
}

func attachmentAliasWorld(t *testing.T, j *Journal, table string) {
	t.Helper()
	attachmentParents(t, j)
	seedRun(t, j, "run_free")
	attachmentTx(t, j, func(tx *sql.Tx) {
		attachmentSQL(t, tx, `INSERT INTO attempts(attempt_id,run_id,task_id,attempt_number,state,launch_token_sha256,worker_pid,worker_start_time,workspace_path) VALUES ('att_free','run_b','free',3,'running','hash_free',123,'synthetic-start','synthetic')`)
		attachmentSQL(t, tx, `INSERT INTO verifications VALUES ('ver_free','run_b','c','cfg','unknown','now',NULL)`)
		attachmentSQL(t, tx, `INSERT INTO operations VALUES ('op_free','probe','','digest','done',1,'{}','now','now'),('op_new','probe','','digest','done',1,'{}','now','now')`)
		attachmentSQL(t, tx, `INSERT INTO operations(operation_id,method,object,digest,state,claimed_at) VALUES ('claim','probe','','digest','claimed','now')`)
		for _, suffix := range []string{"a", "b"} {
			attachmentSQL(t, tx, `INSERT INTO candidates VALUES (?,'base','candidate','tree','hash','[]','[]',0)`, "att_"+suffix)
			attachmentSQL(t, tx, `INSERT INTO verification_bindings VALUES (?,'',NULL,NULL,NULL)`, "ver_"+suffix)
			attachmentSQL(t, tx, `INSERT INTO check_results VALUES (?,'unit','[]','unknown',NULL,NULL,NULL,NULL)`, "ver_"+suffix)
			attachmentSQL(t, tx, `INSERT INTO applies VALUES (?,?,'repo','branch','candidate','pending',NULL,'now')`, "apply_"+suffix, "run_"+suffix)
			attachmentSQL(t, tx, `INSERT INTO run_assignments VALUES (?,'supervisor','now',?)`, "run_"+suffix, "op_"+suffix)
			attachmentSQL(t, tx, `INSERT INTO stop_outbox VALUES (?,?,?, ?,123,'synthetic-start',1,'pending')`, "op_"+suffix, "run_"+suffix, "att_"+suffix, "hash_"+suffix)
			attachmentSQL(t, tx, `INSERT INTO stop_results VALUES (?,'requested','',NULL,1)`, "op_"+suffix)
			attachmentSQL(t, tx, `INSERT INTO prepared_runs VALUES (?,'peer',1,'binding',0,'{}','[]','[]',1,0,'expires')`, "prep_"+suffix)
			attachmentSQL(t, tx, `INSERT INTO dispatch_outbox VALUES (?,?,'peer',1,'binding','evidence',?,?,?,'pending')`, "op_"+suffix, "prep_"+suffix, "run_"+suffix, "att_"+suffix, "hash_"+suffix)
		}
		attachmentSQL(t, tx, `INSERT INTO prepared_runs VALUES ('prep_free','peer',1,'binding',0,'{}','[]','[]',1,0,'expires')`)
		if table == "stop_results" || table == "stop_outbox" {
			attachmentSQL(t, tx, `INSERT INTO stop_outbox VALUES ('op_free','run_b','att_b','hash_b',123,'synthetic-start',1,'pending')`)
		}
	})
}

func TestAttachmentHiddenIdentityCollisions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		table, pk, source, victim string
		fresh                     map[string]string
	}{
		{"runs", "run_id", "run_b", "run_free", map[string]string{"run_id": "'run_new'"}},
		{"attempts", "attempt_id", "att_b", "att_free", map[string]string{"attempt_id": "'att_new'", "task_id": "'new-task'", "attempt_number": "99"}},
		{"candidates", "attempt_id", "att_b", "att_a", map[string]string{"attempt_id": "'att_free'"}},
		{"verifications", "verification_id", "ver_b", "ver_free", map[string]string{"verification_id": "'ver_new'"}},
		{"verification_bindings", "verification_id", "ver_b", "ver_a", map[string]string{"verification_id": "'ver_free'"}},
		{"check_results", "verification_id", "ver_b", "ver_a", map[string]string{"name": "'new-check'"}},
		{"applies", "apply_id", "apply_b", "apply_a", map[string]string{"apply_id": "'apply_new'"}},
		{"run_assignments", "run_id", "run_b", "run_a", map[string]string{"run_id": "'run_free'"}},
		{"stop_results", "operation_id", "op_b", "op_a", map[string]string{"operation_id": "'op_free'"}},
		{"stop_outbox", "operation_id", "op_b", "op_free", map[string]string{"operation_id": "'op_new'"}},
		{"dispatch_outbox", "operation_id", "op_b", "op_a", map[string]string{"operation_id": "'op_free'", "preparation_id": "'prep_free'", "attempt_id": "'att_free'", "launch_token_sha256": "'hash_free'"}},
		// A completed victim is deliberately unreferenced: a foreign-key refusal
		// must not substitute for the immutable-operation collision guard.
		{"operations", "operation_id", "claim", "op_free", map[string]string{"operation_id": "'new_claim'"}},
	}
	for _, tc := range cases {
		t.Run(tc.table, func(t *testing.T) {
			t.Parallel()
			j, _ := openTemp(t)
			attachmentAliasWorld(t, j, tc.table)
			attachmentTx(t, j, func(tx *sql.Tx) {
				attachmentSQL(t, tx, `PRAGMA recursive_triggers=OFF`)
				var victimRow int64
				if err := tx.QueryRowContext(t.Context(), "SELECT rowid FROM "+tc.table+" WHERE "+tc.pk+"=?", tc.victim).Scan(&victimRow); err != nil {
					t.Fatal(err)
				}
				if victimRow <= 0 {
					t.Fatalf("normal writer allocated rowid %d", victimRow)
				}
				rows, err := tx.QueryContext(t.Context(), "SELECT * FROM "+tc.table+" LIMIT 0") //nolint:gosec // G202: table comes only from the closed synthetic fixture inventory.
				if err != nil {
					t.Fatal(err)
				}
				cols, err := rows.Columns()
				_ = rows.Close()
				if err != nil {
					t.Fatal(err)
				}
				values := make([]string, len(cols))
				for i, column := range cols {
					values[i] = column
					if v, ok := tc.fresh[column]; ok {
						values[i] = v
					}
				}
				before := attachmentTableSnapshot(t, tx, tc.table)
				revisions := attachmentTableSnapshot(t, tx, "run_control_revisions")
				a, b := attachmentRevision(t, tx, "run_a"), attachmentRevision(t, tx, "run_b")
				for _, alias := range []string{"rowid", "_rowid_", "oid"} {
					stmt := "INSERT OR REPLACE INTO " + tc.table + " (" + alias + "," + strings.Join(cols, ",") + ") SELECT ?," + strings.Join(values, ",") + " FROM " + tc.table + " WHERE " + tc.pk + "=?" //nolint:gosec // G202: identifiers and expressions come only from the closed synthetic fixture inventory.
					result, err := tx.ExecContext(t.Context(), stmt, victimRow, tc.source)
					if tc.table == "operations" {
						if err != nil {
							t.Fatal(err)
						}
						count, err := result.RowsAffected()
						if err != nil || count != 0 {
							t.Fatalf("completed row collision affected=%d %v", count, err)
						}
					} else if err == nil {
						t.Fatalf("%s INSERT replacement accepted", alias)
					}
					if after := attachmentTableSnapshot(t, tx, tc.table); after != before {
						t.Fatalf("%s INSERT changed source/victim", alias)
					}
					if _, err := tx.ExecContext(t.Context(), "UPDATE OR REPLACE "+tc.table+" SET "+alias+"=? WHERE "+tc.pk+"=?", victimRow, tc.source); err == nil { //nolint:gosec // G202: identifiers and expressions come only from the closed synthetic fixture inventory.
						t.Fatalf("%s UPDATE replacement accepted", alias)
					}
					if after := attachmentTableSnapshot(t, tx, tc.table); after != before {
						t.Fatalf("%s UPDATE changed source/victim", alias)
					}
					if attachmentRevision(t, tx, "run_a") != a || attachmentRevision(t, tx, "run_b") != b {
						t.Fatalf("%s rejected alias changed revisions", alias)
					}
					if attachmentTableSnapshot(t, tx, "run_control_revisions") != revisions {
						t.Fatalf("%s rejected alias changed a source or victim revision", alias)
					}
				}
			})
		})
	}
}

func TestNegativePhysicalIdentityCollisionFailsClosed(t *testing.T) {
	t.Parallel()
	j, _ := openTemp(t)
	attachmentParents(t, j)
	attachmentTx(t, j, func(tx *sql.Tx) {
		attachmentSQL(t, tx, `PRAGMA recursive_triggers=OFF`)
		attachmentSQL(t, tx, `UPDATE attempts SET rowid=-1 WHERE attempt_id='att_a'`)
		attachmentSQL(t, tx, `INSERT INTO operations(operation_id,method,object,digest,state,claimed_at) VALUES ('fresh','probe','','fresh-digest','claimed','fresh')`)
		attachmentSQL(t, tx, `INSERT INTO operations(rowid,operation_id,method,object,digest,state,result,claimed_at,finished_at) VALUES (-1,'negative','probe','','digest','done','{}','now','now')`)
		attempts := attachmentTableSnapshot(t, tx, "attempts")
		operations := attachmentTableSnapshot(t, tx, "operations")
		a, b := attachmentRevision(t, tx, "run_a"), attachmentRevision(t, tx, "run_b")
		for _, stmt := range []string{
			`INSERT OR REPLACE INTO attempts(rowid,attempt_id,run_id,task_id,attempt_number,state,launch_token_sha256,workspace_path) VALUES (-1,'new','run_b','free',3,'running','hash','workspace')`,
			`UPDATE OR REPLACE attempts SET rowid=-1 WHERE attempt_id='att_b'`,
		} {
			if _, err := tx.ExecContext(t.Context(), stmt); err == nil {
				t.Fatalf("negative victim overwritten: %s", stmt)
			}
		}
		result, err := tx.ExecContext(t.Context(), `INSERT OR REPLACE INTO operations(rowid,operation_id,method,object,digest,state,claimed_at) VALUES (-1,'new','probe','','digest','claimed','now')`)
		if err != nil {
			t.Fatal(err)
		}
		affected, err := result.RowsAffected()
		if err != nil || affected != 0 {
			t.Fatalf("negative completed collision affected=%d %v", affected, err)
		}
		if _, err := tx.ExecContext(t.Context(), `UPDATE OR REPLACE operations SET rowid=-1 WHERE operation_id='fresh'`); err == nil {
			t.Fatal("negative completed victim overwritten by UPDATE")
		}
		if attachmentTableSnapshot(t, tx, "attempts") != attempts || attachmentTableSnapshot(t, tx, "operations") != operations {
			t.Fatal("negative collision changed protected rows")
		}
		if attachmentRevision(t, tx, "run_a") != a || attachmentRevision(t, tx, "run_b") != b {
			t.Fatal("negative collision changed revisions")
		}
	})
}

func TestDispatchImmutableUniqueCollisions(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, insert string }{
		{"operation", `INSERT OR REPLACE INTO dispatch_outbox SELECT * FROM dispatch_outbox WHERE operation_id='op_a'`},
		{"preparation", `INSERT OR REPLACE INTO dispatch_outbox VALUES ('op_free','prep_a','peer',1,'binding','evidence','run_b','att_free','hash_free','pending')`},
		{"attempt", `INSERT OR REPLACE INTO dispatch_outbox VALUES ('op_free','prep_free','peer',1,'binding','evidence','run_a','att_a','hash_a','pending')`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			j, _ := openTemp(t)
			attachmentAliasWorld(t, j, "dispatch_outbox")
			attachmentTx(t, j, func(tx *sql.Tx) {
				attachmentSQL(t, tx, `PRAGMA recursive_triggers=OFF`)
				before := attachmentTableSnapshot(t, tx, "dispatch_outbox")
				revisions := attachmentTableSnapshot(t, tx, "run_control_revisions")
				if _, err := tx.ExecContext(t.Context(), tc.insert); err == nil {
					t.Fatalf("dispatch %s identity replaced", tc.name)
				}
				if attachmentTableSnapshot(t, tx, "dispatch_outbox") != before || attachmentTableSnapshot(t, tx, "run_control_revisions") != revisions {
					t.Fatal("refused dispatch replacement changed identity or revisions")
				}
			})
		})
	}
}
