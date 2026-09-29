package journal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// The runs and attempts tables are projections: each row holds an object's
// current state, derived from the journal. Writers change them only inside
// Append's project callback, so a projection commits or rolls back with the
// event it reflects (AC-9.3). Readers such as `runs list` read projections,
// never the journal.

var (
	// ErrNotFound reports a run or attempt with no projection row.
	ErrNotFound = errors.New("not found")
	// ErrNoDatabase reports a state directory with no initialised database,
	// which means no run has been recorded there.
	ErrNoDatabase = errors.New("no state database")
)

// RunRow is a run's projection. Reason, SourceBranch and BaseRev are empty
// when unset (NULL in the database).
type RunRow struct {
	RunID            string
	State            string
	Reason           string
	AdapterID        string
	SourceRepo       string
	SourceBranch     string
	BaseRev          string
	TaskSHA256       string
	BillingPosture   string
	ExecutionProfile string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// AttemptRow is the part of an attempt's projection known when its launch
// intent is recorded.
type AttemptRow struct {
	AttemptID         string
	RunID             string
	TaskID            string
	AttemptNumber     int64
	State             string
	Reason            string
	LaunchTokenSHA256 string
	WorkspacePath     string
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// InsertRun adds r's projection row inside tx.
func InsertRun(ctx context.Context, tx *sql.Tx, r RunRow) error {
	if r.RunID == "" || r.State == "" || r.AdapterID == "" || r.SourceRepo == "" || r.TaskSHA256 == "" ||
		r.BillingPosture == "" || r.ExecutionProfile == "" || r.CreatedAt.IsZero() || r.UpdatedAt.IsZero() {
		return fmt.Errorf("journal: run %q projection is missing a required field", r.RunID)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO runs (run_id, state, reason, adapter_id, source_repo, source_branch,
		base_rev, task_sha256, billing_posture, execution_profile, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.RunID, r.State, nullable(r.Reason), r.AdapterID, r.SourceRepo, nullable(r.SourceBranch),
		nullable(r.BaseRev), r.TaskSHA256, r.BillingPosture, r.ExecutionProfile,
		formatTime(r.CreatedAt), formatTime(r.UpdatedAt)); err != nil {
		return fmt.Errorf("journal: inserting run %s: %w", r.RunID, err)
	}
	return nil
}

// CurrentRunState reads runID's projected state inside tx.
func CurrentRunState(ctx context.Context, tx *sql.Tx, runID string) (string, error) {
	var state string
	err := tx.QueryRowContext(ctx, `SELECT state FROM runs WHERE run_id = ?`, runID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("journal: run %s: %w", runID, ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("journal: reading run %s: %w", runID, err)
	}
	return state, nil
}

// SetRunState sets runID's projected state, reason and updated_at inside tx.
func SetRunState(ctx context.Context, tx *sql.Tx, runID, state, reason string, at time.Time) error {
	res, err := tx.ExecContext(ctx, `UPDATE runs SET state = ?, reason = ?, updated_at = ? WHERE run_id = ?`,
		state, nullable(reason), formatTime(at), runID)
	if err != nil {
		return fmt.Errorf("journal: updating run %s: %w", runID, err)
	}
	return requireOneRow(res, "run", runID)
}

// InsertAttempt adds a's projection row inside tx.
func InsertAttempt(ctx context.Context, tx *sql.Tx, a AttemptRow) error {
	if a.AttemptID == "" || a.RunID == "" || a.TaskID == "" || a.AttemptNumber < 1 || a.State == "" ||
		a.LaunchTokenSHA256 == "" || a.WorkspacePath == "" {
		return fmt.Errorf("journal: attempt %q projection is missing a required field", a.AttemptID)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO attempts (attempt_id, run_id, task_id, attempt_number, state,
		reason, launch_token_sha256, workspace_path) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.AttemptID, a.RunID, a.TaskID, a.AttemptNumber, a.State, nullable(a.Reason),
		a.LaunchTokenSHA256, a.WorkspacePath); err != nil {
		return fmt.Errorf("journal: inserting attempt %s: %w", a.AttemptID, err)
	}
	return nil
}

// CurrentAttemptState reads attemptID's projected state inside tx.
func CurrentAttemptState(ctx context.Context, tx *sql.Tx, attemptID string) (string, error) {
	var state string
	err := tx.QueryRowContext(ctx, `SELECT state FROM attempts WHERE attempt_id = ?`, attemptID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("journal: attempt %s: %w", attemptID, ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("journal: reading attempt %s: %w", attemptID, err)
	}
	return state, nil
}

// SetAttemptState sets attemptID's projected state and reason inside tx.
func SetAttemptState(ctx context.Context, tx *sql.Tx, attemptID, state, reason string) error {
	res, err := tx.ExecContext(ctx, `UPDATE attempts SET state = ?, reason = ? WHERE attempt_id = ?`,
		state, nullable(reason), attemptID)
	if err != nil {
		return fmt.Errorf("journal: updating attempt %s: %w", attemptID, err)
	}
	return requireOneRow(res, "attempt", attemptID)
}

func requireOneRow(res sql.Result, kind, id string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("journal: updating %s %s: %w", kind, id, err)
	}
	if n != 1 {
		return fmt.Errorf("journal: %s %s: %w", kind, id, ErrNotFound)
	}
	return nil
}

// Attempt returns attemptID's projection.
func (j *Journal) Attempt(ctx context.Context, attemptID string) (AttemptRow, error) {
	var a AttemptRow
	var reason sql.NullString
	err := j.db.QueryRowContext(ctx, `SELECT attempt_id, run_id, task_id, attempt_number, state, reason,
		launch_token_sha256, workspace_path FROM attempts WHERE attempt_id = ?`, attemptID).Scan(
		&a.AttemptID, &a.RunID, &a.TaskID, &a.AttemptNumber, &a.State, &reason, &a.LaunchTokenSHA256, &a.WorkspacePath)
	if errors.Is(err, sql.ErrNoRows) {
		return AttemptRow{}, fmt.Errorf("journal: attempt %s: %w", attemptID, ErrNotFound)
	}
	if err != nil {
		return AttemptRow{}, fmt.Errorf("journal: reading attempt %s: %w", attemptID, err)
	}
	a.Reason = reason.String
	return a, nil
}

// OpenReadOnly opens dir/mythhelm.db for reading projections. The handle
// refuses writes, and opening never creates, migrates or writes the database
// file. Like any SQLite reader of a WAL database, it may create the WAL
// index (-shm) and an empty -wal beside it.
// It returns ErrNoDatabase when no database has been initialised in dir, and
// ErrSchemaTooNew for a database written by a newer MYTHHELM (AC-9.4).
func OpenReadOnly(ctx context.Context, dir string) (*Journal, error) {
	path := filepath.Join(dir, DBName)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoDatabase
	} else if err != nil {
		return nil, fmt.Errorf("inspecting %s: %w", path, err)
	}
	db, err := sql.Open("sqlite", dsn(path, "mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)"))
	if err != nil {
		return nil, fmt.Errorf("opening %s read-only: %w", path, err)
	}
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("reading schema version of %s: %w", path, err)
	}
	switch {
	case version > SchemaVersion:
		_ = db.Close()
		return nil, checkVersion(path, version, SchemaVersion)
	case version == 0:
		_ = db.Close()
		return nil, ErrNoDatabase
	case version < SchemaVersion:
		_ = db.Close()
		return nil, fmt.Errorf("%s has schema version %d, older than %d; a mythhelm command that records state migrates it",
			path, version, SchemaVersion)
	}
	return &Journal{db: db}, nil
}

// ListRuns returns up to limit run projections, newest first. Run IDs sort
// by creation time, so the order is by run_id.
func (j *Journal) ListRuns(ctx context.Context, limit int) ([]RunRow, error) {
	if limit < 1 {
		return nil, fmt.Errorf("journal: list limit must be at least 1, got %d", limit)
	}
	rows, err := j.db.QueryContext(ctx, `SELECT run_id, state, reason, adapter_id, source_repo, source_branch,
		base_rev, task_sha256, billing_posture, execution_profile, created_at, updated_at
		FROM runs ORDER BY run_id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("journal: listing runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []RunRow
	for rows.Next() {
		var r RunRow
		var reason, branch, baseRev sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&r.RunID, &r.State, &reason, &r.AdapterID, &r.SourceRepo, &branch, &baseRev,
			&r.TaskSHA256, &r.BillingPosture, &r.ExecutionProfile, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("journal: reading run: %w", err)
		}
		r.Reason, r.SourceBranch, r.BaseRev = reason.String, branch.String, baseRev.String
		if r.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, fmt.Errorf("journal: run %s created_at: %w", r.RunID, err)
		}
		if r.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
			return nil, fmt.Errorf("journal: run %s updated_at: %w", r.RunID, err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("journal: listing runs: %w", err)
	}
	return out, nil
}
