package migrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/turbokast/mythhelm/internal/buildinfo"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/migrate/preview"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

const (
	// quarantinedEventType is the v2 envelope type recording one
	// quarantined run (design §4, D4): ledger-native, so stream 4
	// reconciles it with the same machinery as any quarantined attempt.
	quarantinedEventType = "migration.quarantined"
	// quarantineEventPrefix prefixes the deterministic event_id of a
	// quarantine envelope: quarantineEventPrefix + the legacy run id.
	quarantineEventPrefix = "migration-quarantined-"
	// quarantineProducerID is the journal producer every migration
	// envelope attributes to, shared with the import envelopes so the
	// migration's appends sequence together.
	quarantineProducerID = "migration"
	// quarantineGeneration is the generation the migration producer
	// appends at, shared with the import envelopes.
	quarantineGeneration = 0
	// quarantineReason records why the drain quarantined the run: no
	// live worker with a verified launch identity held it at quiesce
	// (I12 — adopt-only-same-launch-identity).
	quarantineReason = "drain found no live worker with verified launch identity"
)

// quarantinePayload is the migration.quarantined envelope payload: the
// run id, the last known launch identity (the latest attempt's ids and
// launch token hash, empty when the run never launched) and the
// quarantine timestamp.
type quarantinePayload struct {
	RunID             string `json:"run_id"`
	AttemptID         string `json:"attempt_id,omitempty"`
	TaskID            string `json:"task_id,omitempty"`
	LaunchTokenSHA256 string `json:"launch_token_sha256,omitempty"`
	QuarantinedAt     string `json:"quarantined_at"`
	Reason            string `json:"reason"`
}

// Apply executes Drain → Backup → Import → Adopt for the previewed plan
// (supervisor-migration design §6). Drain quiesces without writing;
// Backup copies the quiesced ledger; Import records phase drained,
// persists the quarantine envelopes from the DrainReport, imports every
// v1 run, then records imported; Adopt records adopted, handing the
// ledger to the service (which lazy-starts on next use — Apply never
// spawns it).
//
// Every ledger-mutating write runs inside control.Mutate; the Backup
// step's single VACUUM INTO is the design-excluded file copy. On step
// failure Apply returns the drain report with the error and the phase
// row names the last completed step, so --apply is resumable: the drain
// re-quiesces, the backup is reused by digest (never re-taken over an
// existing one), quarantine envelopes dedupe by event_id, and imported
// runs skip by their import markers — resume never replays effects
// (I12). A run that appears after the drain aborts with
// ownership_unresolved: post-drain admissions are fatal, never silent.
//
// The caller migrates the schema first (journal.Open or
// control.OpenLedger): Apply fails fast when migration_state is missing.
// Every run in plan must still exist at import time, or Apply refuses
// with invalid_contract and the operator re-previews.
func Apply(ctx context.Context, db *sql.DB, plan preview.Plan) (DrainReport, error) {
	if db == nil {
		return DrainReport{}, applyError(v2contract.CodePersistenceUnavailable, "apply", "",
			"pass the open state database handle", "the ledger handle is nil")
	}
	phase, backupPath, err := readMigrationState(ctx, db)
	if err != nil {
		return DrainReport{}, err
	}
	switch phase {
	case string(PhaseAdopted):
		return DrainReport{}, nil
	case string(PhaseImported):
		if err := recordPhase(ctx, db, PhaseAdopted); err != nil {
			return DrainReport{}, err
		}
		return DrainReport{}, nil
	}
	if phase == string(PhaseNotStarted) {
		if err := markPreviewed(ctx, db); err != nil {
			return DrainReport{}, err
		}
	}

	report, err := Drain(ctx, db)
	if err != nil {
		return DrainReport{}, err
	}
	stateDir, err := drainStateDir(ctx, db)
	if err != nil {
		return report, err
	}
	// The migration lock for the rest of the run (design §4): while it
	// is held no service writer starts and no legacy admission proceeds.
	// Drain acquired and released the same pair internally, so the lock
	// cannot span the Drain call itself; the surprise check below closes
	// the microsecond window between Drain's release and this acquire.
	releaseInstance, err := control.AcquireInstance(stateDir)
	if err != nil {
		return report, lockError("apply", err,
			"a supervisor is already running; stop it and retry the migration")
	}
	defer releaseInstance()
	releaseMigration, err := supervisor.AcquireOwner(stateDir)
	if err != nil {
		return report, lockError("apply", err,
			"another migration holds the state directory; retry after it finishes")
	}
	defer releaseMigration()

	current, err := currentRunIDs(ctx, db, stateDir)
	if err != nil {
		return report, err
	}
	if surprise := findSurprise(current, report); surprise != "" {
		return report, applyError(v2contract.CodeOwnershipUnresolved, "apply", surprise,
			"retry the migration after it settles",
			"run %s started during the drain; nothing past the drain ran", surprise)
	}
	// Before the first ledger write: a stale plan refuses with no backup
	// taken and no phase advanced.
	if err := checkPlanRuns(current, plan); err != nil {
		return report, err
	}
	info, err := backupOrReuse(ctx, db, stateDir, backupPath)
	if err != nil {
		return report, err
	}
	if info.Path != backupPath {
		if err := recordBackup(ctx, db, info.Path); err != nil {
			return report, err
		}
	}
	if err := recordPhase(ctx, db, PhaseDrained); err != nil {
		return report, err
	}
	if err := persistQuarantine(ctx, db, report.Quarantined); err != nil {
		return report, err
	}
	if err := importRuns(ctx, db); err != nil {
		return report, err
	}
	if err := recordPhase(ctx, db, PhaseImported); err != nil {
		return report, err
	}
	if err := recordPhase(ctx, db, PhaseAdopted); err != nil {
		return report, err
	}
	return report, nil
}

// readMigrationState returns the durable phase and recorded backup path.
// A database without migration_state has not been migrated yet, so Apply
// — which only runs on a migrated ledger — fails fast instead of
// guessing.
func readMigrationState(ctx context.Context, db *sql.DB) (phase, backupPath string, err error) {
	err = db.QueryRowContext(ctx, `SELECT phase, backup_path FROM migration_state WHERE id = 1`).
		Scan(&phase, &backupPath)
	if err == nil {
		return phase, backupPath, nil
	}
	if errors.Is(err, sql.ErrNoRows) || isMissingMigrationState(err) {
		return "", "", applyError(v2contract.CodeInvalidContract, "apply", "",
			"open the state database once (any mythhelm command migrates it) and retry",
			"the ledger has no migration_state; migrate a migrated database")
	}
	return "", "", applyError(v2contract.CodePersistenceUnavailable, "apply", "",
		"retry once the database answers; see detail migrate/cause", "reading migration_state: %v", err)
}

// isMissingMigrationState reports the SQLite "no such table" error for a
// database predating the migration_state table.
func isMissingMigrationState(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

// markPreviewed records the previewed phase with the start marker and
// build identity on the first apply; resumes keep the original started_at.
func markPreviewed(ctx context.Context, db *sql.DB) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	build := buildinfo.Get().Version
	return control.Mutate(ctx, db, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE migration_state SET phase = 'previewed',
			started_at = CASE WHEN started_at = '' THEN ? ELSE started_at END,
			build_version = ? WHERE id = 1`, now, build)
		if err != nil {
			return applyError(v2contract.CodePersistenceUnavailable, "apply", "",
				"retry once the ledger is writable", "recording previewed: %v", err)
		}
		return nil
	})
}

// recordBackup records the backup path with the start marker and build
// identity fill, so a resume reuses the backup instead of taking another.
func recordBackup(ctx context.Context, db *sql.DB, path string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	build := buildinfo.Get().Version
	return control.Mutate(ctx, db, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE migration_state SET backup_path = ?,
			started_at = CASE WHEN started_at = '' THEN ? ELSE started_at END,
			build_version = ? WHERE id = 1`, path, now, build)
		if err != nil {
			return applyError(v2contract.CodePersistenceUnavailable, "apply", "",
				"retry once the ledger is writable", "recording the backup: %v", err)
		}
		return nil
	})
}

// setPhase advances the durable migration phase.
func recordPhase(ctx context.Context, db *sql.DB, phase Phase) error {
	return control.Mutate(ctx, db, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE migration_state SET phase = ? WHERE id = 1`, string(phase))
		if err != nil {
			return applyError(v2contract.CodePersistenceUnavailable, "apply", "",
				"retry once the ledger is writable", "recording phase %s: %v", phase, err)
		}
		return nil
	})
}

// currentRunIDs lists every v1 run through Drain's own enumeration, so
// the surprise check compares like with like by construction and can
// never drift from what Drain quiesced.
func currentRunIDs(ctx context.Context, db *sql.DB, stateDir string) ([]string, error) {
	runs, err := enumerateRuns(ctx, db, stateDir)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(runs))
	for i, r := range runs {
		ids[i] = r.id
	}
	return ids, nil
}

// findSurprise names the first current run the drain report does not know:
// an admission that landed between Drain's release and the migration
// lock. Empty means the quiesce still covers every run.
func findSurprise(current []string, report DrainReport) string {
	known := map[string]bool{}
	for _, id := range report.Drained {
		known[id] = true
	}
	for _, id := range report.Adopted {
		known[id] = true
	}
	for _, id := range report.Quarantined {
		known[id] = true
	}
	for _, id := range current {
		if !known[id] {
			return id
		}
	}
	return ""
}

// backupOrReuse takes the pre-migration backup, or reuses a valid one a
// previous attempt already took: Backup never overwrites, so a resume
// must not call it blindly. Both backup and sidecar present verifies by
// digest; exactly one present is a half-state the operator moves aside;
// neither present takes a fresh backup — unless a backup was recorded,
// in which case the recorded pre-migration copy is irreplaceable (a
// resume may already hold committed migration rows) and its absence
// refuses.
func backupOrReuse(ctx context.Context, db *sql.DB, stateDir, recorded string) (BackupInfo, error) {
	candidate := recorded
	if candidate == "" {
		candidate = filepath.Join(stateDir, fmt.Sprintf("%s.bak-migration-v%d", dbName, supportedSchemaVersion))
	}
	_, dstErr := os.Lstat(candidate)
	_, scErr := os.Lstat(candidate + ".json")
	switch {
	case dstErr == nil && scErr == nil:
		info, err := verifyBackup(candidate)
		if err != nil {
			return BackupInfo{}, err
		}
		return info, nil
	case errors.Is(dstErr, os.ErrNotExist) && errors.Is(scErr, os.ErrNotExist):
		if recorded != "" {
			// A resume may already hold committed imports or
			// quarantine envelopes: a fresh copy would not be
			// pre-migration state, so the recorded backup is
			// irreplaceable — refuse rather than re-take.
			return BackupInfo{}, applyError(v2contract.CodeInvalidContract, "apply", "",
				"restore the recorded pre-migration backup to its path before re-applying",
				"recorded backup %s is missing; refusing to replace the pre-migration backup", recorded)
		}
		return Backup(ctx, db, stateDir)
	case errors.Is(dstErr, os.ErrNotExist) || errors.Is(scErr, os.ErrNotExist):
		return BackupInfo{}, applyError(v2contract.CodeInvalidContract, "apply", "",
			"move the half-written backup aside; mythhelm never overwrites one",
			"%s holds only half a backup; refusing to complete or replace it", candidate)
	default:
		cause := dstErr
		if cause == nil {
			cause = scErr
		}
		return BackupInfo{}, applyError(v2contract.CodePersistenceUnavailable, "apply", "",
			"retry once the backup target answers; see detail migrate/cause",
			"inspecting %s: %v", candidate, cause)
	}
}

// verifyBackup loads the sidecar beside a previous attempt's backup and
// verifies the copy still matches its digest and schema before reusing
// it. A mismatch refuses: the resume must not bless a tampered copy.
func verifyBackup(dst string) (BackupInfo, error) {
	raw, err := os.ReadFile(dst + ".json") //nolint:gosec // G304: Apply's own backup sidecar, built from the state dir
	if err != nil {
		return BackupInfo{}, applyError(v2contract.CodePersistenceUnavailable, "apply", "",
			"retry once the backup target answers; see detail migrate/cause",
			"reading %s: %v", dst+".json", err)
	}
	var info BackupInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return BackupInfo{}, applyError(v2contract.CodeInvalidContract, "apply", "",
			"move the unreadable backup aside and retry from a fresh backup",
			"%s is not a backup sidecar: %v", dst+".json", err)
	}
	digest, err := hashFile(dst)
	if err != nil {
		return BackupInfo{}, applyError(v2contract.CodePersistenceUnavailable, "apply", "",
			"retry once the backup target answers; see detail migrate/cause",
			"reading %s: %v", dst, err)
	}
	if info.SHA256 == "" || digest != info.SHA256 {
		return BackupInfo{}, applyError(v2contract.CodeInvalidContract, "apply", "",
			"move the tampered backup aside and retry from a fresh backup",
			"backup %s fails its sidecar digest", dst)
	}
	if info.SchemaVersion != supportedSchemaVersion {
		return BackupInfo{}, applyError(v2contract.CodeInvalidContract, "apply", "",
			"move the stale backup aside and retry from a fresh backup",
			"backup %s is schema v%d, not v%d", dst, info.SchemaVersion, supportedSchemaVersion)
	}
	info.Path = dst
	return info, nil
}

// checkPlanRuns refuses when a run the plan named is gone from the
// ledger: the import would otherwise run under a stale plan. Fresh runs
// beyond the plan import normally — the plan is a floor, not a fence.
func checkPlanRuns(current []string, plan preview.Plan) error {
	have := map[string]bool{}
	for _, id := range current {
		have[id] = true
	}
	for _, r := range plan.Runs {
		if !have[r.RunID] {
			return applyError(v2contract.CodeInvalidContract, "apply", r.RunID,
				"re-run migrate --preview and apply the fresh plan",
				"the plan names run %s, which is no longer in the ledger", r.RunID)
		}
	}
	return nil
}

// persistQuarantine journals one migration.quarantined envelope per
// quarantined run in a single Mutate, after the backup step. A resume
// replays this batch: envelopes already journaled with identical content
// skip by event_id, never duplicate.
func persistQuarantine(ctx context.Context, db *sql.DB, runIDs []string) error {
	if len(runIDs) == 0 {
		return nil
	}
	return control.Mutate(ctx, db, func(tx *sql.Tx) error {
		for _, runID := range runIDs {
			reuse, payload, err := checkQuarantineEnvelope(ctx, tx, runID)
			if err != nil {
				return err
			}
			if reuse {
				continue
			}
			if err := appendQuarantined(ctx, tx, runID, payload); err != nil {
				return err
			}
		}
		return nil
	})
}

// quarantineIdentity is the last known launch identity of a run: its
// latest attempt's ids and launch token hash, empty when never launched.
type quarantineIdentity struct {
	attemptID string
	taskID    string
	tokenSHA  string
}

// quarantineEnvelope rebuilds the deterministic quarantine envelope
// content for runID: the payload plus the journal identity columns, so
// the dedupe check and the append compare and write the same bytes.
func quarantineEnvelope(ctx context.Context, tx *sql.Tx, runID string) (quarantinePayload, quarantineIdentity, error) {
	var id quarantineIdentity
	err := tx.QueryRowContext(ctx, `SELECT attempt_id, task_id, launch_token_sha256 FROM attempts
		WHERE run_id = ? ORDER BY attempt_number DESC LIMIT 1`, runID).
		Scan(&id.attemptID, &id.taskID, &id.tokenSHA)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return quarantinePayload{}, id, applyError(v2contract.CodePersistenceUnavailable, "apply", runID,
			"retry once the ledger is writable", "reading attempts of run %s: %v", runID, err)
	}
	payload := quarantinePayload{
		RunID:             runID,
		AttemptID:         id.attemptID,
		TaskID:            id.taskID,
		LaunchTokenSHA256: id.tokenSHA,
		QuarantinedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		Reason:            quarantineReason,
	}
	return payload, id, nil
}

// checkQuarantineEnvelope compares the stored envelope for the run's
// deterministic event_id, if any, with the incoming one. Only intrinsic
// content compares: sequences and observed_at are assigned at append
// time, and quarantined_at stamps the attempt, so the payload compares
// without it. An identical envelope reports reuse; a mismatch is
// revision_conflict.
func checkQuarantineEnvelope(ctx context.Context, tx *sql.Tx, runID string) (bool, quarantinePayload, error) {
	payload, id, err := quarantineEnvelope(ctx, tx, runID)
	if err != nil {
		return false, payload, err
	}
	eventID := quarantineEventPrefix + runID
	var schema int
	var storedRun, storedTask, storedAttempt, producer, typ, storedPayload string
	var generation int64
	err = tx.QueryRowContext(ctx, `SELECT schema_version, run_id, COALESCE(task_id, ''),
		COALESCE(attempt_id, ''), producer_id, type, payload, generation
		FROM journal WHERE event_id = ?`, eventID).Scan(
		&schema, &storedRun, &storedTask, &storedAttempt, &producer, &typ, &storedPayload, &generation)
	if errors.Is(err, sql.ErrNoRows) {
		return false, payload, nil
	}
	if err != nil {
		return false, payload, applyError(v2contract.CodePersistenceUnavailable, "apply", runID,
			"retry once the ledger is writable", "reading quarantine envelope: %v", err)
	}
	mismatch := ""
	switch {
	case schema != v2contract.SchemaVersion:
		mismatch = fmt.Sprintf("schema_version %d", schema)
	case storedRun != runID:
		mismatch = fmt.Sprintf("run_id %q", storedRun)
	case storedTask != id.taskID:
		mismatch = fmt.Sprintf("task_id %q", storedTask)
	case storedAttempt != id.attemptID:
		mismatch = fmt.Sprintf("attempt_id %q", storedAttempt)
	case producer != quarantineProducerID:
		mismatch = fmt.Sprintf("producer_id %q", producer)
	case generation != quarantineGeneration:
		mismatch = fmt.Sprintf("generation %d", generation)
	case typ != quarantinedEventType:
		mismatch = fmt.Sprintf("type %q", typ)
	default:
		var stored, incoming map[string]any
		if json.Unmarshal([]byte(storedPayload), &stored) != nil ||
			json.Unmarshal(mustMarshal(payload), &incoming) != nil {
			mismatch = "payload"
			break
		}
		// quarantined_at stamps the persist attempt, not the run's
		// identity: a resume re-stamps without conflicting.
		delete(stored, "quarantined_at")
		delete(incoming, "quarantined_at")
		storedRaw, _ := json.Marshal(stored)
		incomingRaw, _ := json.Marshal(incoming)
		if string(storedRaw) != string(incomingRaw) {
			mismatch = "payload"
		}
	}
	if mismatch != "" {
		return false, payload, applyError(v2contract.CodeRevisionConflict, "apply", runID,
			"inspect the stored quarantine envelope for this run; do not re-persist",
			"stored %s envelope for run %s differs in %s", quarantinedEventType, runID, mismatch)
	}
	return true, payload, nil
}

func mustMarshal(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("migrate: encoding quarantine payload: %v", err))
	}
	return raw
}

// appendQuarantined journals the migration.quarantined envelope with
// Append's sequencing (producer last+1, run MAX+1) and generation rule
// (accept at or ahead, advance the producer) inside the caller's
// transaction, mirroring import.go's appendImported: Task 2's appendTx is
// unexported and unreachable cross-package, and calling db-level Append
// from inside Mutate would contend with the open transaction.
func appendQuarantined(ctx context.Context, tx *sql.Tx, runID string, payload quarantinePayload) error {
	raw := mustMarshal(payload)
	ev := v2contract.Envelope{
		SchemaVersion: v2contract.SchemaVersion,
		EventID:       quarantineEventPrefix + runID,
		RunID:         runID,
		TaskID:        payload.TaskID,
		AttemptID:     payload.AttemptID,
		ProducerID:    quarantineProducerID,
		Generation:    quarantineGeneration,
		ObservedAt:    time.Now().UTC(),
		Type:          quarantinedEventType,
		Payload:       raw,
	}
	var lastSeq, generation int64
	err := tx.QueryRowContext(ctx, `SELECT last_sequence, generation FROM producers WHERE producer_id = ?`,
		ev.ProducerID).Scan(&lastSeq, &generation)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		lastSeq, generation = 0, ev.Generation
	case err != nil:
		return applyError(v2contract.CodePersistenceUnavailable, "apply", runID,
			"retry once the ledger is writable", "reading producer: %v", err)
	}
	if ev.Generation < generation {
		return applyError(v2contract.CodeRevisionConflict, "apply", runID,
			"inspect the migration producer row; a newer generation owns it",
			"producer %s is at generation %d, quarantine appends at %d",
			ev.ProducerID, generation, ev.Generation)
	}
	ev.ProducerSequence = lastSeq + 1
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(run_sequence), 0) + 1 FROM journal WHERE run_id = ?`,
		ev.RunID).Scan(&ev.RunSequence); err != nil {
		return applyError(v2contract.CodePersistenceUnavailable, "apply", runID,
			"retry once the ledger is writable", "assigning run sequence: %v", err)
	}
	if err := ev.Validate(); err != nil {
		return applyError(v2contract.CodeInvalidContract, "apply", runID,
			"report this mythhelm bug: the quarantine envelope failed validation",
			"built invalid %s envelope for run %s: %v", quarantinedEventType, runID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO journal (event_id, schema_version, run_id, task_id, attempt_id,
		producer_id, producer_sequence, run_sequence, generation, caused_by, observed_at, type, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.EventID, ev.SchemaVersion, ev.RunID, nullable(ev.TaskID), nullable(ev.AttemptID),
		ev.ProducerID, ev.ProducerSequence, ev.RunSequence, ev.Generation, nullable(ev.CausedBy),
		ev.ObservedAt.Format(time.RFC3339Nano), ev.Type, string(ev.Payload)); err != nil {
		return applyError(v2contract.CodePersistenceUnavailable, "apply", runID,
			"retry once the ledger is writable", "inserting event %s: %v", ev.EventID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO producers (producer_id, last_sequence, generation) VALUES (?, ?, ?)
		ON CONFLICT (producer_id) DO UPDATE SET last_sequence = excluded.last_sequence, generation = excluded.generation`,
		ev.ProducerID, ev.ProducerSequence, ev.Generation); err != nil {
		return applyError(v2contract.CodePersistenceUnavailable, "apply", runID,
			"retry once the ledger is writable", "advancing producer %s: %v", ev.ProducerID, err)
	}
	return nil
}

// importRuns imports every v1 run in run id order, one Mutate each, so a
// failed run leaves earlier imports committed and the resume skips them
// by their task_revisions markers. Directory-only entries (half-admitted,
// quarantined by the drain) are not v1 runs and stay for stream-4
// reconciliation from their quarantine envelopes.
func importRuns(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT run_id FROM runs ORDER BY run_id`)
	if err != nil {
		return applyError(v2contract.CodePersistenceUnavailable, "apply", "",
			"retry once the database answers; see detail migrate/cause", "listing runs: %v", err)
	}
	var runIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return applyError(v2contract.CodePersistenceUnavailable, "apply", "",
				"retry once the database answers; see detail migrate/cause", "reading runs: %v", err)
		}
		runIDs = append(runIDs, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return applyError(v2contract.CodePersistenceUnavailable, "apply", "",
			"retry once the database answers; see detail migrate/cause", "reading runs: %v", err)
	}
	_ = rows.Close()
	for _, runID := range runIDs {
		if err := control.Mutate(ctx, db, func(tx *sql.Tx) error {
			var one int
			err := tx.QueryRowContext(ctx, `SELECT 1 FROM task_revisions WHERE run_id = ?`, runID).Scan(&one)
			switch {
			case err == nil:
				return nil // already imported by a previous attempt
			case !errors.Is(err, sql.ErrNoRows):
				return applyError(v2contract.CodePersistenceUnavailable, "apply", runID,
					"retry once the ledger is writable", "reading import marker: %v", err)
			}
			_, err = ImportRun(ctx, tx, runID)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

// applyError builds the catalogue ControlError for an apply failure:
// Owner migrate, the operation (suffixed with the run when one is in
// scope), the code default disposition and the cause under the
// namespaced migrate/cause detail key, so every error passes Validate.
func applyError(code v2contract.Code, op, runID, next, format string, args ...any) *v2contract.ControlError {
	if runID != "" {
		op += ":" + runID
	}
	return &v2contract.ControlError{
		Code:        code,
		Owner:       "migrate",
		OperationID: op,
		Object:      runID,
		Disposition: code.DefaultDisposition(),
		NextAction:  next,
		Detail:      map[string]string{"migrate/cause": fmt.Sprintf(format, args...)},
	}
}
