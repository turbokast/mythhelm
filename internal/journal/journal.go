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

	"github.com/turbokast/mythhelm/internal/v2contract"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

const (
	// DBName is the database file inside the state directory.
	DBName = "mythhelm.db"
	// SchemaVersion is the newest database schema (PRAGMA user_version)
	// this binary understands.
	SchemaVersion = 7
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
	db  *sql.DB
	dir string
}

// StateDir is the directory this journal was opened from.
func (j *Journal) StateDir() string { return j.dir }

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
	return &Journal{db: db, dir: dir}, nil
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
	_, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("inspecting %s: %w", path, err)
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

	// BEGIN IMMEDIATE holds the write lock from here to the commit, so
	// concurrent openers take turns and only the first one migrates.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	if err := checkVersion(path, version, len(migs)); err != nil {
		return err
	}
	if version == len(migs) {
		return nil
	}
	if version > 0 {
		if err := backup(ctx, db, path, version); err != nil {
			return err
		}
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

// backup writes <path>.bak-v<version> while the caller holds the write lock.
// VACUUM INTO cannot run inside a transaction, so it runs on another pooled
// connection as a WAL reader of the committed database. A backup left by an
// interrupted migration is reused only if it is an intact database at the
// same version; anything else at that path is never overwritten.
func backup(ctx context.Context, db *sql.DB, path string, version int) error {
	dst := fmt.Sprintf("%s.bak-v%d", path, version)
	_, err := os.Lstat(dst)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if _, err := db.ExecContext(ctx, "VACUUM INTO ?", dst); err != nil {
			// Under the write lock nothing else creates dst, so a file there now
			// is this call's partial output; leaving it would block every retry.
			_ = os.Remove(dst)
			return fmt.Errorf("backing up schema v%d to %s before migrating: %w", version, dst, err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("inspecting backup %s: %w", dst, err)
	}
	if err := checkBackup(ctx, dst, version); err != nil {
		return fmt.Errorf("%s already exists and is not a usable schema v%d backup (%w); move it aside and retry", dst, version, err)
	}
	return nil
}

func checkBackup(ctx context.Context, path string, version int) error {
	bak, err := sql.Open("sqlite", dsn(path, "mode=ro"))
	if err != nil {
		return err
	}
	defer func() { _ = bak.Close() }()
	var got int
	var check string
	if err := bak.QueryRowContext(ctx, "PRAGMA user_version").Scan(&got); err != nil {
		return err
	}
	if got != version {
		return fmt.Errorf("its schema version is %d", got)
	}
	if err := bak.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&check); err != nil {
		return err
	}
	if check != "ok" {
		return fmt.Errorf("quick_check: %s", check)
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
//
// Envelopes with schema_version == 2 append once the durable migration phase
// allows them (supervisor-migration design §7): migration-owned envelopes
// (migration.quarantined, migration.imported) at phase drained, every v2
// envelope at imported or later. v2 failures return an error that unwraps to
// both a *v2contract.ControlError carrying the catalogue code (errors.As)
// and the stream-1 cause (errors.Is); a v2 envelope refused by the phase
// rule also unwraps to ErrInvalidEvent, since the ledger cannot accept it.
func (j *Journal) Append(ctx context.Context, ev Event, project func(*sql.Tx) error) error {
	if ev.SchemaVersion != v2contract.SchemaVersion {
		if err := validate(ev); err != nil {
			return err
		}
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return txBoundaryError(ev, "starting append", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := appendTx(ctx, tx, ev, project); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return txBoundaryError(ev, "committing event "+ev.EventID, err)
	}
	return nil
}

// txBoundaryError maps Append's transaction-boundary failures: v2 envelopes
// get the catalogue's persistence_unavailable (the mutation did not
// persist), while v1 keeps its exact messages.
func txBoundaryError(ev Event, op string, err error) error {
	if ev.SchemaVersion == v2contract.SchemaVersion {
		return v2Error(v2contract.CodePersistenceUnavailable, ev,
			"retry the append against a healthy ledger",
			fmt.Errorf("%s: %w", op, err))
	}
	return fmt.Errorf("journal: %s: %w", op, err)
}

// appendTx is the transaction-scoped core of Append: checks, insert, producer
// advance and projection. Append calls it after BeginTx; the migration Import
// step calls it inside its Mutate transactions so v2 rows, the import marker
// and the envelope commit or roll back together (design §7). v1 envelopes
// arrive validated by Append, exactly as before; v2 envelopes validate
// inside, where the phase read needs the transaction.
func appendTx(ctx context.Context, tx *sql.Tx, ev Event, project func(*sql.Tx) error) error {
	if ev.SchemaVersion == v2contract.SchemaVersion {
		return appendV2(ctx, tx, ev, project)
	}

	var dup int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM journal WHERE event_id = ?`, ev.EventID).Scan(&dup)
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
	return storeTx(ctx, tx, ev, project)
}

// Migration-owned envelope types (supervisor-migration design §7, D4): the
// only v2 envelopes accepted at phase drained, persisted by the Import step
// from its DrainReport before ordinary v2 traffic begins.
const (
	migrationQuarantinedType = "migration.quarantined"
	migrationImportedType    = "migration.imported"
)

// v2Allowed is the one phase rule for v2 envelopes (design §7): nothing
// before drained, only migration-owned envelopes at drained, everything at
// imported or later. Unknown phases fail closed: v2 never stores silently.
// The phase words mirror the migration_state CHECK vocabulary and
// migrate.Phase as literals, so internal/journal never imports
// internal/migrate (which reaches back into journal for the import path).
func v2Allowed(phase, typ string) bool {
	switch phase {
	case "imported", "adopted":
		return true
	case "drained":
		return typ == migrationQuarantinedType || typ == migrationImportedType
	default:
		return false
	}
}

// migrationPhase reads the durable migration phase in tx. A database that
// predates migration_state (or lost its row) reads as not_started: v2 fails
// closed, and v1 is unaffected — the v1 path never calls this.
func migrationPhase(ctx context.Context, tx *sql.Tx) (string, error) {
	var phase string
	err := tx.QueryRowContext(ctx, `SELECT phase FROM migration_state WHERE id = 1`).Scan(&phase)
	switch {
	case err == nil:
		return phase, nil
	case errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no such table"):
		return "not_started", nil
	default:
		return "", err
	}
}

// v2Error maps a v2 append failure to a catalogue code at the Append boundary
// (design §7). The returned error unwraps to both the *v2contract.ControlError
// (errors.As, for the code) and the stream-1 cause (errors.Is, for the
// sentinel).
func v2Error(code v2contract.Code, ev Event, nextAction string, cause error) error {
	opID := ev.EventID
	if opID == "" {
		// The operation cannot be identified without an event_id; say so
		// (I09: unmeasured, never empty) so the ControlError stays valid.
		opID = "unknown"
	}
	ce := &v2contract.ControlError{
		Code:        code,
		Owner:       "journal",
		OperationID: opID,
		Disposition: code.DefaultDisposition(),
		NextAction:  nextAction,
	}
	return fmt.Errorf("%w: %w", ce, cause)
}

// appendV2 is appendTx for schema_version == 2 envelopes (design §7): the
// stream-1 envelope validation, the one phase rule, then the stream-1 pure
// validators in Append's check order — duplicate, generation, sequence —
// before the shared store tail. A duplicate event_id acks with nil error
// and no append.
func appendV2(ctx context.Context, tx *sql.Tx, ev Event, project func(*sql.Tx) error) error {
	venv := v2contract.Envelope{
		SchemaVersion: ev.SchemaVersion, EventID: ev.EventID, RunID: ev.RunID,
		TaskID: ev.TaskID, AttemptID: ev.AttemptID, ProducerID: ev.ProducerID,
		ProducerSequence: ev.ProducerSequence, RunSequence: ev.RunSequence,
		Generation: ev.Generation, CausedBy: ev.CausedBy, ObservedAt: ev.ObservedAt,
		Type: ev.Type, Payload: ev.Payload,
	}
	if err := venv.Validate(); err != nil {
		return v2Error(v2contract.CodeInvalidContract, ev, "fix the envelope and retry", err)
	}
	phase, err := migrationPhase(ctx, tx)
	if err != nil {
		return v2Error(v2contract.CodePersistenceUnavailable, ev,
			"retry the append against a healthy ledger",
			fmt.Errorf("reading migration phase: %w", err))
	}
	if !v2Allowed(phase, ev.Type) {
		// ErrInvalidEvent joins the chain: the ledger cannot accept this
		// envelope at this phase, so it is invalid here.
		return fmt.Errorf("%w: %w", v2Error(v2contract.CodeInvalidContract, ev,
			"append v2 envelopes only once migration reaches drained (migration-owned) or imported (ordinary)",
			fmt.Errorf("schema_version 2 %q is not accepted at migration phase %q", ev.Type, phase)),
			ErrInvalidEvent)
	}

	var one int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM journal WHERE event_id = ?`, ev.EventID).Scan(&one)
	var seen bool
	switch {
	case err == nil:
		seen = true
	case !errors.Is(err, sql.ErrNoRows):
		return v2Error(v2contract.CodePersistenceUnavailable, ev,
			"retry the append against a healthy ledger",
			fmt.Errorf("checking event_id: %w", err))
	}
	if err := v2contract.CheckDuplicate(seen); err != nil {
		return nil
	}

	var lastSeq, generation int64
	err = tx.QueryRowContext(ctx, `SELECT last_sequence, generation FROM producers WHERE producer_id = ?`,
		ev.ProducerID).Scan(&lastSeq, &generation)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		lastSeq, generation = 0, ev.Generation
	case err != nil:
		return v2Error(v2contract.CodePersistenceUnavailable, ev,
			"retry the append against a healthy ledger",
			fmt.Errorf("reading producer: %w", err))
	}
	if err := v2contract.CheckGeneration(generation, ev.Generation); err != nil {
		return v2Error(v2contract.CodeOwnershipUnresolved, ev,
			"retain the event as quarantined evidence at most; reconcile the producer generation", err)
	}
	if err := v2contract.CheckSequence(lastSeq, ev.ProducerSequence); err != nil {
		return v2Error(v2contract.CodeInvalidContract, ev,
			"resend with producer_sequence exactly one more than the last appended", err)
	}
	return storeTx(ctx, tx, ev, project)
}

// storeTx is the shared tail of the v1 and v2 append paths: run-sequence
// allocation, insert, producer advance and projection. v1 failures keep
// their exact messages; v2 ledger failures map to persistence_unavailable,
// since the mutation did not persist.
func storeTx(ctx context.Context, tx *sql.Tx, ev Event, project func(*sql.Tx) error) error {
	v2 := ev.SchemaVersion == v2contract.SchemaVersion
	fail := func(op string, err error) error {
		if v2 {
			return v2Error(v2contract.CodePersistenceUnavailable, ev,
				"retry the append against a healthy ledger",
				fmt.Errorf("%s: %w", op, err))
		}
		return fmt.Errorf("journal: %s: %w", op, err)
	}

	var runSeq int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(run_sequence), 0) + 1 FROM journal WHERE run_id = ?`,
		ev.RunID).Scan(&runSeq); err != nil {
		return fail("assigning run sequence", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO journal (event_id, schema_version, run_id, task_id, attempt_id,
		producer_id, producer_sequence, run_sequence, generation, caused_by, observed_at, type, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.EventID, ev.SchemaVersion, ev.RunID, nullable(ev.TaskID), nullable(ev.AttemptID),
		ev.ProducerID, ev.ProducerSequence, runSeq, ev.Generation, nullable(ev.CausedBy),
		ev.ObservedAt.UTC().Format(time.RFC3339Nano), ev.Type, string(ev.Payload)); err != nil {
		return fail("inserting event "+ev.EventID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO producers (producer_id, last_sequence, generation) VALUES (?, ?, ?)
		ON CONFLICT (producer_id) DO UPDATE SET last_sequence = excluded.last_sequence, generation = excluded.generation`,
		ev.ProducerID, ev.ProducerSequence, ev.Generation); err != nil {
		return fail("advancing producer "+ev.ProducerID, err)
	}
	if project != nil {
		if err := project(tx); err != nil {
			return fail("projecting event "+ev.EventID, err)
		}
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
	return j.queryEvents(ctx, `SELECT event_id, schema_version, run_id, task_id, attempt_id,
		producer_id, producer_sequence, run_sequence, generation, caused_by, observed_at, type, payload
		FROM journal WHERE run_id = ? AND run_sequence > ? ORDER BY run_sequence`, runID, afterRunSeq)
}

// EventsLimit is Events capped at limit rows, so paged readers never load
// the whole history to discover it overflows the page.
func (j *Journal) EventsLimit(ctx context.Context, runID string, afterRunSeq int64, limit int) ([]Event, error) {
	if limit < 1 {
		return nil, fmt.Errorf("journal: event limit must be at least 1, got %d", limit)
	}
	return j.queryEvents(ctx, `SELECT event_id, schema_version, run_id, task_id, attempt_id,
		producer_id, producer_sequence, run_sequence, generation, caused_by, observed_at, type, payload
		FROM journal WHERE run_id = ? AND run_sequence > ? ORDER BY run_sequence LIMIT ?`, runID, afterRunSeq, limit)
}

func (j *Journal) queryEvents(ctx context.Context, query string, args ...any) ([]Event, error) {
	rows, err := j.db.QueryContext(ctx, query, args...)
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
