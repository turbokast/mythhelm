package migrate_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/migrate"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// fixtureTime is the fixed timestamp every seeded v1 row carries, so
// golden comparisons never depend on the wall clock.
var fixtureTime = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func openTestJournal(t *testing.T) *journal.Journal {
	t.Helper()
	j, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := j.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return j
}

type seedRun struct {
	runID    string
	taskID   string // legacy v1 task id shared by the run's attempts
	taskIDs  []string
	state    string
	reason   string
	posture  string
	attempts int
}

// seedV1Run inserts a runs row, its attempts rows and two v1 journal events
// through the public journal API, and returns the attempt IDs in
// attempt-number order.
func seedV1Run(t *testing.T, j *journal.Journal, s seedRun) []string {
	t.Helper()
	ctx := t.Context()
	if s.posture == "" {
		s.posture = "subscription-declared"
	}
	run := journal.RunRow{
		RunID:            s.runID,
		State:            s.state,
		Reason:           s.reason,
		AdapterID:        "claude-code",
		SourceRepo:       "example.com/demo",
		TaskSHA256:       strings.Repeat("a", 64),
		BillingPosture:   s.posture,
		ExecutionProfile: "default",
		CreatedAt:        fixtureTime,
		UpdatedAt:        fixtureTime,
	}
	var attemptIDs []string
	err := j.Transact(ctx, func(tx *sql.Tx) error {
		if err := journal.InsertRun(ctx, tx, run); err != nil {
			return err
		}
		for i := 1; i <= s.attempts; i++ {
			id := fmt.Sprintf("%s-att-%d", s.runID, i)
			attemptIDs = append(attemptIDs, id)
			taskID := s.taskID
			if i <= len(s.taskIDs) && s.taskIDs[i-1] != "" {
				taskID = s.taskIDs[i-1]
			}
			if err := journal.InsertAttempt(ctx, tx, journal.AttemptRow{
				AttemptID:         id,
				RunID:             s.runID,
				TaskID:            taskID,
				AttemptNumber:     int64(i),
				State:             "launch_intent_recorded",
				LaunchTokenSHA256: strings.Repeat("b", 64),
				WorkspacePath:     "/tmp/ws",
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seeding run %s: %v", s.runID, err)
	}
	for i, typ := range []string{"run.created", "run.state_changed"} {
		ev := journal.Event{
			SchemaVersion:    journal.EnvelopeVersion,
			EventID:          fmt.Sprintf("evt-%s-%d", s.runID, i),
			RunID:            s.runID,
			ProducerID:       "sup-" + s.runID,
			ProducerSequence: int64(i + 1),
			Generation:       1,
			ObservedAt:       fixtureTime,
			Type:             typ,
			Payload:          json.RawMessage(`{"state":"executing","reason":null}`),
		}
		if err := j.Append(ctx, ev, nil); err != nil {
			t.Fatalf("seeding event %s: %v", ev.EventID, err)
		}
	}
	return attemptIDs
}

// importCommitted runs ImportRun inside one caller transaction and commits it.
func importCommitted(t *testing.T, j *journal.Journal, runID string) (v2contract.TaskRevision, error) {
	t.Helper()
	var rev v2contract.TaskRevision
	err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		var err error
		rev, err = migrate.ImportRun(t.Context(), tx, runID)
		return err
	})
	return rev, err
}

func requireControlError(t *testing.T, err error, code v2contract.Code) *v2contract.ControlError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s error, got nil", code)
	}
	var cerr *v2contract.ControlError
	if !errors.As(err, &cerr) {
		t.Fatalf("error is %T, want *v2contract.ControlError: %v", err, err)
	}
	if cerr.Code != code {
		t.Fatalf("code = %s, want %s (%v)", cerr.Code, code, err)
	}
	if err := cerr.Validate(); err != nil {
		t.Fatalf("ControlError.Validate: %v", err)
	}
	return cerr
}

func countRows(t *testing.T, j *journal.Journal, table string) int {
	t.Helper()
	var n int
	err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM `+table).Scan(&n)
	})
	if err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return n
}

// dumpJournal returns every journal row as one string, in seq order.
func dumpJournal(t *testing.T, j *journal.Journal) []string {
	t.Helper()
	var out []string
	err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(t.Context(), `SELECT seq, event_id, schema_version, run_id,
			COALESCE(task_id, ''), COALESCE(attempt_id, ''), producer_id, producer_sequence,
			run_sequence, generation, COALESCE(caused_by, ''), observed_at, type, payload
			FROM journal ORDER BY seq`)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var seq, schema, prodSeq, runSeq, gen int64
			var eventID, runID, taskID, attemptID, producer, causedBy, observed, typ, payload string
			if err := rows.Scan(&seq, &eventID, &schema, &runID, &taskID, &attemptID,
				&producer, &prodSeq, &runSeq, &gen, &causedBy, &observed, &typ, &payload); err != nil {
				return err
			}
			out = append(out, fmt.Sprintf("%d|%s|%d|%s|%s|%s|%s|%d|%d|%d|%s|%s|%s|%s",
				seq, eventID, schema, runID, taskID, attemptID, producer, prodSeq, runSeq,
				gen, causedBy, observed, typ, payload))
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("dumping journal: %v", err)
	}
	return out
}

func dumpRuns(t *testing.T, j *journal.Journal) []string {
	t.Helper()
	var out []string
	err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(t.Context(), `SELECT run_id, state, COALESCE(reason, ''),
			adapter_id, source_repo, COALESCE(source_branch, ''), COALESCE(base_rev, ''),
			task_sha256, billing_posture, execution_profile, created_at, updated_at
			FROM runs ORDER BY run_id`)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var cols [12]string
			ptrs := make([]any, len(cols))
			for i := range cols {
				ptrs[i] = &cols[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				return err
			}
			out = append(out, strings.Join(cols[:], "|"))
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("dumping runs: %v", err)
	}
	return out
}

func dumpAttempts(t *testing.T, j *journal.Journal) []string {
	t.Helper()
	var out []string
	err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(t.Context(), `SELECT attempt_id, run_id, task_id, attempt_number,
			state, COALESCE(reason, ''), launch_token_sha256, COALESCE(workspace_path, ''), spool_offset
			FROM attempts ORDER BY attempt_id`)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var attemptID, runID, taskID, state, reason, token, ws string
			var number, offset int64
			if err := rows.Scan(&attemptID, &runID, &taskID, &number, &state,
				&reason, &token, &ws, &offset); err != nil {
				return err
			}
			out = append(out, fmt.Sprintf("%s|%s|%s|%d|%s|%s|%s|%s|%d",
				attemptID, runID, taskID, number, state, reason, token, ws, offset))
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("dumping attempts: %v", err)
	}
	return out
}

// readTaskRevision returns the stored record and digest for task_id/revision.
func readTaskRevision(t *testing.T, j *journal.Journal, taskID string, revision int) (record, digest, runID string) {
	t.Helper()
	err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT record, digest, run_id FROM task_revisions
			WHERE task_id = ? AND revision = ?`, taskID, revision).Scan(&record, &digest, &runID)
	})
	if err != nil {
		t.Fatalf("reading task_revisions %s/%d: %v", taskID, revision, err)
	}
	return record, digest, runID
}

// readTasksRow returns the tasks row for task_id.
func readTasksRow(t *testing.T, j *journal.Journal, taskID string) (head int, runID string) {
	t.Helper()
	err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT head_revision, run_id FROM tasks
			WHERE task_id = ?`, taskID).Scan(&head, &runID)
	})
	if err != nil {
		t.Fatalf("reading tasks %s: %v", taskID, err)
	}
	return head, runID
}

// readEnvelope returns the journal row for event_id.
func readEnvelope(t *testing.T, j *journal.Journal, eventID string) (schema int, runID, taskID, attemptID, producer, typ, payload string, prodSeq, runSeq, gen int64) {
	t.Helper()
	err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT schema_version, run_id,
			COALESCE(task_id, ''), COALESCE(attempt_id, ''), producer_id, type, payload,
			producer_sequence, run_sequence, generation
			FROM journal WHERE event_id = ?`, eventID).Scan(
			&schema, &runID, &taskID, &attemptID, &producer, &typ, &payload, &prodSeq, &runSeq, &gen)
	})
	if err != nil {
		t.Fatalf("reading envelope %s: %v", eventID, err)
	}
	return schema, runID, taskID, attemptID, producer, typ, payload, prodSeq, runSeq, gen
}

func TestImportPreservesIDsPostureEvidence(t *testing.T) {
	t.Parallel()
	postures := []string{"subscription-declared", "subscription-only", "local-scripted"}
	for _, posture := range postures {
		t.Run(posture, func(t *testing.T) {
			t.Parallel()
			j := openTestJournal(t)
			const runID, taskID = "run-golden", "task-golden"
			attempts := seedV1Run(t, j, seedRun{runID: runID, taskID: taskID,
				state: "executing", posture: posture, attempts: 2})
			beforeJournal, beforeRuns, beforeAttempts := dumpJournal(t, j), dumpRuns(t, j), dumpAttempts(t, j)

			rev, err := importCommitted(t, j, runID)
			if err != nil {
				t.Fatalf("ImportRun: %v", err)
			}
			if rev.Revision != 1 {
				t.Errorf("Revision = %d, want 1", rev.Revision)
			}
			if rev.TaskID != taskID {
				t.Errorf("TaskID = %q, want legacy %q", rev.TaskID, taskID)
			}
			if err := rev.Validate(); err != nil {
				t.Errorf("returned TaskRevision.Validate: %v", err)
			}

			// Golden record: posture never leaks into the v2 record, so the
			// bytes are identical for every posture word. I15 (v2 §2):
			// declared posture is never promoted.
			wantRecord := `{"schema_version":2,"task_id":"task-golden","revision":1,` +
				`"deliverable":"","acceptance_contract_digest":"sha256:` + strings.Repeat("a", 64) + `",` +
				`"dependencies":null,"write_scope":null,"risk":"","resource_claims":null,` +
				`"integration_destination":"","lifecycle":"running"}`
			record, digest, storedRun := readTaskRevision(t, j, taskID, 1)
			if record != wantRecord {
				t.Errorf("stored record:\n got %s\nwant %s", record, wantRecord)
			}
			if strings.Contains(record, "verified") {
				t.Errorf("stored record claims verification: %s", record)
			}
			if digest != v2contract.Digest(rev) {
				t.Errorf("stored digest %q != Digest(returned) %q", digest, v2contract.Digest(rev))
			}
			if storedRun != runID {
				t.Errorf("task_revisions run_id = %q, want %q", storedRun, runID)
			}
			if head, markerRun := readTasksRow(t, j, taskID); head != 1 || markerRun != runID {
				t.Errorf("tasks row = (%d, %q), want (1, %q)", head, markerRun, runID)
			}

			// The envelope preserves the legacy IDs and copies the posture
			// word verbatim. I15 (v2 §2): never verified.
			eventID := "migration-imported-" + runID
			schema, envRun, envTask, envAttempt, producer, typ, payload, prodSeq, runSeq, gen :=
				readEnvelope(t, j, eventID)
			if schema != 2 || envRun != runID || envTask != taskID || envAttempt != attempts[1] {
				t.Errorf("envelope ids = (%d, %q, %q, %q), want (2, %q, %q, %q)",
					schema, envRun, envTask, envAttempt, runID, taskID, attempts[1])
			}
			if producer != "migration" || typ != "migration.imported" || gen != 0 {
				t.Errorf("envelope = (%q, %q, gen %d), want (migration, migration.imported, gen 0)",
					producer, typ, gen)
			}
			if prodSeq != 1 || runSeq != 3 {
				t.Errorf("sequences = (producer %d, run %d), want (1, 3)", prodSeq, runSeq)
			}
			wantPayload := fmt.Sprintf(`{"run_id":"run-golden","task_id":"task-golden",`+
				`"revision":1,"posture":%q,"legacy_state":"executing",`+
				`"attempt_ids":["run-golden-att-1","run-golden-att-2"]}`, posture)
			if payload != wantPayload {
				t.Errorf("payload:\n got %s\nwant %s", payload, wantPayload)
			}
			var decoded map[string]any
			if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
				t.Fatalf("payload is not JSON: %v", err)
			}
			if decoded["posture"] != posture {
				t.Errorf("payload posture = %v, want %q verbatim", decoded["posture"], posture)
			}
			if _, ok := decoded["verified"]; ok {
				t.Errorf("payload claims verification: %s", payload)
			}

			// The v1 evidence is untouched: every pre-existing journal row is
			// byte-identical and the projections are unchanged. AC-7.5.
			afterJournal := dumpJournal(t, j)
			if len(afterJournal) != len(beforeJournal)+1 {
				t.Fatalf("journal rows = %d, want %d + 1 envelope", len(afterJournal), len(beforeJournal))
			}
			for i, row := range beforeJournal {
				if afterJournal[i] != row {
					t.Errorf("journal row %d changed:\n got %s\nwant %s", i, afterJournal[i], row)
				}
			}
			if got := dumpRuns(t, j); strings.Join(got, "\n") != strings.Join(beforeRuns, "\n") {
				t.Errorf("runs projection changed:\n got %v\nwant %v", got, beforeRuns)
			}
			if got := dumpAttempts(t, j); strings.Join(got, "\n") != strings.Join(beforeAttempts, "\n") {
				t.Errorf("attempts projection changed:\n got %v\nwant %v", got, beforeAttempts)
			}
		})
	}
}

func TestImportReadyForReviewStaysCandidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		reason string
	}{
		{name: "unverified reason stays candidate", reason: "unverified"},
		// Even a fully verified v1 review is only a v1 observation: v2
		// acceptance needs v2-bound evidence. I07 (v2 §2).
		{name: "verified empty reason stays candidate", reason: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			j := openTestJournal(t)
			const runID, taskID = "run-rfr", "task-rfr"
			seedV1Run(t, j, seedRun{runID: runID, taskID: taskID,
				state: "ready_for_review", reason: tc.reason, attempts: 1})
			rev, err := importCommitted(t, j, runID)
			if err != nil {
				t.Fatalf("ImportRun: %v", err)
			}
			// I09 (v2 §2): unverified stays unverified. A variant
			// promoting to accepted fails here.
			if rev.State != v2contract.TaskCandidate {
				t.Errorf("State = %q, want %q", rev.State, v2contract.TaskCandidate)
			}
			if rev.State == v2contract.TaskAccepted {
				t.Errorf("import promoted ready_for_review to accepted")
			}
		})
	}
}

func TestImportTwiceConflicts(t *testing.T) {
	t.Parallel()
	j := openTestJournal(t)
	const runID, taskID = "run-twice", "task-twice"
	seedV1Run(t, j, seedRun{runID: runID, taskID: taskID, state: "executing", attempts: 1})
	first, err := importCommitted(t, j, runID)
	if err != nil {
		t.Fatalf("first ImportRun: %v", err)
	}
	_, err = importCommitted(t, j, runID)
	cerr := requireControlError(t, err, v2contract.CodeRevisionConflict)
	if !strings.Contains(cerr.Detail["migration/cause"], runID) {
		t.Errorf("conflict detail %q does not name run %s", cerr.Detail["migration/cause"], runID)
	}
	// Nothing new was written: one marker, one revision, one envelope.
	if n := countRows(t, j, "tasks"); n != 1 {
		t.Errorf("tasks rows = %d, want 1", n)
	}
	if n := countRows(t, j, "task_revisions"); n != 1 {
		t.Errorf("task_revisions rows = %d, want 1", n)
	}
	var envelopes int
	err = j.Transact(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM journal WHERE event_id = ?`,
			"migration-imported-"+runID).Scan(&envelopes)
	})
	if err != nil {
		t.Fatalf("counting envelopes: %v", err)
	}
	if envelopes != 1 {
		t.Errorf("envelopes = %d, want 1", envelopes)
	}
	// The stored revision still decodes to the first import's value.
	record, _, _ := readTaskRevision(t, j, taskID, 1)
	again, err := v2contract.Decode[v2contract.TaskRevision]([]byte(record))
	if err != nil {
		t.Fatalf("decoding stored record: %v", err)
	}
	if !reflect.DeepEqual(again, first) {
		t.Errorf("stored record != first import:\n got %+v\nwant %+v", again, first)
	}
}

func TestImportEventIDReuseConflicts(t *testing.T) {
	t.Parallel()
	j := openTestJournal(t)
	const runID, taskID = "run-reuse", "task-reuse"
	seedV1Run(t, j, seedRun{runID: runID, taskID: taskID, state: "executing", attempts: 1})
	eventID := "migration-imported-" + runID
	// A foreign envelope already owns the generated event_id, with
	// different contents: the import must refuse, not overwrite it.
	err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), `INSERT INTO journal
			(event_id, schema_version, run_id, producer_id, producer_sequence,
			run_sequence, generation, observed_at, type, payload)
			VALUES (?, 2, ?, 'foreign', 1, 99, 0, ?, 'migration.imported', ?)`,
			eventID, runID, fixtureTime.Format(time.RFC3339Nano),
			`{"run_id":"`+runID+`","posture":"forged"}`)
		return err
	})
	if err != nil {
		t.Fatalf("planting foreign envelope: %v", err)
	}
	before := dumpJournal(t, j)

	_, err = importCommitted(t, j, runID)
	cerr := requireControlError(t, err, v2contract.CodeRevisionConflict)
	if !strings.Contains(cerr.Detail["migration/cause"], "differs") {
		t.Errorf("conflict detail %q does not report the mismatch", cerr.Detail["migration/cause"])
	}
	// No v2 rows, no import marker and no envelope were committed.
	if n := countRows(t, j, "tasks"); n != 0 {
		t.Errorf("tasks rows = %d, want 0", n)
	}
	if n := countRows(t, j, "task_revisions"); n != 0 {
		t.Errorf("task_revisions rows = %d, want 0", n)
	}
	if got := dumpJournal(t, j); strings.Join(got, "\n") != strings.Join(before, "\n") {
		t.Errorf("journal changed:\n got %v\nwant %v", got, before)
	}
	var migrationProducers int
	err = j.Transact(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM producers WHERE producer_id = 'migration'`).Scan(&migrationProducers)
	})
	if err != nil {
		t.Fatalf("counting migration producers: %v", err)
	}
	if migrationProducers != 0 {
		t.Errorf("migration producer rows = %d, want 0", migrationProducers)
	}
}

func TestImportCorruptV1Refuses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		state  string
		reason string
		// mutate corrupts the seeded run after a valid insert.
		mutate func(t *testing.T, tx *sql.Tx, runID string)
		// skipSeed leaves the database without the run.
		skipSeed bool
	}{
		{name: "unknown state word", state: "bogus-state"},
		{name: "failed without reason", state: "failed"},
		{name: "ready_for_review with bogus reason", state: "ready_for_review", reason: "verified-by-nobody"},
		{
			name:  "unparsable created_at",
			state: "executing",
			mutate: func(t *testing.T, tx *sql.Tx, runID string) {
				t.Helper()
				if _, err := tx.ExecContext(t.Context(), `UPDATE runs SET created_at = 'not-a-time' WHERE run_id = ?`, runID); err != nil {
					t.Fatalf("corrupting created_at: %v", err)
				}
			},
		},
		{
			name:  "empty billing posture",
			state: "executing",
			mutate: func(t *testing.T, tx *sql.Tx, runID string) {
				t.Helper()
				if _, err := tx.ExecContext(t.Context(), `UPDATE runs SET billing_posture = '' WHERE run_id = ?`, runID); err != nil {
					t.Fatalf("clearing posture: %v", err)
				}
			},
		},
		{
			name:  "unknown attempt state",
			state: "executing",
			mutate: func(t *testing.T, tx *sql.Tx, runID string) {
				t.Helper()
				if _, err := tx.ExecContext(t.Context(), `UPDATE attempts SET state = 'bogus' WHERE run_id = ?`, runID); err != nil {
					t.Fatalf("corrupting attempt state: %v", err)
				}
			},
		},
		{name: "missing run", skipSeed: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			j := openTestJournal(t)
			const runID, taskID = "run-corrupt", "task-corrupt"
			if !tc.skipSeed {
				seedV1Run(t, j, seedRun{runID: runID, taskID: taskID,
					state: tc.state, reason: tc.reason, attempts: 1})
				if tc.mutate != nil {
					err := j.Transact(t.Context(), func(tx *sql.Tx) error {
						tc.mutate(t, tx, runID)
						return nil
					})
					if err != nil {
						t.Fatalf("corrupting fixture: %v", err)
					}
				}
			}
			before := dumpJournal(t, j)

			// AC-7.5: a v1 row that fails v1 decode is refused, never
			// reinterpreted.
			_, err := importCommitted(t, j, runID)
			_ = requireControlError(t, err, v2contract.CodeInvalidContract)
			if n := countRows(t, j, "tasks"); n != 0 {
				t.Errorf("tasks rows = %d, want 0", n)
			}
			if n := countRows(t, j, "task_revisions"); n != 0 {
				t.Errorf("task_revisions rows = %d, want 0", n)
			}
			if got := dumpJournal(t, j); strings.Join(got, "\n") != strings.Join(before, "\n") {
				t.Errorf("journal changed:\n got %v\nwant %v", got, before)
			}
		})
	}
}

func TestImportRollsBackAtomically(t *testing.T) {
	t.Parallel()
	j := openTestJournal(t)
	const runID, taskID = "run-rollback", "task-rollback"
	seedV1Run(t, j, seedRun{runID: runID, taskID: taskID, state: "executing", attempts: 1})
	// A foreign row already owns the migration producer's first sequence,
	// so the envelope insert fails after the v2 writes ran.
	err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), `INSERT INTO journal
			(event_id, schema_version, run_id, producer_id, producer_sequence,
			run_sequence, generation, observed_at, type, payload)
			VALUES ('foreign-collide', 1, ?, 'migration', 1, 99, 0, ?, 'run.created', '{}')`,
			runID, fixtureTime.Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		t.Fatalf("planting colliding row: %v", err)
	}
	before := dumpJournal(t, j)

	// ImportRun shares the caller's transaction, so the probe below reads
	// the partial writes before Transact rolls them back: the marker must
	// be visible inside the transaction and gone after it.
	ctx := t.Context()
	probeSawMarker := false
	txErr := j.Transact(ctx, func(tx *sql.Tx) error {
		_, err := migrate.ImportRun(ctx, tx, runID)
		if err == nil {
			t.Errorf("ImportRun with colliding producer sequence succeeded, want persistence_unavailable")
			return nil
		}
		var one int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM tasks WHERE task_id = ?`, taskID).Scan(&one); err == nil {
			probeSawMarker = true
		}
		return err
	})
	if !probeSawMarker {
		t.Errorf("tasks marker not visible after the envelope fault: the test would pass without partial writes")
	}
	_ = requireControlError(t, txErr, v2contract.CodePersistenceUnavailable)
	// The caller's rollback removed all three writes.
	if n := countRows(t, j, "tasks"); n != 0 {
		t.Errorf("tasks rows = %d, want 0", n)
	}
	if n := countRows(t, j, "task_revisions"); n != 0 {
		t.Errorf("task_revisions rows = %d, want 0", n)
	}
	if got := dumpJournal(t, j); strings.Join(got, "\n") != strings.Join(before, "\n") {
		t.Errorf("journal changed:\n got %v\nwant %v", got, before)
	}
	var migrationProducers int
	err = j.Transact(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM producers WHERE producer_id = 'migration'`).Scan(&migrationProducers)
	})
	if err != nil {
		t.Fatalf("counting migration producers: %v", err)
	}
	if migrationProducers != 0 {
		t.Errorf("migration producer rows = %d, want 0", migrationProducers)
	}
}

func TestImportReusesIdenticalEnvelope(t *testing.T) {
	t.Parallel()
	j := openTestJournal(t)
	const runID, taskID = "run-reuse-identical", "task-reuse-identical"
	seedV1Run(t, j, seedRun{runID: runID, taskID: taskID, state: "executing", attempts: 1})
	first, err := importCommitted(t, j, runID)
	if err != nil {
		t.Fatalf("first ImportRun: %v", err)
	}
	// Drop the v2 rows but keep the journaled envelope (the journal is
	// append-only): the next import finds its own identical envelope with
	// no marker and completes the import without duplicating it.
	err = j.Transact(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(t.Context(), `DELETE FROM task_revisions WHERE task_id = ?`, taskID); err != nil {
			return err
		}
		_, err = tx.ExecContext(t.Context(), `DELETE FROM tasks WHERE task_id = ?`, taskID)
		return err
	})
	if err != nil {
		t.Fatalf("dropping v2 rows: %v", err)
	}
	second, err := importCommitted(t, j, runID)
	if err != nil {
		t.Fatalf("second ImportRun: %v", err)
	}
	if !reflect.DeepEqual(second, first) {
		t.Errorf("second import != first:\n got %+v\nwant %+v", second, first)
	}
	var envelopes int
	err = j.Transact(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM journal WHERE event_id = ?`,
			"migration-imported-"+runID).Scan(&envelopes)
	})
	if err != nil {
		t.Fatalf("counting envelopes: %v", err)
	}
	if envelopes != 1 {
		t.Errorf("envelopes = %d, want 1 (no duplicate)", envelopes)
	}
	var lastSeq int64
	err = j.Transact(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT last_sequence FROM producers WHERE producer_id = 'migration'`).Scan(&lastSeq)
	})
	if err != nil {
		t.Fatalf("reading migration producer: %v", err)
	}
	if lastSeq != 1 {
		t.Errorf("migration last_sequence = %d, want 1 (no new append)", lastSeq)
	}
}

func TestImportRunWithoutAttempts(t *testing.T) {
	t.Parallel()
	j := openTestJournal(t)
	const runID = "run-no-attempts"
	seedV1Run(t, j, seedRun{runID: runID, state: "created", attempts: 0})
	rev, err := importCommitted(t, j, runID)
	if err != nil {
		t.Fatalf("ImportRun: %v", err)
	}
	// With no legacy task id to preserve, the task is the run.
	if rev.TaskID != runID {
		t.Errorf("TaskID = %q, want run id %q", rev.TaskID, runID)
	}
	if rev.State != v2contract.TaskPending {
		t.Errorf("State = %q, want %q", rev.State, v2contract.TaskPending)
	}
	_, _, _, envAttempt, _, _, _, _, _, _ := readEnvelope(t, j, "migration-imported-"+runID)
	if envAttempt != "" {
		t.Errorf("envelope attempt_id = %q, want empty", envAttempt)
	}
}

func TestImportUsesFirstAttemptTaskID(t *testing.T) {
	t.Parallel()
	j := openTestJournal(t)
	const runID = "run-two-tasks"
	seedV1Run(t, j, seedRun{runID: runID, taskIDs: []string{"task-first", "task-second"},
		state: "executing", attempts: 2})
	rev, err := importCommitted(t, j, runID)
	if err != nil {
		t.Fatalf("ImportRun: %v", err)
	}
	if rev.TaskID != "task-first" {
		t.Errorf("TaskID = %q, want earliest attempt's %q", rev.TaskID, "task-first")
	}
}

func TestImportMapsV1States(t *testing.T) {
	t.Parallel()
	// The total v1 → v2 state map. Import never yields accepted: v2
	// acceptance needs v2-bound evidence. I07 (v2 §2).
	cases := []struct {
		from   supervisor.RunState
		reason string
		want   v2contract.TaskState
	}{
		{from: supervisor.RunCreated, want: v2contract.TaskPending},
		{from: supervisor.RunAdmission, want: v2contract.TaskPending},
		{from: supervisor.RunExecuting, want: v2contract.TaskRunning},
		{from: supervisor.RunRecovering, want: v2contract.TaskRunning},
		{from: supervisor.RunStopping, want: v2contract.TaskRunning},
		{from: supervisor.RunVerifying, want: v2contract.TaskVerifying},
		{from: supervisor.RunReadyForReview, reason: "unverified", want: v2contract.TaskCandidate},
		{from: supervisor.RunApplying, want: v2contract.TaskCandidate},
		{from: supervisor.RunCompleted, want: v2contract.TaskCandidate},
		{from: supervisor.RunBlocked, reason: "needs-operator", want: v2contract.TaskBlocked},
		{from: supervisor.RunFailed, reason: "native-failed", want: v2contract.TaskFailed},
		{from: supervisor.RunCancelled, want: v2contract.TaskCancelled},
		{from: supervisor.RunInterrupted, reason: "worker-lost", want: v2contract.TaskBlocked},
	}
	for _, tc := range cases {
		t.Run(string(tc.from), func(t *testing.T) {
			t.Parallel()
			j := openTestJournal(t)
			runID := "run-state-" + string(tc.from)
			seedV1Run(t, j, seedRun{runID: runID, taskID: "task-" + string(tc.from),
				state: string(tc.from), reason: tc.reason, attempts: 1})
			rev, err := importCommitted(t, j, runID)
			if err != nil {
				t.Fatalf("ImportRun: %v", err)
			}
			if rev.State != tc.want {
				t.Errorf("State = %q, want %q", rev.State, tc.want)
			}
			if rev.State == v2contract.TaskAccepted {
				t.Errorf("import promoted %s to accepted", tc.from)
			}
		})
	}
}
