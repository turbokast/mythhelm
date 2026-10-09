// Package preview computes the hermetic migration plan for `mythhelm
// migrate --preview` (supervisor-migration design §6, D3): everything
// --apply would do, derived read-only from a caller-supplied handle.
//
// Hermeticity is the package's contract (I13): it imports neither
// internal/control nor any network package (pinned by
// TestPreviewHermetic with `go list -deps`), reads no environment
// variable, no clock and no credential, and writes nothing. The plan is
// a pure function of the database bytes, so it is byte-identical under
// any environment.
package preview

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

// supportedSchemaVersion mirrors journal.SchemaVersion and dbName mirrors
// journal.DBName. They are copies rather than imports because the preview
// planner stays dependency-lean for the hermeticity pin; TestPreviewPins
// fails when the mirrors drift, so a schema bump must sweep this file too.
const (
	dbName                 = "mythhelm.db"
	supportedSchemaVersion = 7
)

// migrationFiles names every schema migration in version order, mirroring
// internal/journal/migrations/*.sql. The preview derives the pending
// schema steps from the database's user_version without running them.
var migrationFiles = []string{
	"0001_init.sql",
	"0002_qualification.sql",
	"0003_ledger.sql",
	"0004_supervisor.sql",
	"0005_reservation_host.sql",
	"0006_evaluator.sql",
	"0007_v2contracts.sql",
}

// Owners lists the legacy owner sites Drain quiesces, in drain order
// (design §4): the interactive writers first, run execution next,
// recovery last. Each names a file holding an AcquireOwner site.
var Owners = []string{
	"internal/cli/apply.go",
	"internal/cli/tui.go",
	"internal/supervisor/pipeline.go",
	"internal/supervisor/recover.go",
}

// Run is one v1 run the plan would import: the legacy run id, the v2
// task id ImportRun would derive (the earliest attempt's legacy task id,
// or the run id when attempt-less), the imported revision (always 1) and
// the billing posture word copied verbatim. The shape mirrors
// migrate.ImportResult field-for-field (same JSON); it is a local type
// because importing internal/migrate would cycle (migrate consumes Plan).
type Run struct {
	RunID    string `json:"run_id"`
	TaskID   string `json:"task_id"`
	Revision int    `json:"revision"`
	Posture  string `json:"posture"`
}

// Plan is the hermetic preview: everything --apply would do, computed
// read-only (design §6).
type Plan struct {
	Runs     []Run    `json:"runs"`
	Owners   []string `json:"owners"`
	Steps    []string `json:"steps"`
	BackupTo string   `json:"backup_to"`
}

// PreviewPlan computes the migration plan from a read-only handle. It
// refuses a database newer than this binary (or a migration_state row
// naming a newer schema) with schema_too_new and otherwise never fails
// on old state: missing v1 tables read as no runs, so a bare or partial
// database previews instead of erroring. It writes nothing.
//
//nolint:revive // PreviewPlan is the spec's required exported name (tasks.md Task 6 Produces).
func PreviewPlan(ctx context.Context, db *sql.DB) (Plan, error) {
	if db == nil {
		return Plan{}, previewError(v2contract.CodePersistenceUnavailable,
			"open the state database read-only and retry", errors.New("nil database handle"))
	}
	version, err := userVersion(ctx, db)
	if err != nil {
		return Plan{}, previewError(v2contract.CodePersistenceUnavailable,
			"retry once the database answers; see detail migrate/cause", err)
	}
	if version > supportedSchemaVersion {
		return Plan{}, previewError(v2contract.CodeSchemaTooNew,
			fmt.Sprintf("upgrade mythhelm: database schema is v%d, this binary supports up to v%d",
				version, supportedSchemaVersion),
			fmt.Errorf("database schema v%d newer than supported v%d", version, supportedSchemaVersion))
	}
	if newer, err := migrationStateNewer(ctx, db); err != nil {
		return Plan{}, previewError(v2contract.CodePersistenceUnavailable,
			"retry once the database answers; see detail migrate/cause", err)
	} else if newer {
		return Plan{}, previewError(v2contract.CodeSchemaTooNew,
			fmt.Sprintf("upgrade mythhelm: migration_state names a schema newer than v%d", supportedSchemaVersion),
			fmt.Errorf("migration_state schema is newer than supported v%d", supportedSchemaVersion))
	}
	runs, err := planRuns(ctx, db)
	if err != nil {
		return Plan{}, previewError(v2contract.CodePersistenceUnavailable,
			"retry once the database answers; see detail migrate/cause", err)
	}
	dir, err := stateDir(ctx, db)
	if err != nil {
		return Plan{}, previewError(v2contract.CodePersistenceUnavailable,
			"retry once the database answers; see detail migrate/cause", err)
	}
	plan := Plan{
		Runs:     runs,
		Owners:   append([]string(nil), Owners...),
		Steps:    pendingSteps(version),
		BackupTo: filepath.Join(dir, fmt.Sprintf("%s.bak-migration-v%d", dbName, supportedSchemaVersion)),
	}
	return plan, nil
}

// userVersion reads PRAGMA user_version: the schema version the pending
// steps derive from.
func userVersion(ctx context.Context, db *sql.DB) (int, error) {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("reading user_version: %w", err)
	}
	return version, nil
}

// migrationStateNewer reports whether migration_state names a schema
// newer than this binary supports (design §5 downgrade refusal). A
// database predating the table is never newer.
func migrationStateNewer(ctx context.Context, db *sql.DB) (bool, error) {
	var version int
	err := db.QueryRowContext(ctx, `SELECT schema_version FROM migration_state WHERE id = 1`).Scan(&version)
	switch {
	case err == nil:
		return version > supportedSchemaVersion, nil
	case errors.Is(err, sql.ErrNoRows) || isNoSuchTable(err):
		return false, nil
	default:
		return false, fmt.Errorf("reading migration_state: %w", err)
	}
}

// planRuns lists every v1 run with the task id and posture the import
// would use, in run id order. A database without the v1 tables (empty
// or partial) previews zero runs rather than failing.
func planRuns(ctx context.Context, db *sql.DB) ([]Run, error) {
	rows, err := db.QueryContext(ctx, `SELECT run_id, billing_posture FROM runs ORDER BY run_id`)
	if err != nil {
		if isNoSuchTable(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var runs []Run
	for rows.Next() {
		var r Run
		r.Revision = 1
		if err := rows.Scan(&r.RunID, &r.Posture); err != nil {
			return nil, fmt.Errorf("reading runs: %w", err)
		}
		r.TaskID = r.RunID
		var taskID string
		err := db.QueryRowContext(ctx, `SELECT task_id FROM attempts
			WHERE run_id = ? ORDER BY attempt_number LIMIT 1`, r.RunID).Scan(&taskID)
		switch {
		case err == nil:
			r.TaskID = taskID
		case errors.Is(err, sql.ErrNoRows) || isNoSuchTable(err):
		default:
			return nil, fmt.Errorf("reading attempts of run %s: %w", r.RunID, err)
		}
		runs = append(runs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading runs: %w", err)
	}
	return runs, nil
}

// pendingSteps names the migration files user_version still needs:
// versions (version, supported]. A current database previews no steps;
// the names pin the apply-time migration the preview advertises.
func pendingSteps(version int) []string {
	var steps []string
	for v := version + 1; v <= supportedSchemaVersion && v <= len(migrationFiles); v++ {
		steps = append(steps, migrationFiles[v-1])
	}
	return steps
}

// stateDir resolves the state directory holding db: the parent of the
// main database file, so BackupTo names the backup beside the ledger.
func stateDir(ctx context.Context, db *sql.DB) (string, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA database_list")
	if err != nil {
		return "", fmt.Errorf("listing databases: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var seq int
		var name, file string
		if err := rows.Scan(&seq, &name, &file); err != nil {
			return "", fmt.Errorf("reading database list: %w", err)
		}
		if name == "main" {
			if file == "" {
				return "", errors.New("the ledger has no database file")
			}
			return filepath.Dir(file), nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("reading database list: %w", err)
	}
	return "", errors.New("the ledger has no main database")
}

// isNoSuchTable reports the SQLite "no such table" error: a database
// predating the queried table, which the preview tolerates.
func isNoSuchTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

// previewError builds the catalogue ControlError for a preview failure:
// the code default disposition (never widened) with the cause under the
// namespaced migrate/cause detail key, so every error passes Validate.
func previewError(code v2contract.Code, nextAction string, cause error) *v2contract.ControlError {
	detail := "unknown"
	if cause != nil {
		detail = cause.Error()
	}
	return &v2contract.ControlError{
		Code:        code,
		Owner:       "migrate",
		OperationID: "preview",
		Disposition: code.DefaultDisposition(),
		NextAction:  nextAction,
		Detail:      map[string]string{"migrate/cause": detail},
	}
}
