package migrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

// ImportOptions selects the runs to import; empty RunIDs means all v1 runs.
type ImportOptions struct {
	RunIDs []string
}

// ImportResult reports one imported run for the preview/receipt.
type ImportResult struct {
	RunID    string `json:"run_id"`
	TaskID   string `json:"task_id"`
	Revision int    `json:"revision"`
	Posture  string `json:"posture"` // copied declaration word, e.g. "subscription-declared"
}

const (
	// importProducerID is the journal producer every migration envelope
	// attributes to, so the import's appends sequence together.
	importProducerID = "migration"
	// importedEventType is the v2 envelope type recording one imported run.
	importedEventType = "migration.imported"
	// importEventPrefix prefixes the deterministic event_id of an import
	// envelope: importEventPrefix + the legacy run id.
	importEventPrefix = "migration-imported-"
	// importGeneration is the generation the migration producer appends at.
	importGeneration = 0
)

// v1RunStates is the frozen v1 run vocabulary (internal/supervisor/state.go:
// RunCreated … RunFailed) with each word's v2 TaskState. v1 is frozen
// forever (G16), so this copy must never gain words to match new
// development: an unknown word fails v1 decode and the import refuses it.
// Import never yields accepted: v2 acceptance needs v2-bound evidence
// (I07), and a v1 completion is only a v1 observation.
var v1RunStates = map[string]v2contract.TaskState{
	"created":          v2contract.TaskPending,
	"admission":        v2contract.TaskPending,
	"executing":        v2contract.TaskRunning,
	"recovering":       v2contract.TaskRunning,
	"stopping":         v2contract.TaskRunning,
	"verifying":        v2contract.TaskVerifying,
	"ready_for_review": v2contract.TaskCandidate,
	"applying":         v2contract.TaskCandidate,
	"completed":        v2contract.TaskCandidate,
	"blocked":          v2contract.TaskBlocked,
	"failed":           v2contract.TaskFailed,
	"cancelled":        v2contract.TaskCancelled,
	"interrupted":      v2contract.TaskBlocked,
}

// v1AttemptStates is the frozen v1 attempt vocabulary
// (internal/supervisor/state.go: AttemptLaunchIntentRecorded …
// AttemptQuarantined). Attempts keep no v2 lifecycle on import — the
// envelope preserves their IDs — so this only validates the decode.
var v1AttemptStates = map[string]bool{
	"launch_intent_recorded": true,
	"launching":              true,
	"running":                true,
	"succeeded_native":       true,
	"failed_native":          true,
	"stop_requested":         true,
	"stopped":                true,
	"interrupted":            true,
	"quarantined":            true,
}

// v1Run is the decoded v1 runs row ImportRun maps from.
type v1Run struct {
	state          string
	reason         string
	taskSHA256     string
	billingPosture string
}

// v1Attempt is the decoded identity of one v1 attempt row.
type v1Attempt struct {
	attemptID string
	taskID    string
}

// importedPayload is the migration.imported envelope payload: the preserved
// legacy IDs, the verbatim posture word and the v1 lifecycle facts the v2
// state mapping does not carry.
type importedPayload struct {
	RunID        string   `json:"run_id"`
	TaskID       string   `json:"task_id"`
	Revision     int      `json:"revision"`
	Posture      string   `json:"posture"`
	LegacyState  string   `json:"legacy_state"`
	LegacyReason string   `json:"legacy_reason,omitempty"`
	AttemptIDs   []string `json:"attempt_ids,omitempty"`
}

// ImportRun imports one legacy run as a one-task v2 run inside the caller's
// Mutate transaction: the v2 rows (tasks marker + task_revisions revision 1)
// and the migration.imported envelope commit or roll back with the caller —
// ImportRun itself never commits. The v2 task id is the run's earliest
// attempt's legacy task id, or the run id when the run has no attempts;
// the envelope's run and attempt ids are the legacy ids verbatim. Failure
// cases: revision_conflict (run already imported, or the stored
// migration.imported envelope for the generated event_id differs from the
// incoming envelope — the import-marker layer compares envelopes before
// treating a duplicate event_id as idempotent, and a mismatch aborts before
// anything commits); invalid_contract (v1 row fails v1 decode — never
// reinterpreted, AC-7.5); persistence_unavailable (ledger I/O).
func ImportRun(ctx context.Context, tx *sql.Tx, runID string) (v2contract.TaskRevision, error) {
	if tx == nil {
		return v2contract.TaskRevision{}, errors.New("migrate: ImportRun needs a transaction")
	}
	run, err := loadV1Run(ctx, tx, runID)
	if err != nil {
		return v2contract.TaskRevision{}, err
	}
	attempts, err := loadV1Attempts(ctx, tx, runID)
	if err != nil {
		return v2contract.TaskRevision{}, err
	}
	taskID, latestAttempt := runID, ""
	if len(attempts) > 0 {
		taskID, latestAttempt = attempts[0].taskID, attempts[len(attempts)-1].attemptID
	}
	rev := v2contract.TaskRevision{
		SchemaVersion: v2contract.SchemaVersion,
		TaskID:        taskID,
		Revision:      1,
		// v1 stored the admitted bundle as a digest only, so there is no
		// deliverable text, write scope, risk, claims or destination to
		// copy: the digest binds the record to the v1 contract (I09 —
		// missing stays missing, never zero-filled prose).
		AcceptanceContractDigest: "sha256:" + run.taskSHA256,
		State:                    v1RunStates[run.state],
	}
	// Every conflict check runs before the first write, so a conflict
	// writes nothing even inside the caller's transaction.
	if err := checkImportMarker(ctx, tx, taskID, runID); err != nil {
		return v2contract.TaskRevision{}, err
	}
	payload, err := json.Marshal(importedPayload{
		RunID:        runID,
		TaskID:       taskID,
		Revision:     rev.Revision,
		Posture:      run.billingPosture,
		LegacyState:  run.state,
		LegacyReason: run.reason,
		AttemptIDs:   attemptIDs(attempts),
	})
	if err != nil {
		return v2contract.TaskRevision{}, fmt.Errorf("migrate: encoding import payload for run %s: %w", runID, err)
	}
	reuse, err := checkImportEnvelope(ctx, tx, runID, taskID, latestAttempt, payload)
	if err != nil {
		return v2contract.TaskRevision{}, err
	}
	raw, err := json.Marshal(rev)
	if err != nil {
		return v2contract.TaskRevision{}, fmt.Errorf("migrate: encoding task revision for run %s: %w", runID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tasks (task_id, head_revision, run_id) VALUES (?, 1, ?)`,
		taskID, runID); err != nil {
		return v2contract.TaskRevision{}, importError(v2contract.CodePersistenceUnavailable, runID,
			"retry once the ledger is writable", "inserting tasks marker: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO task_revisions (task_id, revision, run_id, record, digest)
		VALUES (?, 1, ?, ?, ?)`, taskID, runID, string(raw), v2contract.Digest(rev)); err != nil {
		return v2contract.TaskRevision{}, importError(v2contract.CodePersistenceUnavailable, runID,
			"retry once the ledger is writable", "inserting task revision: %v", err)
	}
	if !reuse {
		if err := appendImported(ctx, tx, runID, taskID, latestAttempt, payload); err != nil {
			return v2contract.TaskRevision{}, err
		}
	}
	return rev, nil
}

func attemptIDs(attempts []v1Attempt) []string {
	if len(attempts) == 0 {
		return nil
	}
	ids := make([]string, len(attempts))
	for i, a := range attempts {
		ids[i] = a.attemptID
	}
	return ids
}

// loadV1Run reads the runs row inside tx and decodes it under the v1
// contract: every required field present, timestamps parseable, state and
// reason words v1 knows. Anything else is invalid_contract, never a guess.
func loadV1Run(ctx context.Context, tx *sql.Tx, runID string) (v1Run, error) {
	var run v1Run
	var state, adapterID, sourceRepo, execProfile string
	var reason sql.NullString
	var createdAt, updatedAt string
	err := tx.QueryRowContext(ctx, `SELECT state, reason, adapter_id, source_repo,
		task_sha256, billing_posture, execution_profile, created_at, updated_at
		FROM runs WHERE run_id = ?`, runID).Scan(
		&state, &reason, &adapterID, &sourceRepo,
		&run.taskSHA256, &run.billingPosture, &execProfile, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return v1Run{}, importError(v2contract.CodeInvalidContract, runID,
			"quarantine the run for operator repair; migration never reinterprets v1 data",
			"run %s has no v1 projection row", runID)
	}
	if err != nil {
		return v1Run{}, importError(v2contract.CodePersistenceUnavailable, runID,
			"retry once the ledger is writable", "reading v1 run: %v", err)
	}
	run.state, run.reason = state, reason.String
	for field, value := range map[string]string{
		"adapter_id": adapterID, "source_repo": sourceRepo, "task_sha256": run.taskSHA256,
		"billing_posture": run.billingPosture, "execution_profile": execProfile,
	} {
		if value == "" {
			return v1Run{}, invalidV1(runID, "runs.%s is empty", field)
		}
	}
	for field, value := range map[string]string{"created_at": createdAt, "updated_at": updatedAt} {
		if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
			return v1Run{}, invalidV1(runID, "runs.%s %q does not parse: %v", field, value, err)
		}
	}
	if _, ok := v1RunStates[state]; !ok {
		return v1Run{}, invalidV1(runID, "runs.state %q is not a v1 run state", state)
	}
	// Mirrors supervisor.checkReason: the reason vocabulary is part of v1
	// decode, so a row v1 itself could not have written refuses.
	switch state {
	case "blocked", "failed", "interrupted":
		if run.reason == "" {
			return v1Run{}, invalidV1(runID, "runs.state %q requires a reason", state)
		}
	case "ready_for_review":
		if run.reason != "" && run.reason != "unverified" {
			return v1Run{}, invalidV1(runID, "ready_for_review reason must be empty or unverified, got %q", run.reason)
		}
	}
	return run, nil
}

func invalidV1(runID, format string, args ...any) *v2contract.ControlError {
	return importError(v2contract.CodeInvalidContract, runID,
		"quarantine the run for operator repair; migration never reinterprets v1 data",
		format, args...)
}

// loadV1Attempts reads the run's attempts in attempt-number order and
// decodes each state word under the v1 contract.
func loadV1Attempts(ctx context.Context, tx *sql.Tx, runID string) ([]v1Attempt, error) {
	rows, err := tx.QueryContext(ctx, `SELECT attempt_id, task_id, state FROM attempts
		WHERE run_id = ? ORDER BY attempt_number`, runID)
	if err != nil {
		return nil, importError(v2contract.CodePersistenceUnavailable, runID,
			"retry once the ledger is writable", "reading v1 attempts: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []v1Attempt
	for rows.Next() {
		var a v1Attempt
		var state string
		if err := rows.Scan(&a.attemptID, &a.taskID, &state); err != nil {
			return nil, importError(v2contract.CodePersistenceUnavailable, runID,
				"retry once the ledger is writable", "reading v1 attempts: %v", err)
		}
		if a.attemptID == "" || a.taskID == "" {
			return nil, invalidV1(runID, "attempts row has an empty id")
		}
		if !v1AttemptStates[state] {
			return nil, invalidV1(runID, "attempts.state %q is not a v1 attempt state", state)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, importError(v2contract.CodePersistenceUnavailable, runID,
			"retry once the ledger is writable", "reading v1 attempts: %v", err)
	}
	return out, nil
}

// checkImportMarker refuses a run whose v2 rows already exist: the tasks
// row by task id, or any revision row by run id.
func checkImportMarker(ctx context.Context, tx *sql.Tx, taskID, runID string) error {
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM tasks WHERE task_id = ?`, taskID).Scan(&one)
	switch {
	case err == nil:
		return importError(v2contract.CodeRevisionConflict, runID,
			"inspect the stored import marker for this run; do not re-import",
			"run %s is already imported as task %s", runID, taskID)
	case !errors.Is(err, sql.ErrNoRows):
		return importError(v2contract.CodePersistenceUnavailable, runID,
			"retry once the ledger is writable", "reading import marker: %v", err)
	}
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM task_revisions WHERE run_id = ?`, runID).Scan(&one)
	switch {
	case err == nil:
		return importError(v2contract.CodeRevisionConflict, runID,
			"inspect the stored import marker for this run; do not re-import",
			"run %s already has an imported revision", runID)
	case !errors.Is(err, sql.ErrNoRows):
		return importError(v2contract.CodePersistenceUnavailable, runID,
			"retry once the ledger is writable", "reading import marker: %v", err)
	}
	return nil
}

// checkImportEnvelope compares the stored envelope for the generated
// event_id, if any, with the incoming one. A mismatch is
// revision_conflict; an identical envelope reports reuse, so the caller
// completes the import without appending a duplicate. Only the envelope's
// intrinsic content compares: producer/run sequences and observed_at are
// assigned at append time, never part of the identity.
func checkImportEnvelope(ctx context.Context, tx *sql.Tx, runID, taskID, attemptID string, payload []byte) (bool, error) {
	eventID := importEventPrefix + runID
	var schema int
	var storedRun, storedTask, storedAttempt, producer, typ, storedPayload string
	var generation int64
	err := tx.QueryRowContext(ctx, `SELECT schema_version, run_id, COALESCE(task_id, ''),
		COALESCE(attempt_id, ''), producer_id, type, payload, generation
		FROM journal WHERE event_id = ?`, eventID).Scan(
		&schema, &storedRun, &storedTask, &storedAttempt, &producer, &typ, &storedPayload, &generation)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, importError(v2contract.CodePersistenceUnavailable, runID,
			"retry once the ledger is writable", "reading import envelope: %v", err)
	}
	mismatch := ""
	switch {
	case schema != v2contract.SchemaVersion:
		mismatch = fmt.Sprintf("schema_version %d", schema)
	case storedRun != runID:
		mismatch = fmt.Sprintf("run_id %q", storedRun)
	case storedTask != taskID:
		mismatch = fmt.Sprintf("task_id %q", storedTask)
	case storedAttempt != attemptID:
		mismatch = fmt.Sprintf("attempt_id %q", storedAttempt)
	case producer != importProducerID:
		mismatch = fmt.Sprintf("producer_id %q", producer)
	case generation != importGeneration:
		mismatch = fmt.Sprintf("generation %d", generation)
	case typ != importedEventType:
		mismatch = fmt.Sprintf("type %q", typ)
	case storedPayload != string(payload):
		mismatch = "payload"
	}
	if mismatch != "" {
		return false, importError(v2contract.CodeRevisionConflict, runID,
			"inspect the stored import marker for this run; do not re-import",
			"stored %s envelope for run %s differs in %s", importedEventType, runID, mismatch)
	}
	return true, nil
}

// appendImported journals the migration.imported envelope with Append's
// sequencing (producer last+1, run MAX+1) inside the caller's transaction.
// Task 2's appendTx will own this path once it lands; until then the
// statements mirror Journal.Append line for line, minus the duplicate ack
// the marker layer already handled.
func appendImported(ctx context.Context, tx *sql.Tx, runID, taskID, attemptID string, payload []byte) error {
	ev := v2contract.Envelope{
		SchemaVersion: v2contract.SchemaVersion,
		EventID:       importEventPrefix + runID,
		RunID:         runID,
		TaskID:        taskID,
		AttemptID:     attemptID,
		ProducerID:    importProducerID,
		Generation:    importGeneration,
		ObservedAt:    time.Now().UTC(),
		Type:          importedEventType,
		Payload:       payload,
	}
	var lastSeq, generation int64
	err := tx.QueryRowContext(ctx, `SELECT last_sequence, generation FROM producers WHERE producer_id = ?`,
		ev.ProducerID).Scan(&lastSeq, &generation)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		lastSeq, generation = 0, ev.Generation
	case err != nil:
		return importError(v2contract.CodePersistenceUnavailable, runID,
			"retry once the ledger is writable", "reading producer: %v", err)
	}
	if generation != ev.Generation {
		return importError(v2contract.CodePersistenceUnavailable, runID,
			"retry once the ledger is writable",
			"producer %s is at generation %d, import appends at %d",
			ev.ProducerID, generation, ev.Generation)
	}
	ev.ProducerSequence = lastSeq + 1
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(run_sequence), 0) + 1 FROM journal WHERE run_id = ?`,
		ev.RunID).Scan(&ev.RunSequence); err != nil {
		return importError(v2contract.CodePersistenceUnavailable, runID,
			"retry once the ledger is writable", "assigning run sequence: %v", err)
	}
	if err := ev.Validate(); err != nil {
		return fmt.Errorf("migrate: built invalid %s envelope for run %s: %w", importedEventType, runID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO journal (event_id, schema_version, run_id, task_id, attempt_id,
		producer_id, producer_sequence, run_sequence, generation, caused_by, observed_at, type, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.EventID, ev.SchemaVersion, ev.RunID, nullable(ev.TaskID), nullable(ev.AttemptID),
		ev.ProducerID, ev.ProducerSequence, ev.RunSequence, ev.Generation, nullable(ev.CausedBy),
		ev.ObservedAt.Format(time.RFC3339Nano), ev.Type, string(ev.Payload)); err != nil {
		return importError(v2contract.CodePersistenceUnavailable, runID,
			"retry once the ledger is writable", "inserting event %s: %v", ev.EventID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO producers (producer_id, last_sequence, generation) VALUES (?, ?, ?)
		ON CONFLICT (producer_id) DO UPDATE SET last_sequence = excluded.last_sequence, generation = excluded.generation`,
		ev.ProducerID, ev.ProducerSequence, ev.Generation); err != nil {
		return importError(v2contract.CodePersistenceUnavailable, runID,
			"retry once the ledger is writable", "advancing producer %s: %v", ev.ProducerID, err)
	}
	return nil
}

func nullable(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// importError builds the catalogue ControlError for an import failure.
func importError(code v2contract.Code, runID, next, format string, args ...any) *v2contract.ControlError {
	return &v2contract.ControlError{
		Code:        code,
		Owner:       "migration",
		OperationID: "import:" + runID,
		Object:      runID,
		Disposition: code.DefaultDisposition(),
		NextAction:  next,
		Detail:      map[string]string{"migration/cause": fmt.Sprintf(format, args...)},
	}
}
