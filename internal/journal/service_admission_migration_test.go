package journal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestServiceAdmissionMigration keeps the schema11 authority records additive;
// old preparations have no immutable binding and cannot become consumable.
func TestServiceAdmissionMigration(t *testing.T) { // I09/I12/I20/I23 (v2 §§5/6/8)
	t.Parallel()
	t.Run("new schema and historical rows", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		old, err := open(t.Context(), dir, migrations[:10])
		if err != nil {
			t.Fatal(err)
		}
		seedRun(t, old, "historical")
		attachmentTx(t, old, func(tx *sql.Tx) {
			attachmentSQL(t, tx, `INSERT INTO prepared_runs VALUES ('legacy','peer',1,'old-digest',1,'{}','[]','[]',1,0,'2027-01-01T00:00:00Z')`)
		})
		before := schemaObjects(t, old.db)
		if err := old.Close(); err != nil {
			t.Fatal(err)
		}
		j, err := Open(t.Context(), dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = j.Close() }()
		if SchemaVersion != 11 {
			t.Fatalf("schema=%d want11", SchemaVersion)
		}
		after := schemaObjects(t, j.db)
		for name, ddl := range before {
			if after[name] != ddl {
				t.Errorf("released schema %s changed", name)
			}
		}
		for _, name := range []string{"service_admission_bindings", "service_admission_consumptions", "service_pending_dispatch", "service_dispatch_claims"} {
			if !strings.Contains(after[name], "STRICT") {
				t.Errorf("missing STRICT table %s", name)
			}
		}
		var n int
		if err := j.db.QueryRowContext(t.Context(), `SELECT count(*) FROM service_admission_bindings`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatal("historical preparation was promoted into a binding")
		}
		var ready, consumed int
		if err := j.db.QueryRowContext(t.Context(), `SELECT ready, consumed FROM prepared_runs WHERE preparation_id='legacy'`).Scan(&ready, &consumed); err != nil {
			t.Fatal(err)
		}
		if ready != 1 || consumed != 0 {
			t.Fatal("historical preparation was rewritten")
		}
	})
	t.Run("future schema is unchanged", func(t *testing.T) {
		t.Parallel()
		j, dir := openTemp(t)
		if _, err := j.db.ExecContext(t.Context(), `PRAGMA user_version=12`); err != nil {
			t.Fatal(err)
		}
		if err := j.Close(); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, DBName)
		before := fileSHA256(t, p)
		if newer, err := Open(t.Context(), dir); !errors.Is(err, ErrSchemaTooNew) {
			if newer != nil {
				_ = newer.Close()
			}
			t.Fatalf("future schema: %v", err)
		}
		if fileSHA256(t, p) != before {
			t.Fatal("future schema bytes changed")
		}
		if _, err := os.Stat(p + ".bak-v12"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("future backup created: %v", err)
		}
	})
}

// SQL fixtures intentionally isolate structural relations. Task30 owns the
// complete canonical codec, retention consent, expiry and authenticated APIs.
type serviceSQLFixture struct{ prep, op, run, attempt, digest string }

func serviceSQLSeed(t *testing.T, j *Journal, suffix, stage string) serviceSQLFixture {
	t.Helper()
	f := serviceSQLFixture{"prep_" + suffix, "op_" + suffix, "run_" + suffix, "att_" + suffix, strings.Repeat("a", 64)}
	seedRun(t, j, f.run)
	attachmentTx(t, j, func(tx *sql.Tx) {
		attachmentSQL(t, tx, `INSERT INTO prepared_runs VALUES (?, 'peer',1,?,1,'{}','[]','[]',1,0,'2027-01-01T00:00:00Z')`, f.prep, f.digest)
		attachmentSQL(t, tx, `INSERT INTO operations VALUES (?, 'run.start',?,'params','claimed',0,NULL,'now',NULL)`, f.op, f.run)
		if stage == "prepared_runs" {
			return
		}
		binding := fmt.Sprintf(`{"SchemaVersion":1,"PreparationID":%q,"RootID":"root","PeerID":"peer","Generation":1,"ContextEpoch":"epoch","PolicyRevision":"policy","ExpiresAt":"2027-01-01T00:00:00Z","RunID":%q,"AttemptID":%q,"Decision":{"Task":{"SHA256":%q,"ArtifactRef":"synthetic"}}}`, f.prep, f.run, f.attempt, f.digest)
		approval := fmt.Sprintf(`{"PreparationID":%q,"BindingSHA256":%q,"RootID":"root","PeerID":"peer","Generation":1,"ContextEpoch":"epoch","PolicyRevision":"policy","ExpiresAt":"2027-01-01T00:00:00Z","Decision":"approve"}`, f.prep, f.digest)
		attachmentSQL(t, tx, `INSERT INTO service_admission_bindings VALUES (?,1,?,?,'root','peer',1,'epoch','policy',?,'synthetic','2027-01-01T00:00:00Z','{"Revision":"policy","Content":{"RootID":"root"}}',?,?,?)`, f.prep, binding, f.digest, f.digest, f.digest, approval, f.digest)
		if stage == "service_admission_bindings" {
			return
		}
		attachmentSQL(t, tx, `INSERT INTO service_admission_consumptions VALUES (?,?,?,?,1,?)`, f.prep, f.op, f.run, f.attempt, f.digest)
		if stage == "service_admission_consumptions" {
			return
		}
		attachmentSQL(t, tx, `INSERT INTO service_pending_dispatch VALUES (?,?,?,?,?,1,'pending',1)`, f.op, f.prep, f.run, f.attempt, f.digest)
		if stage == "service_pending_dispatch" {
			return
		}
		if err := InsertAttempt(t.Context(), tx, AttemptRow{AttemptID: f.attempt, RunID: f.run, TaskID: "task", AttemptNumber: 1, State: "launch_intent_recorded", LaunchTokenSHA256: f.digest, WorkspacePath: "synthetic"}); err != nil {
			t.Fatal(err)
		}
		attachmentSQL(t, tx, `INSERT INTO journal (event_id,schema_version,run_id,attempt_id,producer_id,producer_sequence,run_sequence,generation,observed_at,type,payload) VALUES (?,1,?,NULL,?,1,1,1,'now','workspace.snapshot_created','{}')`, "evt_snapshot_"+suffix, f.run, "producer_"+suffix)
		attachmentSQL(t, tx, `INSERT INTO dispatch_outbox VALUES (?,?, 'peer',1,?,?,?, ?,?,'pending')`, f.op, f.prep, f.digest, f.digest, f.run, f.attempt, f.digest)
		attachmentSQL(t, tx, `INSERT INTO service_dispatch_claims VALUES (?,1,'claimed',1,?,?,?,'ladder','2027-01-01T00:00:00Z',NULL,NULL,NULL)`, f.op, "evt_snapshot_"+suffix, f.digest, f.digest)
	})
	return f
}

type serviceSQLQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func serviceTableRows(t *testing.T, j *Journal, table string) string {
	return serviceSQLRows(t, j.db, table)
}

func serviceSQLRows(t *testing.T, q serviceSQLQueryer, table string) string {
	t.Helper()
	var result strings.Builder
	rows, err := q.QueryContext(t.Context(), "SELECT rowid,* FROM "+table+" ORDER BY rowid")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		values := make([]any, len(columns))
		targets := make([]any, len(columns))
		for i := range values {
			targets[i] = &values[i]
		}
		if err := rows.Scan(targets...); err != nil {
			t.Fatal(err)
		}
		result.WriteString(fmt.Sprint(values) + "\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result.String()
}

//nolint:misspell // Exact SQL column names retain their accepted contract spelling.
func TestServiceBindingImmutability(t *testing.T) { // I02/I12/I20/I21/I23 (v2 §§5/6/8)
	t.Parallel()
	testServiceAuthorityEvolution(t)
	tables := []struct {
		name string
		keys []string
	}{
		{"service_admission_bindings", []string{"preparation_id"}},
		{"service_admission_consumptions", []string{"preparation_id", "operation_id", "run_id", "attempt_id"}},
		{"service_pending_dispatch", []string{"operation_id", "preparation_id", "run_id", "attempt_id"}},
		{"service_dispatch_claims", []string{"operation_id"}},
	}
	for _, table := range tables {
		t.Run(table.name, func(t *testing.T) {
			t.Parallel()
			for _, key := range append(append([]string{}, table.keys...), "rowid") {
				for _, kind := range []string{"insert", "update"} {
					t.Run(kind+" collision "+key, func(t *testing.T) {
						t.Parallel()
						j, _ := openTemp(t)
						serviceSQLSeed(t, j, "a", table.name)
						serviceSQLSeed(t, j, "b", table.name)
						before := serviceTableRows(t, j, table.name)
						var query string
						// Fixed test metadata supplies all identifiers; values never supply SQL.
						if kind == "insert" {
							columns, err := j.db.QueryContext(t.Context(), "SELECT name FROM pragma_table_info(?)", table.name)
							if err != nil {
								t.Fatal(err)
							}
							var names, expr []string
							for columns.Next() {
								var name string
								if err := columns.Scan(&name); err != nil {
									_ = columns.Close()
									t.Fatal(err)
								}
								names = append(names, name)
								if name == key {
									expr = append(expr, "(SELECT "+key+" FROM "+table.name+" WHERE rowid=1)")
								} else {
									expr = append(expr, name)
								}
							}
							if err := columns.Err(); err != nil {
								_ = columns.Close()
								t.Fatal(err)
							}
							_ = columns.Close()
							if key == "rowid" {
								names = append([]string{"rowid"}, names...)
								expr = append([]string{"1"}, expr...)
							}
							query = "INSERT OR REPLACE INTO " + table.name + " (" + strings.Join(names, ",") + ") SELECT " + strings.Join(expr, ",") + " FROM " + table.name + " WHERE rowid=2"
						} else {
							query = "UPDATE OR REPLACE " + table.name + " SET " + key + "=(SELECT " + key + " FROM " + table.name + " WHERE rowid=1) WHERE rowid=2"
						}
						err := j.Transact(t.Context(), func(tx *sql.Tx) error { _, err := tx.ExecContext(t.Context(), query); return err })
						if err == nil {
							t.Fatal("hostile replacement accepted")
						}
						if got := serviceTableRows(t, j, table.name); got != before {
							t.Fatal("replacement changed an immutable victim")
						}
						// Both victims have no child row. Removing the guards lets the same
						// compile-valid statement change a real victim instead of an FK refusal.
						rollback := errors.New("control rollback")
						err = j.Transact(t.Context(), func(tx *sql.Tx) error {
							for _, guard := range []string{"_no_replace", "_immutable", "_identity"} {
								if _, err := tx.ExecContext(t.Context(), "DROP TRIGGER "+table.name+guard); err != nil {
									return err
								}
							}
							if _, err := tx.ExecContext(t.Context(), query); err != nil {
								return err
							}
							if after := serviceSQLRows(t, tx, table.name); after == before {
								t.Error("removed guards left both physical victims unchanged")
							}
							return rollback
						})
						if !errors.Is(err, rollback) {
							t.Fatalf("removed-guard control did not reach victim: %v", err)
						}
					})
				}
			}
		})
	}
	t.Run("consumed operation parents are stable", func(t *testing.T) {
		t.Parallel()
		for _, q := range []string{
			`UPDATE operations SET method='foreign' WHERE operation_id='op_a'`,
			`INSERT OR REPLACE INTO operations SELECT operation_id,'foreign',object,digest,state,revision,result,claimed_at,finished_at FROM operations WHERE operation_id='op_a'`,
			`UPDATE OR REPLACE operations SET rowid=1,operation_id='op_a',method='foreign' WHERE operation_id='unbound'`,
			`UPDATE operations SET rowid=99 WHERE operation_id='op_a'`,
		} {
			t.Run(q, func(t *testing.T) {
				t.Parallel()
				j, _ := openTemp(t)
				serviceSQLSeed(t, j, "a", "service_admission_consumptions")
				attachmentTx(t, j, func(tx *sql.Tx) {
					attachmentSQL(t, tx, `INSERT INTO operations VALUES ('unbound','old','object','digest','claimed',0,NULL,'now',NULL)`)
				})
				before := serviceTableRows(t, j, "operations")
				if _, err := j.db.ExecContext(t.Context(), q); err == nil {
					t.Fatal("consumed operation parent changed")
				}
				if serviceTableRows(t, j, "operations") != before {
					t.Fatal("refused parent mutation changed operation rows")
				}
			})
		}
	})
	t.Run("retained preparation replacement guards", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct{ name, sql string }{
			{"insert replaces retained primary key", `INSERT OR REPLACE INTO prepared_runs SELECT preparation_id,'foreign',generation,binding_sha256,revision,spec,challenges,confirmations,ready,consumed,expires_at FROM prepared_runs WHERE preparation_id='prep_a'`},
			{"update replaces retained physical victim", `UPDATE OR REPLACE prepared_runs SET rowid=1,preparation_id='prep_a',peer_id='foreign' WHERE preparation_id='unbound'`},
			{"bound row moves physical identity", `UPDATE prepared_runs SET rowid=99 WHERE preparation_id='prep_a'`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				j, _ := openTemp(t)
				serviceSQLSeed(t, j, "a", "service_admission_bindings")
				attachmentTx(t, j, func(tx *sql.Tx) {
					attachmentSQL(t, tx, `INSERT INTO prepared_runs VALUES ('unbound','peer',1,'old',1,'{}','[]','[]',1,0,'future')`)
				})
				before := serviceTableRows(t, j, "prepared_runs")
				if _, err := j.db.ExecContext(t.Context(), tc.sql); err == nil {
					t.Fatal("retained preparation victim was replaced")
				}
				if serviceTableRows(t, j, "prepared_runs") != before {
					t.Fatal("refused preparation replacement changed rows")
				}
			})
		}
	})
	t.Run("every retained binding value is immutable", func(t *testing.T) {
		t.Parallel()
		j, _ := openTemp(t)
		serviceSQLSeed(t, j, "a", "service_admission_bindings")
		for _, column := range []string{"canonical_binding", "binding_sha256", "root_id", "peer_id", "generation", "context_epoch", "policy_revision", "task_artifact_sha256", "task_artifact_ref", "expires_at", "retention_policy", "retention_policy_sha256", "retention_approval", "retention_approval_sha256"} {
			var changed any = "changed"
			switch {
			case column == "generation":
				changed = int64(2)
			case strings.HasSuffix(column, "sha256"):
				changed = strings.Repeat("b", 64)
			case column == "canonical_binding" || column == "retention_policy" || column == "retention_approval":
				changed = "{}"
			}
			//nolint:gosec // G202: column comes only from the fixed test-owned column inventory above.
			if _, err := j.db.ExecContext(t.Context(), "UPDATE service_admission_bindings SET "+column+"=? WHERE preparation_id='prep_a'", changed); err == nil {
				t.Errorf("binding update accepted for %s", column)
			}
		}
		if _, err := j.db.ExecContext(t.Context(), `UPDATE prepared_runs SET peer_id='foreign' WHERE preparation_id='prep_a'`); err == nil {
			t.Fatal("retained preparation identity changed")
		}
	})
	t.Run("legacy unbound preparation cannot consume", func(t *testing.T) {
		t.Parallel()
		j, _ := openTemp(t)
		seedRun(t, j, "legacy")
		attachmentTx(t, j, func(tx *sql.Tx) {
			attachmentSQL(t, tx, `INSERT INTO prepared_runs VALUES ('legacy','peer',1,'old',1,'{}','[]','[]',1,0,'future')`)
			attachmentSQL(t, tx, `INSERT INTO operations VALUES ('legacy','run.start','legacy','digest','claimed',0,NULL,'now',NULL)`)
		})
		if _, err := j.db.ExecContext(t.Context(), `INSERT INTO service_admission_consumptions VALUES ('legacy','legacy','legacy','planned',1,?)`, strings.Repeat("a", 64)); err == nil {
			t.Fatal("old ready bit conferred binding authority")
		}
	})
	t.Run("pending admission has no attempt or launch clock", func(t *testing.T) {
		t.Parallel()
		j, _ := openTemp(t)
		serviceSQLSeed(t, j, "a", "service_pending_dispatch")
		var n int
		if err := j.db.QueryRowContext(t.Context(), `SELECT count(*) FROM attempts`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatal("planned attempt became an attempts row")
		}
		if _, err := j.db.ExecContext(t.Context(), `UPDATE service_pending_dispatch SET state='armed',revision=revision+1`); err == nil {
			t.Fatal("pending intent armed without a dispatch claim")
		}
	})
	t.Run("claim transitions and revisions are closed", func(t *testing.T) {
		t.Parallel()
		j, _ := openTemp(t)
		serviceSQLSeed(t, j, "a", "service_dispatch_claims")
		before := serviceTableRows(t, j, "service_dispatch_claims")
		for _, q := range []string{`UPDATE service_dispatch_claims SET state='identified',revision=2`, `UPDATE service_dispatch_claims SET state='spawn_intent'`, `UPDATE service_dispatch_claims SET claim_generation=2`, `UPDATE service_dispatch_claims SET launch_token_sha256='changed'`, `DELETE FROM service_dispatch_claims`} {
			if _, err := j.db.ExecContext(t.Context(), q); err == nil {
				t.Fatalf("invalid claim change accepted: %s", q)
			}
		}
		if serviceTableRows(t, j, "service_dispatch_claims") != before {
			t.Fatal("refused claim changed")
		}
		attachmentTx(t, j, func(tx *sql.Tx) {
			attachmentSQL(t, tx, `UPDATE service_pending_dispatch SET state='armed',revision=2`)
			attachmentSQL(t, tx, `UPDATE service_dispatch_claims SET state='spawn_intent',revision=2`)
			attachmentSQL(t, tx, `UPDATE service_dispatch_claims SET state='unconfirmed',revision=3`)
			attachmentSQL(t, tx, `UPDATE service_dispatch_claims SET state='settled',revision=4`)
		})
		if _, err := j.db.ExecContext(t.Context(), `UPDATE service_dispatch_claims SET state='claimed',revision=5`); err == nil {
			t.Fatal("settled claim reopened")
		}
	})
}

//nolint:misspell // Exact SQL column names retain their accepted contract spelling.
func testServiceAuthorityEvolution(t *testing.T) {
	t.Helper()
	t.Run("structural retention revisions are exact", func(t *testing.T) {
		t.Parallel()
		for _, path := range []string{"policy", "approval"} {
			t.Run(path, func(t *testing.T) {
				t.Parallel()
				j, _ := openTemp(t)
				serviceSQLSeed(t, j, "a", "service_admission_bindings")
				serviceSQLSeed(t, j, "b", "prepared_runs")
				policy, approval := "policy", "policy"
				if path == "policy" {
					policy = "foreign"
				} else {
					approval = "foreign"
				}
				_, err := j.db.ExecContext(t.Context(), `INSERT INTO service_admission_bindings
    SELECT 'prep_b',codec_version,json_set(canonical_binding,'$.PreparationID','prep_b','$.RunID','run_b','$.AttemptID','att_b'),binding_sha256,root_id,peer_id,generation,context_epoch,policy_revision,task_artifact_sha256,task_artifact_ref,expires_at,
     json_set(retention_policy,'$.Revision',?),retention_policy_sha256,json_set(retention_approval,'$.PreparationID','prep_b','$.PolicyRevision',?),retention_approval_sha256
    FROM service_admission_bindings WHERE preparation_id='prep_a'`, policy, approval)
				if err == nil {
					t.Fatal("mismatched structural retention revision accepted")
				}
			})
		}
	})
	t.Run("identified worker evidence and exact revisions", func(t *testing.T) {
		t.Parallel()
		j, _ := openTemp(t)
		f := serviceSQLSeed(t, j, "a", "service_dispatch_claims")
		var before uint64
		attachmentTx(t, j, func(tx *sql.Tx) { before = attachmentRevision(t, tx, f.run) })
		if _, err := j.db.ExecContext(t.Context(), `UPDATE service_dispatch_claims SET state='spawn_intent',revision=2`); err == nil {
			t.Fatal("unarmed pending intent permitted spawn")
		}
		attachmentTx(t, j, func(tx *sql.Tx) {
			attachmentSQL(t, tx, `UPDATE service_pending_dispatch SET state='armed',revision=2`)
			attachmentSQL(t, tx, `UPDATE service_dispatch_claims SET state='spawn_intent',revision=2`)
			attachmentSQL(t, tx, `UPDATE attempts SET worker_pid=123,worker_start_time='start' WHERE attempt_id=?`, f.attempt)
			attachmentSQL(t, tx, `INSERT INTO journal (event_id,schema_version,run_id,attempt_id,producer_id,producer_sequence,run_sequence,generation,observed_at,type,payload) VALUES ('evt_launched',1,?,?,'producer_a',2,2,1,'now','attempt.launched','{}')`, f.run, f.attempt)
			attachmentSQL(t, tx, `UPDATE service_dispatch_claims SET state='identified',revision=3,worker_pid=123,worker_start_time='start',identifying_event_id='evt_launched'`)
			if got := attachmentRevision(t, tx, f.run); got <= before {
				t.Fatalf("authority did not advance actual revision %d -> %d", before, got)
			}
		})
		var stable uint64
		attachmentTx(t, j, func(tx *sql.Tx) {
			stable = attachmentRevision(t, tx, f.run)
			attachmentSQL(t, tx, `UPDATE service_dispatch_claims SET state=state,revision=revision`)
			if got := attachmentRevision(t, tx, f.run); got != stable {
				t.Fatal("duplicate claim write advanced revision")
			}
		})
		for _, q := range []string{`UPDATE service_dispatch_claims SET state='unconfirmed',revision=4,worker_pid=456`, `UPDATE service_dispatch_claims SET state='unconfirmed',revision=4,identifying_event_id='evt_snapshot_a'`} {
			if _, err := j.db.ExecContext(t.Context(), q); err == nil {
				t.Fatal("identified worker evidence was rebound")
			}
		}
		rows := serviceTableRows(t, j, "service_dispatch_claims")
		synthetic := errors.New("injected transaction failure")
		err := j.Transact(t.Context(), func(tx *sql.Tx) error {
			attachmentSQL(t, tx, `UPDATE service_dispatch_claims SET state='unconfirmed',revision=4`)
			return synthetic
		})
		if !errors.Is(err, synthetic) || serviceTableRows(t, j, "service_dispatch_claims") != rows {
			t.Fatal("failed transaction retained claim mutation")
		}
		attachmentTx(t, j, func(tx *sql.Tx) {
			if got := attachmentRevision(t, tx, f.run); got != stable {
				t.Fatal("rollback retained authority revision advance")
			}
		})
	})
}
