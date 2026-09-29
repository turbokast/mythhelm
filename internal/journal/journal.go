// Package journal is MYTHHELM's durable local state (design §5, ADR 0003):
// a SQLite database, opened through the pure-Go modernc.org/sqlite driver,
// holding an append-only event journal and the projections derived from it.
package journal

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

const (
	// DBName is the database file inside the state directory.
	DBName = "mythhelm.db"
	// SchemaVersion is the newest database schema (PRAGMA user_version)
	// this binary understands.
	SchemaVersion = 1
	// EnvelopeVersion is the only event envelope schema_version accepted.
	EnvelopeVersion = 1
)

// pragmas are applied by the driver to every pooled connection. _txlock
// makes each transaction BEGIN IMMEDIATE, so writers queue on busy_timeout
// instead of failing on a read-to-write lock upgrade.
const pragmas = "_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_txlock=immediate"

var (
	// ErrSchemaTooNew reports a database written by a newer MYTHHELM. It maps
	// to exit 2 (AC-9.4); the database is left untouched.
	ErrSchemaTooNew = errors.New("database schema is newer than this binary supports")
	// ErrSequenceGap reports a producer_sequence that is not exactly one more
	// than the producer's last appended sequence.
	ErrSequenceGap = errors.New("producer sequence gap or regression")
	// ErrStaleGeneration reports an event from a producer generation older
	// than one already journaled.
	ErrStaleGeneration = errors.New("stale producer generation")
	// ErrInvalidEvent reports an envelope with a missing or malformed field.
	ErrInvalidEvent = errors.New("invalid event")
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migrations[i] upgrades the schema from version i to i+1.
var migrations = loadMigrations()

func loadMigrations() []string {
	names, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		panic(err)
	}
	slices.Sort(names)
	out := make([]string, len(names))
	for i, name := range names {
		b, err := migrationFS.ReadFile(name)
		if err != nil {
			panic(err)
		}
		out[i] = string(b)
	}
	return out
}

// Event is the journal envelope (§7.6 subset). TaskID, AttemptID and
// CausedBy are optional; every other field is required. RunSequence is
// assigned by Append and ignored on input.
type Event struct {
	SchemaVersion    int             `json:"schema_version"`
	EventID          string          `json:"event_id"`
	RunID            string          `json:"run_id"`
	TaskID           string          `json:"task_id,omitempty"`
	AttemptID        string          `json:"attempt_id,omitempty"`
	ProducerID       string          `json:"producer_id"`
	ProducerSequence int64           `json:"producer_sequence"`
	RunSequence      int64           `json:"run_sequence"`
	Generation       int64           `json:"generation"`
	CausedBy         string          `json:"caused_by,omitempty"`
	ObservedAt       time.Time       `json:"observed_at"`
	Type             string          `json:"type"`
	Payload          json.RawMessage `json:"payload"`
}

// Journal is an open state database.
type Journal struct {
	db *sql.DB
}

// Open opens dir/mythhelm.db, creating it if needed, and migrates it to
// SchemaVersion. dir must already exist (see statedir.Ensure). A database
// newer than SchemaVersion yields ErrSchemaTooNew without writing anything.
func Open(ctx context.Context, dir string) (*Journal, error) {
	return open(ctx, dir, migrations)
}

func open(ctx context.Context, dir string, migs []string) (*Journal, error) {
	path := filepath.Join(dir, DBName)
	// Probe read-only first: the WAL pragma alone would rewrite the header
	// of a rollback-journal database this binary must not touch.
	if err := probeVersion(ctx, path, len(migs)); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn(path, pragmas))
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	if err := migrate(ctx, db, path, migs); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Journal{db: db}, nil
}

// Close closes the database.
func (j *Journal) Close() error {
	return j.db.Close()
}

// dsn builds a SQLite URI for path. Percent-encoding keeps spaces, '?', '#'
// and '%' in the path from being read as URI syntax; the leading slash turns
// a Windows drive path into file:///C:/....
func dsn(path, query string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p, RawQuery: query}).String()
}

func probeVersion(ctx context.Context, path string, supported int) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	db, err := sql.Open("sqlite", dsn(path, "mode=ro&_pragma=busy_timeout(5000)"))
	if err != nil {
		return fmt.Errorf("opening %s read-only: %w", path, err)
	}
	defer func() { _ = db.Close() }()
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("reading schema version of %s: %w", path, err)
	}
	return checkVersion(path, version, supported)
}

func checkVersion(path string, version, supported int) error {
	if version > supported {
		return fmt.Errorf("%w: %s has schema version %d, this binary supports up to %d; "+
			"upgrade mythhelm, or restore a backup written by this version (%s.bak-v<N>)",
			ErrSchemaTooNew, path, version, supported, path)
	}
	return nil
}

// migrate applies migs[current:] in one transaction. Before upgrading a
// database from a non-zero version it writes a copy to <path>.bak-v<N>
// (design §5 migration policy).
func migrate(ctx context.Context, db *sql.DB, path string, migs []string) error {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	if err := checkVersion(path, version, len(migs)); err != nil {
		return err
	}
	if version == len(migs) {
		return nil
	}
	if version > 0 {
		backup := fmt.Sprintf("%s.bak-v%d", path, version)
		// VACUUM INTO refuses to overwrite a non-empty file, so an earlier
		// backup is never clobbered.
		if _, err := db.ExecContext(ctx, "VACUUM INTO ?", backup); err != nil {
			return fmt.Errorf("backing up schema v%d to %s before migrating: %w", version, backup, err)
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Another process may have migrated since the check above.
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	if err := checkVersion(path, version, len(migs)); err != nil {
		return err
	}
	for v := version; v < len(migs); v++ {
		if _, err := tx.ExecContext(ctx, migs[v]); err != nil {
			return fmt.Errorf("applying migration %d: %w", v+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", v+1)); err != nil {
			return fmt.Errorf("recording schema version %d: %w", v+1, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration: %w", err)
	}
	return nil
}

// Append journals ev and applies project, the matching projection update, in
// one transaction (AC-9.2, AC-9.3). An event whose event_id is already
// journaled is ignored: Append returns nil and project does not run. It
// rejects a producer_sequence other than the producer's last sequence plus
// one (ErrSequenceGap) and a generation older than the producer's newest
// (ErrStaleGeneration). project may be nil; if it returns an error, nothing
// is written.
func (j *Journal) Append(ctx context.Context, ev Event, project func(*sql.Tx) error) error {
	if err := validate(ev); err != nil {
		return err
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("journal: starting append: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var dup int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM journal WHERE event_id = ?`, ev.EventID).Scan(&dup)
	switch {
	case err == nil:
		return nil
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("journal: checking event_id: %w", err)
	}

	var lastSeq, generation int64
	err = tx.QueryRowContext(ctx, `SELECT last_sequence, generation FROM producers WHERE producer_id = ?`,
		ev.ProducerID).Scan(&lastSeq, &generation)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		lastSeq, generation = 0, ev.Generation
	case err != nil:
		return fmt.Errorf("journal: reading producer: %w", err)
	}
	if ev.Generation < generation {
		return fmt.Errorf("%w: producer %s is at generation %d, event %s has %d",
			ErrStaleGeneration, ev.ProducerID, generation, ev.EventID, ev.Generation)
	}
	if ev.ProducerSequence != lastSeq+1 {
		return fmt.Errorf("%w: producer %s expected sequence %d, event %s has %d",
			ErrSequenceGap, ev.ProducerID, lastSeq+1, ev.EventID, ev.ProducerSequence)
	}

	var runSeq int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(run_sequence), 0) + 1 FROM journal WHERE run_id = ?`,
		ev.RunID).Scan(&runSeq); err != nil {
		return fmt.Errorf("journal: assigning run sequence: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO journal (event_id, schema_version, run_id, task_id, attempt_id,
		producer_id, producer_sequence, run_sequence, generation, caused_by, observed_at, type, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.EventID, ev.SchemaVersion, ev.RunID, nullable(ev.TaskID), nullable(ev.AttemptID),
		ev.ProducerID, ev.ProducerSequence, runSeq, ev.Generation, nullable(ev.CausedBy),
		ev.ObservedAt.UTC().Format(time.RFC3339Nano), ev.Type, string(ev.Payload)); err != nil {
		return fmt.Errorf("journal: inserting event %s: %w", ev.EventID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO producers (producer_id, last_sequence, generation) VALUES (?, ?, ?)
		ON CONFLICT (producer_id) DO UPDATE SET last_sequence = excluded.last_sequence, generation = excluded.generation`,
		ev.ProducerID, ev.ProducerSequence, ev.Generation); err != nil {
		return fmt.Errorf("journal: advancing producer %s: %w", ev.ProducerID, err)
	}
	if project != nil {
		if err := project(tx); err != nil {
			return fmt.Errorf("journal: projecting event %s: %w", ev.EventID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("journal: committing event %s: %w", ev.EventID, err)
	}
	return nil
}

func validate(ev Event) error {
	var missing []string
	for _, f := range []struct {
		name  string
		empty bool
	}{
		{"event_id", ev.EventID == ""},
		{"run_id", ev.RunID == ""},
		{"producer_id", ev.ProducerID == ""},
		{"type", ev.Type == ""},
		{"observed_at", ev.ObservedAt.IsZero()},
		{"payload", len(ev.Payload) == 0},
	} {
		if f.empty {
			missing = append(missing, f.name)
		}
	}
	switch {
	case len(missing) > 0:
		return fmt.Errorf("%w: missing %s", ErrInvalidEvent, strings.Join(missing, ", "))
	case ev.SchemaVersion != EnvelopeVersion:
		return fmt.Errorf("%w: schema_version %d, want %d", ErrInvalidEvent, ev.SchemaVersion, EnvelopeVersion)
	case ev.Generation < 0:
		return fmt.Errorf("%w: negative generation %d", ErrInvalidEvent, ev.Generation)
	case !json.Valid(ev.Payload):
		return fmt.Errorf("%w: payload is not valid JSON", ErrInvalidEvent)
	case !bytes.HasPrefix(bytes.TrimSpace(ev.Payload), []byte("{")):
		// Payload values are always named fields, so a bare number can never
		// be mistaken for a measurement of unknown kind (I09).
		return fmt.Errorf("%w: payload must be a JSON object", ErrInvalidEvent)
	}
	return nil
}

func nullable(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// Events returns runID's events with run_sequence greater than afterRunSeq,
// in run_sequence order.
func (j *Journal) Events(ctx context.Context, runID string, afterRunSeq int64) ([]Event, error) {
	rows, err := j.db.QueryContext(ctx, `SELECT event_id, schema_version, run_id, task_id, attempt_id,
		producer_id, producer_sequence, run_sequence, generation, caused_by, observed_at, type, payload
		FROM journal WHERE run_id = ? AND run_sequence > ? ORDER BY run_sequence`, runID, afterRunSeq)
	if err != nil {
		return nil, fmt.Errorf("journal: querying events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Event
	for rows.Next() {
		var ev Event
		var taskID, attemptID, causedBy sql.NullString
		var observedAt, payload string
		if err := rows.Scan(&ev.EventID, &ev.SchemaVersion, &ev.RunID, &taskID, &attemptID,
			&ev.ProducerID, &ev.ProducerSequence, &ev.RunSequence, &ev.Generation, &causedBy,
			&observedAt, &ev.Type, &payload); err != nil {
			return nil, fmt.Errorf("journal: reading event: %w", err)
		}
		ev.TaskID, ev.AttemptID, ev.CausedBy = taskID.String, attemptID.String, causedBy.String
		if ev.ObservedAt, err = time.Parse(time.RFC3339Nano, observedAt); err != nil {
			return nil, fmt.Errorf("journal: event %s observed_at: %w", ev.EventID, err)
		}
		ev.Payload = json.RawMessage(payload)
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("journal: reading events: %w", err)
	}
	return out, nil
}
