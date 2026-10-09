package journal

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Hashes of the released migrations; a released migration is never edited.
const (
	sha0001 = "262e63e2c7630dc637a916d476a1862db361affcb734dcbe2317f6770cb72e36"
	sha0002 = "52ce1d7a33fe4ace12e06ac3eae4032d9f11d647199113ef287416ab0202da7e"
)

var ledgerTables = []string{"usage_observations", "run_envelopes", "reservations", "bucket_state"}

func migrationSHA(t *testing.T, name string) string {
	t.Helper()
	b, err := migrationFS.ReadFile("migrations/" + name)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func schemaObjects(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `SELECT name, COALESCE(sql, '') FROM sqlite_master`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var name, ddl string
		if err := rows.Scan(&name, &ddl); err != nil {
			t.Fatal(err)
		}
		out[name] = ddl
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMigration0003CreatesLedgerTables(t *testing.T) {
	dir := t.TempDir()
	v2, err := open(t.Context(), dir, migrations[:2])
	if err != nil {
		t.Fatal(err)
	}
	before := schemaObjects(t, v2.db)
	if err := v2.Close(); err != nil {
		t.Fatal(err)
	}
	for _, table := range ledgerTables {
		if _, ok := before[table]; ok {
			t.Fatalf("table %s exists before migration 0003", table)
		}
	}

	j, err := Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	after := schemaObjects(t, j.db)
	for _, table := range ledgerTables {
		if after[table] == "" {
			t.Errorf("table %s missing after migration 0003", table)
		}
	}
	for name, ddl := range before {
		if after[name] != ddl {
			t.Errorf("DDL of %s changed by migration 0003:\nbefore %q\nafter  %q", name, ddl, after[name])
		}
	}
	if got := migrationSHA(t, "0002_qualification.sql"); got != sha0002 {
		t.Errorf("0002_qualification.sql sha256 = %s, want %s", got, sha0002)
	}
}

func TestMigration0001Untouched(t *testing.T) {
	if got := migrationSHA(t, "0001_init.sql"); got != sha0001 {
		t.Fatalf("0001_init.sql sha256 = %s, want %s: a released migration is never edited", got, sha0001)
	}
}

func TestSchemaVersionIs3(t *testing.T) {
	j, _ := openTemp(t)
	var version int
	if err := j.db.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if SchemaVersion < 3 || version != SchemaVersion {
		t.Fatalf("SchemaVersion = %d, user_version = %d, want at least 3 and equal", SchemaVersion, version)
	}
}

func seedRun(t *testing.T, j *Journal, runID string) {
	t.Helper()
	now := time.Now()
	err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return InsertRun(t.Context(), tx, RunRow{RunID: runID, State: "created", AdapterID: "builtin/fake", SourceRepo: "/tmp/repo",
			TaskSHA256: "t", BillingPosture: "local-scripted", ExecutionProfile: "trusted-host", CreatedAt: now, UpdatedAt: now})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func transact(t *testing.T, j *Journal, fn func(context.Context, *sql.Tx) error) error {
	t.Helper()
	return j.Transact(t.Context(), func(tx *sql.Tx) error { return fn(t.Context(), tx) })
}

func TestUsageObservationRoundTripAndNullProducer(t *testing.T) {
	j, _ := openTemp(t)
	seedRun(t, j, "run_1")
	rows := []UsageRow{
		{ObservationID: "obs_1", RunID: "run_1", Scope: "m", Unit: "tokens", Source: "native-reported", Label: "reported", Quantity: "15",
			ProducerID: "wrk_a", ProducerSequence: 4, ObservedAt: "2026-01-01T00:00:00Z"},
		{ObservationID: "obs_2", RunID: "run_1", Scope: "m", Unit: "tokens", Source: "native-reported", Label: "estimated", Quantity: "3",
			ObservedAt: "2026-01-01T00:00:01Z"},
	}
	for _, r := range rows {
		if err := transact(t, j, func(ctx context.Context, tx *sql.Tx) error { return InsertUsageObservation(ctx, tx, r) }); err != nil {
			t.Fatal(err)
		}
	}
	got, err := j.UsageObservations(t.Context(), "run_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != rows[0] || got[1] != rows[1] {
		t.Fatalf("round trip = %+v, want %+v", got, rows)
	}
	var producer sql.NullString
	var seq sql.NullInt64
	if err := j.db.QueryRowContext(t.Context(), `SELECT producer_id, producer_sequence FROM usage_observations WHERE observation_id='obs_2'`).Scan(&producer, &seq); err != nil {
		t.Fatal(err)
	}
	if producer.Valid || seq.Valid {
		t.Fatalf("identity-less row stored producer %v sequence %v, want NULL", producer, seq)
	}
}

func TestUnknownLabelRejectedNeverDefaulted(t *testing.T) { // I09 (v2 §2)
	j, _ := openTemp(t)
	seedRun(t, j, "run_1")
	for _, label := range []string{"", "measured", "Reported"} {
		err := transact(t, j, func(ctx context.Context, tx *sql.Tx) error {
			return InsertUsageObservation(ctx, tx, UsageRow{ObservationID: "obs_" + label, RunID: "run_1", Scope: "m", Unit: "tokens",
				Source: "s", Label: label, Quantity: "1", ObservedAt: "2026-01-01T00:00:00Z"})
		})
		if err == nil || !strings.Contains(err.Error(), "usage observation label") {
			t.Errorf("label %q: err = %v, want the accessor's label error", label, err)
		}
	}
	rows, err := j.UsageObservations(t.Context(), "run_1")
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows after rejected labels = %v, %v; want none", rows, err)
	}
	// The CHECK is the second line of defence when the accessor is bypassed.
	_, err = j.db.ExecContext(t.Context(), `INSERT INTO usage_observations (observation_id, run_id, scope, unit, source, label, quantity, observed_at)
		VALUES ('x','run_1','m','u','s','measured','1','t')`)
	if err == nil {
		t.Fatal("the table accepted an out-of-scale label")
	}
}

func TestProducerSequenceWithoutProducerRejected(t *testing.T) { // I09 (v2 §2)
	j, _ := openTemp(t)
	seedRun(t, j, "run_1")
	err := transact(t, j, func(ctx context.Context, tx *sql.Tx) error {
		return InsertUsageObservation(ctx, tx, UsageRow{ObservationID: "obs_1", RunID: "run_1", Scope: "m", Unit: "tokens",
			Source: "s", Label: "reported", Quantity: "10", ProducerSequence: 4, ObservedAt: "2026-01-01T00:00:00Z"})
	})
	if err == nil || !strings.Contains(err.Error(), "a producer sequence but no producer") {
		t.Errorf("usage row with a sequence but no producer: err = %v, want the accessor's producer error", err)
	}
	// Control: a fully identity-less row (empty producer, zero sequence)
	// still persists with NULL producer, never erroring.
	err = transact(t, j, func(ctx context.Context, tx *sql.Tx) error {
		return InsertUsageObservation(ctx, tx, UsageRow{ObservationID: "obs_2", RunID: "run_1", Scope: "m", Unit: "tokens",
			Source: "s", Label: "unknown", Quantity: "unknown", ObservedAt: "2026-01-01T00:00:00Z"})
	})
	if err != nil {
		t.Errorf("identity-less usage row: err = %v, want nil", err)
	}
}

func TestEmptyQuantityRejected(t *testing.T) { // I09 (v2 §2)
	j, _ := openTemp(t)
	seedRun(t, j, "run_1")
	err := transact(t, j, func(ctx context.Context, tx *sql.Tx) error {
		return InsertUsageObservation(ctx, tx, UsageRow{ObservationID: "obs_1", RunID: "run_1", Scope: "m", Unit: "tokens",
			Source: "s", Label: "unknown", Quantity: "", ObservedAt: "2026-01-01T00:00:00Z"})
	})
	if err == nil || !strings.Contains(err.Error(), "usage observation quantity is empty") {
		t.Errorf("usage row with an empty quantity: err = %v, want the accessor's quantity error", err)
	}
	err = transact(t, j, func(ctx context.Context, tx *sql.Tx) error {
		return InsertReservation(ctx, tx, ReservationRow{ReservationID: "res_1", RunID: "run_1", Bucket: "b", Scope: "s", Owner: "o",
			Quantity: "", Status: "held", ExpiresAt: "2026-01-02T00:00:00Z", CreatedAt: "2026-01-01T00:00:00Z"})
	})
	if err == nil || !strings.Contains(err.Error(), "reservation quantity is empty") {
		t.Errorf("reservation with an empty quantity: err = %v, want the accessor's quantity error", err)
	}
	for _, ddl := range []string{
		`INSERT INTO usage_observations (observation_id, run_id, scope, unit, source, label, quantity, observed_at) VALUES ('x','run_1','m','u','s','unknown','','t')`,
		`INSERT INTO reservations (reservation_id, run_id, bucket, scope, owner, quantity, status, expires_at, created_at) VALUES ('x','run_1','b','s','o','','held','t','t')`,
	} {
		if _, err := j.db.ExecContext(t.Context(), ddl); err == nil {
			t.Errorf("the table accepted an empty quantity: %s", ddl)
		}
	}
}

func TestBucketStateMissingIsNoRows(t *testing.T) {
	j, _ := openTemp(t)
	if _, err := j.BucketState(t.Context(), "never-exhausted"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("BucketState = %v, want sql.ErrNoRows", err)
	}
}

func TestBucketLifecycle(t *testing.T) {
	j, _ := openTemp(t)
	reset := "2026-01-02T00:00:00Z"
	steps := []func(context.Context, *sql.Tx) error{
		func(ctx context.Context, tx *sql.Tx) error {
			return SetBucketExhausted(ctx, tx, "b", "2026-01-01T00:00:00Z", nil)
		},
		func(ctx context.Context, tx *sql.Tx) error { return NoteBucketRetry(ctx, tx, "b") },
		func(ctx context.Context, tx *sql.Tx) error { return NoteBucketRetry(ctx, tx, "b") },
	}
	for _, s := range steps {
		if err := transact(t, j, s); err != nil {
			t.Fatal(err)
		}
	}
	b, err := j.BucketState(t.Context(), "b")
	if err != nil {
		t.Fatal(err)
	}
	if b.ResetAt != nil || b.RetriesUsed != 2 {
		t.Fatalf("bucket = %+v, want unknown reset and 2 retries", b)
	}
	if err := transact(t, j, func(ctx context.Context, tx *sql.Tx) error {
		return SetBucketExhausted(ctx, tx, "b", "2026-01-01T01:00:00Z", &reset)
	}); err != nil {
		t.Fatal(err)
	}
	b, _ = j.BucketState(t.Context(), "b")
	if b.ResetAt == nil || *b.ResetAt != reset || b.RetriesUsed != 2 {
		t.Fatalf("re-exhausted bucket = %+v, want reset %s and the 2 retries kept", b, reset)
	}
	if err := transact(t, j, func(ctx context.Context, tx *sql.Tx) error { return ClearBucket(ctx, tx, "b") }); err != nil {
		t.Fatal(err)
	}
	if _, err := j.BucketState(t.Context(), "b"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cleared bucket = %v, want sql.ErrNoRows", err)
	}
	if err := transact(t, j, func(ctx context.Context, tx *sql.Tx) error { return NoteBucketRetry(ctx, tx, "b") }); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("retry on a cleared bucket = %v, want sql.ErrNoRows", err)
	}
}

func TestReservationTransitions(t *testing.T) {
	j, _ := openTemp(t)
	seedRun(t, j, "run_1")
	r := ReservationRow{ReservationID: "res_1", RunID: "run_1", Bucket: "b", Scope: "s", Owner: "o", Quantity: "unknown",
		Status: "held", ExpiresAt: "2026-01-02T00:00:00Z", CreatedAt: "2026-01-01T00:00:00Z"}
	if err := transact(t, j, func(ctx context.Context, tx *sql.Tx) error { return InsertReservation(ctx, tx, r) }); err != nil {
		t.Fatal(err)
	}
	if err := transact(t, j, func(ctx context.Context, tx *sql.Tx) error { return InsertReservation(ctx, tx, r) }); err == nil {
		t.Fatal("duplicate reservation id accepted")
	}
	hb := time.Date(2026, 1, 1, 0, 5, 0, 0, time.UTC)
	if err := transact(t, j, func(ctx context.Context, tx *sql.Tx) error { return TouchReservation(ctx, tx, "res_1", hb) }); err != nil {
		t.Fatal(err)
	}
	got, err := j.Reservation(t.Context(), "res_1")
	if err != nil || got.HeartbeatAt != formatTime(hb) || got.Status != "held" {
		t.Fatalf("after touch = %+v, %v", got, err)
	}
	if err := transact(t, j, func(ctx context.Context, tx *sql.Tx) error { return MarkReservationOrphaned(ctx, tx, "res_1") }); err != nil {
		t.Fatal(err)
	}
	if err := transact(t, j, func(ctx context.Context, tx *sql.Tx) error { return TouchReservation(ctx, tx, "res_1", hb) }); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("touching an orphaned reservation = %v, want it refused", err)
	}
	rel := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	if err := transact(t, j, func(ctx context.Context, tx *sql.Tx) error {
		return SetReservationReleased(ctx, tx, "res_1", "run concluded", rel)
	}); err != nil {
		t.Fatal(err)
	}
	got, _ = j.Reservation(t.Context(), "res_1")
	if got.Status != "released" || got.ReleaseEvidence != "run concluded" {
		t.Fatalf("released = %+v", got)
	}
	if err := transact(t, j, func(ctx context.Context, tx *sql.Tx) error {
		return SetReservationReleased(ctx, tx, "res_1", "again", rel)
	}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("second release = %v, want it refused", err)
	}
	if _, err := j.Reservation(t.Context(), "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing reservation = %v, want sql.ErrNoRows", err)
	}
}

func TestRunEnvelopeUpsert(t *testing.T) {
	j, _ := openTemp(t)
	seedRun(t, j, "run_1")
	if _, err := j.RunEnvelope(t.Context(), "run_1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing envelope = %v, want sql.ErrNoRows", err)
	}
	e := EnvelopeRow{RunID: "run_1", ExecutionSeconds: 1800, Repairs: 3, Replans: 2, TransportRetries: 5, UpdatedAt: "2026-01-01T00:00:00Z"}
	for _, step := range []func(*EnvelopeRow){func(*EnvelopeRow) {}, func(e *EnvelopeRow) {
		e.FirstStartAt, e.RepairsUsed, e.PauseSpans, e.UpdatedAt = "2026-01-01T00:01:00Z", 1, `[{"requested_at":"a"}]`, "2026-01-01T00:02:00Z"
	}} {
		step(&e)
		if err := transact(t, j, func(ctx context.Context, tx *sql.Tx) error { return UpsertRunEnvelope(ctx, tx, e) }); err != nil {
			t.Fatal(err)
		}
	}
	got, err := j.RunEnvelope(t.Context(), "run_1")
	if err != nil {
		t.Fatal(err)
	}
	if got != e {
		t.Fatalf("envelope = %+v, want %+v", got, e)
	}
}

func TestTransactRollsBackOnError(t *testing.T) {
	j, _ := openTemp(t)
	seedRun(t, j, "run_1")
	boom := errors.New("boom")
	err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		if err := SetBucketExhausted(t.Context(), tx, "b", "2026-01-01T00:00:00Z", nil); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Transact = %v, want boom", err)
	}
	if _, err := j.BucketState(t.Context(), "b"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("write survived a failed Transact: %v", err)
	}
}

func TestMigrationFilesPresent(t *testing.T) {
	// guards the embed: the ledger migration must be the third file
	names, err := filepath.Glob(filepath.Join("migrations", "*.sql"))
	if err != nil || len(names) != SchemaVersion {
		t.Fatalf("migrations = %v (%v), want %d files", names, err, SchemaVersion)
	}
	if _, err := os.Stat(filepath.Join("migrations", "0003_ledger.sql")); err != nil {
		t.Fatal(err)
	}
}
