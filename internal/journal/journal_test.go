package journal

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
)

func openTemp(t *testing.T) (*Journal, string) {
	t.Helper()
	dir := t.TempDir()
	j, err := Open(t.Context(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := j.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return j, dir
}

func event(runID, producer string, seq, gen int64) Event {
	return Event{
		SchemaVersion:    EnvelopeVersion,
		EventID:          ids.New("evt"),
		RunID:            runID,
		ProducerID:       producer,
		ProducerSequence: seq,
		Generation:       gen,
		ObservedAt:       time.Now(),
		Type:             "run.state_changed",
		Payload:          json.RawMessage(`{"state":"created","reason":null}`),
	}
}

func mustAppend(t *testing.T, j *Journal, ev Event) {
	t.Helper()
	if err := j.Append(t.Context(), ev, nil); err != nil {
		t.Fatalf("Append(%s seq %d): %v", ev.ProducerID, ev.ProducerSequence, err)
	}
}

func events(t *testing.T, j *Journal, runID string, after int64) []Event {
	t.Helper()
	evs, err := j.Events(t.Context(), runID, after)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	return evs
}

func rawOpen(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", dsn(path, "_pragma=busy_timeout(5000)"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func fileSHA256(t *testing.T, path string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

func TestPragmasApplied(t *testing.T) {
	j, _ := openTemp(t)
	// Pin one pooled connection so every PRAGMA reads the same one.
	conn, err := j.db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	for pragma, want := range map[string]string{
		"journal_mode": "wal",
		"foreign_keys": "1",
		"synchronous":  "2",
		"busy_timeout": "5000",
		"user_version": "1",
	} {
		var got string
		if err := conn.QueryRowContext(t.Context(), "PRAGMA "+pragma).Scan(&got); err != nil {
			t.Fatalf("PRAGMA %s: %v", pragma, err)
		}
		if got != want {
			t.Errorf("PRAGMA %s = %q, want %q", pragma, got, want)
		}
	}
}

func TestJournalRejectsUpdateAndDelete(t *testing.T) {
	j, dir := openTemp(t)
	mustAppend(t, j, event("run_a", "sup_a", 1, 1))

	other := rawOpen(t, filepath.Join(dir, DBName))
	for _, stmt := range []string{
		"UPDATE journal SET type = 'forged'",
		"DELETE FROM journal",
	} {
		_, err := other.ExecContext(t.Context(), stmt)
		if err == nil || !strings.Contains(err.Error(), "journal is append-only") {
			t.Errorf("%s: err = %v, want the append-only trigger to abort", stmt, err)
		}
	}
	evs := events(t, j, "run_a", 0)
	if len(evs) != 1 || evs[0].Type != "run.state_changed" {
		t.Fatalf("journal changed after rejected writes: %+v", evs)
	}
}

func TestAppendIsIdempotentOnEventID(t *testing.T) {
	j, _ := openTemp(t)
	ev := event("run_a", "sup_a", 1, 1)
	projections := 0
	project := func(*sql.Tx) error { projections++; return nil }

	for range 2 {
		if err := j.Append(t.Context(), ev, project); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if evs := events(t, j, "run_a", 0); len(evs) != 1 {
		t.Fatalf("got %d journal rows for one event_id, want 1", len(evs))
	}
	if projections != 1 {
		t.Fatalf("projection applied %d times, want 1", projections)
	}
	// The duplicate did not advance the producer: the next sequence is 2.
	mustAppend(t, j, event("run_a", "sup_a", 2, 1))
}

func TestAppendRejectsProducerSequenceGap(t *testing.T) {
	j, _ := openTemp(t)
	tests := []struct {
		name string
		seq  int64
	}{
		{name: "gap", seq: 4},
		{name: "regression", seq: 1},
		{name: "repeat of the last sequence", seq: 2},
		{name: "zero", seq: 0},
	}
	mustAppend(t, j, event("run_a", "sup_a", 1, 1))
	mustAppend(t, j, event("run_a", "sup_a", 2, 1))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projected := false
			err := j.Append(t.Context(), event("run_a", "sup_a", tt.seq, 1), func(*sql.Tx) error { projected = true; return nil })
			if !errors.Is(err, ErrSequenceGap) {
				t.Fatalf("Append(seq %d) err = %v, want ErrSequenceGap", tt.seq, err)
			}
			if projected {
				t.Fatal("projection ran for a rejected event")
			}
		})
	}
	if err := j.Append(t.Context(), event("run_b", "sup_new", 2, 1), nil); !errors.Is(err, ErrSequenceGap) {
		t.Fatalf("a new producer starting at 2: err = %v, want ErrSequenceGap", err)
	}
	if n := len(events(t, j, "run_a", 0)) + len(events(t, j, "run_b", 0)); n != 2 {
		t.Fatalf("got %d journal rows, want 2", n)
	}
	mustAppend(t, j, event("run_a", "sup_a", 3, 1))
}

func TestAppendRejectsStaleGeneration(t *testing.T) {
	j, _ := openTemp(t)
	mustAppend(t, j, event("run_a", "wrk_att_1", 1, 2))

	err := j.Append(t.Context(), event("run_a", "wrk_att_1", 2, 1), nil)
	if !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("Append from generation 1 after 2: err = %v, want ErrStaleGeneration", err)
	}
	mustAppend(t, j, event("run_a", "wrk_att_1", 2, 2))
	mustAppend(t, j, event("run_a", "wrk_att_1", 3, 3))
	if err := j.Append(t.Context(), event("run_a", "wrk_att_1", 4, 2), nil); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("Append from generation 2 after 3: err = %v, want ErrStaleGeneration", err)
	}
	evs := events(t, j, "run_a", 0)
	if len(evs) != 3 {
		t.Fatalf("got %d rows, want 3", len(evs))
	}
	if last := evs[len(evs)-1]; last.Generation != 3 || last.ProducerSequence != 3 {
		t.Fatalf("last event = generation %d sequence %d, want 3/3", last.Generation, last.ProducerSequence)
	}
}

func TestRunSequenceIsPerRunMonotonic(t *testing.T) {
	j, _ := openTemp(t)
	// Interleave two runs and three producers.
	mustAppend(t, j, event("run_a", "sup_a", 1, 1))
	mustAppend(t, j, event("run_b", "sup_b", 1, 1))
	mustAppend(t, j, event("run_a", "wrk_a", 1, 1))
	mustAppend(t, j, event("run_a", "sup_a", 2, 1))
	mustAppend(t, j, event("run_b", "sup_b", 2, 1))

	assertRunSequences(t, events(t, j, "run_a", 0), 1, 2, 3)
	assertRunSequences(t, events(t, j, "run_b", 0), 1, 2)
	assertRunSequences(t, events(t, j, "run_a", 1), 2, 3)
	assertRunSequences(t, events(t, j, "run_a", 3))

	// A caller-supplied run_sequence is ignored; ingest assigns it.
	ev := event("run_a", "sup_a", 3, 1)
	ev.RunSequence = 99
	mustAppend(t, j, ev)
	assertRunSequences(t, events(t, j, "run_a", 3), 4)
}

func assertRunSequences(t *testing.T, evs []Event, want ...int64) {
	t.Helper()
	got := make([]int64, len(evs))
	for i, ev := range evs {
		got[i] = ev.RunSequence
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("run sequences = %v, want %v", got, want)
	}
}

func TestConcurrentAppendsFromTwoHandles(t *testing.T) {
	// Two handles on one file stand in for two supervisor processes; SQLite's
	// immediate transactions and busy timeout serialise them.
	j1, dir := openTemp(t)
	j2, err := Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j2.Close() }()

	const perProducer = 25
	var wg sync.WaitGroup
	errs := make(chan error, 2*perProducer)
	for i, j := range []*Journal{j1, j2} {
		producer := fmt.Sprintf("sup_%d", i)
		wg.Go(func() {
			for seq := int64(1); seq <= perProducer; seq++ {
				if err := j.Append(t.Context(), event("run_a", producer, seq, 1), nil); err != nil {
					errs <- err
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent Append: %v", err)
	}
	want := make([]int64, 2*perProducer)
	for i := range want {
		want[i] = int64(i + 1)
	}
	assertRunSequences(t, events(t, j1, "run_a", 0), want...)
}

func TestAppendProjectionErrorRollsBack(t *testing.T) {
	j, _ := openTemp(t)
	boom := errors.New("projection failed")
	ev := event("run_a", "sup_a", 1, 1)
	err := j.Append(t.Context(), ev, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(t.Context(), `INSERT INTO producers VALUES ('marker', 1, 1)`); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Append err = %v, want the projection error", err)
	}
	if evs := events(t, j, "run_a", 0); len(evs) != 0 {
		t.Fatalf("journal row survived a failed projection: %+v", evs)
	}
	var n int
	if err := j.db.QueryRowContext(t.Context(), `SELECT count(*) FROM producers`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("producers has %d rows after rollback, want 0", n)
	}
	// Nothing was consumed: the same event can be appended again.
	mustAppend(t, j, ev)
}

func TestAppendProjectionSharesTransaction(t *testing.T) {
	j, _ := openTemp(t)
	ev := event("run_a", "sup_a", 1, 1)
	err := j.Append(t.Context(), ev, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(t.Context(), `SELECT count(*) FROM journal WHERE event_id = ?`, ev.EventID).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("projection sees %d rows for its event, want 1", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAppendValidatesEnvelope(t *testing.T) {
	j, _ := openTemp(t)
	tests := []struct {
		name   string
		mutate func(*Event)
	}{
		{name: "schema version", mutate: func(e *Event) { e.SchemaVersion = 2 }},
		{name: "event_id", mutate: func(e *Event) { e.EventID = "" }},
		{name: "run_id", mutate: func(e *Event) { e.RunID = "" }},
		{name: "producer_id", mutate: func(e *Event) { e.ProducerID = "" }},
		{name: "type", mutate: func(e *Event) { e.Type = "" }},
		{name: "observed_at", mutate: func(e *Event) { e.ObservedAt = time.Time{} }},
		{name: "negative generation", mutate: func(e *Event) { e.Generation = -1 }},
		{name: "missing payload", mutate: func(e *Event) { e.Payload = nil }},
		{name: "invalid payload", mutate: func(e *Event) { e.Payload = json.RawMessage(`{"state":`) }},
		{name: "unlabelled payload", mutate: func(e *Event) { e.Payload = json.RawMessage(`0.42`) }},
		{name: "array payload", mutate: func(e *Event) { e.Payload = json.RawMessage(`["created"]`) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := event("run_a", "sup_a", 1, 1)
			tt.mutate(&ev)
			if err := j.Append(t.Context(), ev, nil); !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("err = %v, want ErrInvalidEvent", err)
			}
		})
	}
	if evs := events(t, j, "run_a", 0); len(evs) != 0 {
		t.Fatalf("invalid events were stored: %+v", evs)
	}
}

func TestEventsRoundTrip(t *testing.T) {
	j, _ := openTemp(t)
	observed := time.Date(2026, 9, 29, 12, 0, 0, 123456789, time.FixedZone("CEST", 2*3600))
	full := Event{
		SchemaVersion:    EnvelopeVersion,
		EventID:          "evt_full",
		RunID:            "run_a",
		TaskID:           "task_1",
		AttemptID:        "att_1",
		ProducerID:       "wrk_att_1",
		ProducerSequence: 1,
		Generation:       2,
		CausedBy:         "evt_cause",
		ObservedAt:       observed,
		Type:             "attempt.native_result",
		// Decimal strings and explicit nulls survive byte for byte (I09).
		Payload: json.RawMessage(`{"retail_equivalent_estimate_usd":"0.4200","num_turns":null}`),
	}
	bare := event("run_a", "sup_a", 1, 1)
	mustAppend(t, j, full)
	mustAppend(t, j, bare)

	evs := events(t, j, "run_a", 0)
	if len(evs) != 2 {
		t.Fatalf("got %d events, want 2", len(evs))
	}
	want := full
	want.RunSequence = 1
	want.ObservedAt = observed.UTC()
	if got := evs[0]; !got.ObservedAt.Equal(want.ObservedAt) || got.ObservedAt.Location() != time.UTC {
		t.Fatalf("observed_at = %v, want %v in UTC", got.ObservedAt, want.ObservedAt)
	}
	gotJSON, _ := json.Marshal(evs[0])
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("round trip:\n got %s\nwant %s", gotJSON, wantJSON)
	}
	if evs[1].TaskID != "" || evs[1].AttemptID != "" || evs[1].CausedBy != "" {
		t.Fatalf("optional fields not empty: %+v", evs[1])
	}
	var nulls int
	if err := j.db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM journal WHERE task_id IS NULL AND attempt_id IS NULL AND caused_by IS NULL`).Scan(&nulls); err != nil {
		t.Fatal(err)
	}
	if nulls != 1 {
		t.Fatalf("absent optional fields stored as NULL in %d rows, want 1", nulls)
	}
}

func TestOpenRefusesNewerSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DBName)
	db := rawOpen(t, path)
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE future (x INTEGER); PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	before := fileSHA256(t, path)

	j, err := Open(t.Context(), dir)
	if err == nil {
		_ = j.Close()
	}
	if !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("Open err = %v, want ErrSchemaTooNew", err)
	}
	for _, want := range []string{"99", "restore"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if after := fileSHA256(t, path); after != before {
		t.Fatal("Open changed the database file")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("state dir holds %v, want only %s", names, DBName)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	for range 2 {
		j, err := Open(t.Context(), dir)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		var version int
		if err := j.db.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version); err != nil {
			t.Fatal(err)
		}
		if version != SchemaVersion {
			t.Fatalf("user_version = %d, want %d", version, SchemaVersion)
		}
		if err := j.Close(); err != nil {
			t.Fatal(err)
		}
	}
	matches, err := filepath.Glob(filepath.Join(dir, DBName+".bak-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("reopening at the current version made backups %v", matches)
	}
}

func TestMigrationBacksUpBeforeUpgrade(t *testing.T) {
	ctx := t.Context()
	dir, v2 := v1Database(t)
	j, err := open(ctx, dir, v2)
	if err != nil {
		t.Fatalf("migrating to v2: %v", err)
	}
	defer func() { _ = j.Close() }()
	var version int
	if err := j.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Fatalf("user_version = %d after migration, want 2", version)
	}

	backup := rawOpen(t, filepath.Join(dir, DBName+".bak-v1"))
	var backupVersion, rows int
	if err := backup.QueryRowContext(ctx, "PRAGMA user_version").Scan(&backupVersion); err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if err := backup.QueryRowContext(ctx, "SELECT count(*) FROM journal").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if backupVersion != 1 || rows != 1 {
		t.Fatalf("backup has user_version %d and %d journal rows, want 1 and 1", backupVersion, rows)
	}
}

// v1Database creates a schema v1 database with one journal row in a new
// directory and returns the directory and the two-step migration list.
func v1Database(t *testing.T) (dir string, v2 []string) {
	t.Helper()
	dir = t.TempDir()
	v1, err := open(t.Context(), dir, migrations[:1])
	if err != nil {
		t.Fatal(err)
	}
	mustAppend(t, v1, event("run_a", "sup_a", 1, 1))
	if err := v1.Close(); err != nil {
		t.Fatal(err)
	}
	return dir, append(migrations[:1:1], `CREATE TABLE v2_marker (x INTEGER) STRICT;`)
}

func TestConcurrentOpensMigrateOnce(t *testing.T) {
	dir, v2 := v1Database(t)
	const openers = 4
	var wg sync.WaitGroup
	errs := make(chan error, openers)
	for range openers {
		wg.Go(func() {
			j, err := open(t.Context(), dir, v2)
			if err != nil {
				errs <- err
				return
			}
			errs <- j.Close()
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Open during migration: %v", err)
		}
	}
	backup := rawOpen(t, filepath.Join(dir, DBName+".bak-v1"))
	var version int
	if err := backup.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Fatalf("backup user_version = %d, want 1", version)
	}
}

func TestMigrationReusesCompleteBackup(t *testing.T) {
	// A crash after the backup but before the migration commit leaves a
	// complete backup behind; the retry must not be stuck on it.
	dir, v2 := v1Database(t)
	path := filepath.Join(dir, DBName)
	db := rawOpen(t, path)
	if _, err := db.ExecContext(t.Context(), "VACUUM INTO ?", path+".bak-v1"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	j, err := open(t.Context(), dir, v2)
	if err != nil {
		t.Fatalf("Open with a complete earlier backup: %v", err)
	}
	_ = j.Close()
}

func TestMigrationRefusesUnusableBackup(t *testing.T) {
	dir, v2 := v1Database(t)
	backup := filepath.Join(dir, DBName+".bak-v1")
	if err := os.WriteFile(backup, []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	j, err := open(t.Context(), dir, v2)
	if err == nil {
		_ = j.Close()
		t.Fatal("Open migrated although the backup path holds something that is not a v1 backup")
	}
	if !strings.Contains(err.Error(), backup) {
		t.Fatalf("error %q does not name the backup path", err)
	}
	if got, err := os.ReadFile(backup); err != nil || string(got) != "not a database" {
		t.Fatalf("existing file at the backup path was changed: %q, %v", got, err)
	}
	var version int
	if err := rawOpen(t, filepath.Join(dir, DBName)).QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Fatalf("user_version = %d after a refused migration, want 1", version)
	}
}

func TestOpenPathWithSpacesAndUnicode(t *testing.T) {
	name := "state dir ünïcödé"
	if runtime.GOOS != "windows" {
		name += " ?#%20&="
	}
	dir := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	j, err := Open(t.Context(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	mustAppend(t, j, event("run_a", "sup_a", 1, 1))
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, DBName)); err != nil {
		t.Fatalf("database not created at the exact path: %v", err)
	}
}

func TestOpenMissingDirFails(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	j, err := Open(t.Context(), missing)
	if err == nil {
		_ = j.Close()
		t.Fatal("Open on a missing directory succeeded; the caller must create it with statedir.Ensure")
	}
	if _, statErr := os.Stat(missing); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Open created %s", missing)
	}
}

func TestBuildWithoutCgo(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the module; skipped with -short")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go command not on PATH")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, goBin, "build", "./...")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("CGO_ENABLED=0 go build ./...: %v\n%s", err, out)
	}
}
