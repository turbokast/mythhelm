package migrate_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/migrate"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// openRaw opens a read-write handle on the state database in dir. Backup
// takes *sql.DB (the migrate package works below the Journal type, as Apply
// will), so tests hold their own handle and close every journal before
// calling into Backup/Restore.
func openRaw(t *testing.T, dir string) *sql.DB {
	t.Helper()
	p := filepath.ToSlash(filepath.Join(dir, journal.DBName))
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	dsn := (&url.URL{Scheme: "file", Path: p, RawQuery: "_pragma=busy_timeout(5000)"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// appendV1 appends one schema_version-1 envelope for runID and returns its
// event_id.
func appendV1(t *testing.T, j *journal.Journal, runID string, seq int64) string {
	t.Helper()
	ev := journal.Event{
		SchemaVersion:    1,
		EventID:          "evt-" + runID + "-" + strconv.FormatInt(seq, 10),
		RunID:            runID,
		ProducerID:       "worker-" + runID,
		ProducerSequence: seq,
		Generation:       0,
		ObservedAt:       time.Date(2026, 10, 1, 0, 0, int(seq), 0, time.UTC),
		Type:             "decision",
		Payload:          json.RawMessage(`{"choice":"a","status":"ready_for_review"}`),
	}
	if err := j.Append(t.Context(), ev, nil); err != nil {
		t.Fatal(err)
	}
	return ev.EventID
}

// projections reads the v1 journal rows for runID through the v1 decoder and
// returns their canonical JSON: the "v1 projections" the round trip must
// preserve byte-identically.
func projections(t *testing.T, j *journal.Journal, runID string) string {
	t.Helper()
	evs, err := j.Events(t.Context(), runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(evs)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// hashDir hashes every regular file under dir (sorted relative paths plus
// bytes): the "target dir's SHA-256" refusal tests hold unchanged.
func hashDir(t *testing.T, dir string) string {
	t.Helper()
	var names []string
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		names = append(names, rel)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	for _, n := range names {
		raw, err := os.ReadFile(filepath.Join(dir, n)) //nolint:gosec // G304: the test's own temp dir; n comes from walking it
		if err != nil {
			t.Fatal(err)
		}
		h.Write([]byte(n))
		h.Write([]byte{0})
		h.Write(raw)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// codeOf extracts the catalogue code from a migrate error, failing the test
// when the error is not a valid ControlError.
func codeOf(t *testing.T, err error) v2contract.Code {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var cerr *v2contract.ControlError
	if !errors.As(err, &cerr) {
		t.Fatalf("error is not a *v2contract.ControlError: %T %v", err, err)
	}
	if err := cerr.Validate(); err != nil {
		t.Fatalf("ControlError fails Validate: %v (%v)", err, cerr)
	}
	return cerr.Code
}

// fixtureWithV1 opens a migrated state dir holding two v1 envelopes and
// returns the dir; every handle is closed on return so Backup/Restore own
// the files.
func fixtureWithV1(t *testing.T) (dir, runID string) {
	t.Helper()
	dir = t.TempDir()
	runID = "run-t5"
	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	appendV1(t, j, runID, 1)
	appendV1(t, j, runID, 2)
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	return dir, runID
}

func TestBackupRestoreRoundTrip(t *testing.T) {
	ctx := t.Context()
	dir, runID := fixtureWithV1(t)

	j, err := journal.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	before := projections(t, j, runID)
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	raw := openRaw(t, dir)
	info, err := migrate.Backup(ctx, raw, dir)
	if err != nil {
		t.Fatal(err)
	}
	// Restore requires every handle closed (last close checkpoints the WAL).
	_ = raw.Close()

	// The sidecar carries schema version, build version, digest, timestamp.
	sidecarRaw, err := os.ReadFile(info.Path + ".json")
	if err != nil {
		t.Fatalf("reading sidecar: %v", err)
	}
	var sidecar migrate.BackupInfo
	if err := json.Unmarshal(sidecarRaw, &sidecar); err != nil {
		t.Fatalf("sidecar is not BackupInfo JSON: %v", err)
	}
	if sidecar != info {
		t.Errorf("sidecar %+v != returned info %+v", sidecar, info)
	}
	if info.SchemaVersion != journal.SchemaVersion {
		t.Errorf("sidecar schema_version = %d, want %d", info.SchemaVersion, journal.SchemaVersion)
	}
	if info.BuildVersion == "" {
		t.Error("sidecar build_version is empty")
	}
	backupRaw, err := os.ReadFile(info.Path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(backupRaw)
	if info.SHA256 != hex.EncodeToString(sum[:]) {
		t.Error("sidecar digest does not match the backup bytes")
	}
	if _, err := time.Parse(time.RFC3339, info.CreatedAt); err != nil {
		t.Errorf("sidecar created_at %q is not RFC3339: %v", info.CreatedAt, err)
	}

	// Migrate after the backup: post-backup writes the restore must drop.
	j2, err := journal.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	appendV1(t, j2, runID, 3)
	if err := j2.Close(); err != nil {
		t.Fatal(err)
	}
	mut := openRaw(t, dir)
	if _, err := mut.ExecContext(ctx, `UPDATE migration_state SET phase = 'drained' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	_ = mut.Close()

	if err := migrate.Restore(ctx, info, dir); err != nil {
		t.Fatal(err)
	}

	j3, err := journal.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j3.Close() }()
	if after := projections(t, j3, runID); after != before {
		t.Errorf("v1 projections differ after restore:\nbefore %s\nafter  %s", before, after)
	}
	evs, err := j3.Events(ctx, runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 {
		t.Errorf("restored journal holds %d events, want 2 (post-backup write dropped)", len(evs))
	}
}

func TestRestoreNewerSchemaRefuses(t *testing.T) {
	ctx := t.Context()
	dir, _ := fixtureWithV1(t)
	raw := openRaw(t, dir)
	info, err := migrate.Backup(ctx, raw, dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	info.SchemaVersion = journal.SchemaVersion + 1
	before := hashDir(t, dir)
	if code := codeOf(t, migrate.Restore(ctx, info, dir)); code != v2contract.CodeSchemaTooNew {
		t.Errorf("code = %q, want %q", code, v2contract.CodeSchemaTooNew)
	}
	if after := hashDir(t, dir); after != before {
		t.Error("target dir changed on schema_too_new refusal")
	}
}

func TestRestoreDigestMismatchRefuses(t *testing.T) {
	ctx := t.Context()
	dir, _ := fixtureWithV1(t)
	raw := openRaw(t, dir)
	info, err := migrate.Backup(ctx, raw, dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	// Tamper with the backup bytes after the sidecar digest was recorded.
	backupRaw, err := os.ReadFile(info.Path)
	if err != nil {
		t.Fatal(err)
	}
	backupRaw[len(backupRaw)/2] ^= 0xff
	if err := os.WriteFile(info.Path, backupRaw, 0o600); err != nil { //nolint:gosec // G703: the backup this test just took in its temp dir
		t.Fatal(err)
	}

	before := hashDir(t, dir)
	if code := codeOf(t, migrate.Restore(ctx, info, dir)); code != v2contract.CodeInvalidContract {
		t.Errorf("code = %q, want %q", code, v2contract.CodeInvalidContract)
	}
	if after := hashDir(t, dir); after != before {
		t.Error("target dir changed on digest-mismatch refusal")
	}
}

// TestBackupPinsMatchJournal pins backup.go's mirrors of the journal
// constants behaviorally (they are copies, not imports: journal's tests
// import migrate, so importing journal from migrate would cycle). A schema
// bump that forgets to sweep backup.go fails here: the expected backup name
// derives from journal.DBName, and a restore at exactly
// journal.SchemaVersion must be accepted.
// TestRestoreSidecarVersionMismatchRefuses pins the backup file as
// authoritative for its own version: a sidecar disagreeing with the file
// refuses with invalid_contract and writes nothing.
func TestRestoreSidecarVersionMismatchRefuses(t *testing.T) {
	ctx := t.Context()
	dir, _ := fixtureWithV1(t)
	raw := openRaw(t, dir)
	info, err := migrate.Backup(ctx, raw, dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	info.SchemaVersion = journal.SchemaVersion - 1
	before := hashDir(t, dir)
	if code := codeOf(t, migrate.Restore(ctx, info, dir)); code != v2contract.CodeInvalidContract {
		t.Errorf("code = %q, want %q", code, v2contract.CodeInvalidContract)
	}
	if after := hashDir(t, dir); after != before {
		t.Error("target dir changed on sidecar/file version-mismatch refusal")
	}
}

// TestRestoreLoweredSidecarWithNewerBackupRefuses is the downgrade case:
// the sidecar digest covers the database bytes, not the sidecar fields, so
// a lowered schema_version with a correct digest must still refuse with
// schema_too_new when the backup file itself is newer than the binary.
func TestRestoreLoweredSidecarWithNewerBackupRefuses(t *testing.T) {
	ctx := t.Context()
	dir, _ := fixtureWithV1(t)

	newerDir := t.TempDir()
	j, err := journal.Open(ctx, newerDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	newerRaw := openRaw(t, newerDir)
	if _, err := newerRaw.ExecContext(ctx, "PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	_ = newerRaw.Close()
	newerPath := filepath.Join(newerDir, journal.DBName)
	newerBytes, err := os.ReadFile(newerPath) //nolint:gosec // G304: the fixture database this test just built in its temp dir
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(newerBytes)
	info := migrate.BackupInfo{
		Path:          newerPath,
		SchemaVersion: journal.SchemaVersion, // lowered: the file is v99
		BuildVersion:  "test",
		SHA256:        hex.EncodeToString(sum[:]),
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	before := hashDir(t, dir)
	if code := codeOf(t, migrate.Restore(ctx, info, dir)); code != v2contract.CodeSchemaTooNew {
		t.Errorf("code = %q, want %q", code, v2contract.CodeSchemaTooNew)
	}
	if after := hashDir(t, dir); after != before {
		t.Error("target dir changed on lowered-sidecar refusal")
	}
}

// TestRestoreRefusesLiveWAL pins the pre-staging guard: a non-empty WAL at
// the target may hold uncheckpointed frames, so Restore refuses with
// persistence_unavailable before staging anything, leaving the WAL intact.
func TestRestoreRefusesLiveWAL(t *testing.T) {
	ctx := t.Context()
	dir, _ := fixtureWithV1(t)
	raw := openRaw(t, dir)
	info, err := migrate.Backup(ctx, raw, dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	wal := filepath.Join(dir, journal.DBName+"-wal")
	frames := []byte("uncheckpointed-frames")
	if err := os.WriteFile(wal, frames, 0o600); err != nil {
		t.Fatal(err)
	}
	before := hashDir(t, dir)
	if code := codeOf(t, migrate.Restore(ctx, info, dir)); code != v2contract.CodePersistenceUnavailable {
		t.Errorf("code = %q, want %q", code, v2contract.CodePersistenceUnavailable)
	}
	if after := hashDir(t, dir); after != before {
		t.Error("target dir changed on live-WAL refusal")
	}
	if kept, _ := os.ReadFile(wal); string(kept) != string(frames) { //nolint:gosec // G304: the WAL plant this test just wrote in its temp dir
		t.Error("live WAL bytes changed on refusal")
	}
}

func TestBackupPinsMatchJournal(t *testing.T) {
	ctx := t.Context()
	dir, _ := fixtureWithV1(t)
	raw := openRaw(t, dir)
	var version int
	if err := raw.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != journal.SchemaVersion {
		t.Fatalf("fixture user_version = %d, journal.SchemaVersion = %d", version, journal.SchemaVersion)
	}
	info, err := migrate.Backup(ctx, raw, dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	want := filepath.Join(dir, journal.DBName+".bak-migration-v"+strconv.Itoa(version))
	if info.Path != want {
		t.Errorf("backup path = %q, want %q", info.Path, want)
	}
	if err := migrate.Restore(ctx, info, dir); err != nil {
		t.Errorf("restore at journal.SchemaVersion refused: %v", err)
	}
}

func TestBackupRefusesOverwrite(t *testing.T) {
	ctx := t.Context()
	dir, _ := fixtureWithV1(t)
	raw := openRaw(t, dir)
	info, err := migrate.Backup(ctx, raw, dir)
	if err != nil {
		t.Fatal(err)
	}
	backupBefore, err := os.ReadFile(info.Path)
	if err != nil {
		t.Fatal(err)
	}
	sidecarBefore, err := os.ReadFile(info.Path + ".json")
	if err != nil {
		t.Fatal(err)
	}

	_, err = migrate.Backup(ctx, raw, dir)
	if code := codeOf(t, err); code != v2contract.CodeInvalidContract {
		t.Errorf("code = %q, want %q", code, v2contract.CodeInvalidContract)
	}
	if after, _ := os.ReadFile(info.Path); string(after) != string(backupBefore) {
		t.Error("existing backup bytes changed on overwrite refusal")
	}
	if after, _ := os.ReadFile(info.Path + ".json"); string(after) != string(sidecarBefore) {
		t.Error("existing sidecar bytes changed on overwrite refusal")
	}
}
