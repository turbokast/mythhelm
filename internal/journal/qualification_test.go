package journal

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// frozenV1MigrationSHA256 is the sha256 of origin/main's
// internal/journal/migrations/0001_init.sql. A released migration is never
// edited (v2 §5.2); this golden pins that.
const frozenV1MigrationSHA256 = "262e63e2c7630dc637a916d476a1862db361affcb734dcbe2317f6770cb72e36"

func TestMigration0002Applies(t *testing.T) {
	ctx := t.Context()

	raw, err := os.ReadFile(filepath.Clean("migrations/0001_init.sql"))
	if err != nil {
		t.Fatalf("reading 0001_init.sql: %v", err)
	}
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != frozenV1MigrationSHA256 {
		t.Fatalf("0001_init.sql sha256 = %s, want %s (released migrations are frozen)", got, frozenV1MigrationSHA256)
	}

	if SchemaVersion != 4 {
		t.Fatalf("SchemaVersion = %d, want 4", SchemaVersion)
	}

	// A database at user_version=1 migrates to 2 with the table present and
	// its existing rows intact.
	dir, _ := v1Database(t)
	j, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open on a v1 database: %v", err)
	}
	defer func() { _ = j.Close() }()
	var version int
	if err := j.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != SchemaVersion {
		t.Fatalf("user_version = %d after migration, want %d", version, SchemaVersion)
	}
	for _, name := range []string{"qualification_records", "idx_qualification_current"} {
		var found string
		err := j.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE name = ?`, name).Scan(&found)
		if err != nil {
			t.Fatalf("migration 0002 did not create %s: %v", name, err)
		}
	}
	var journalRows int
	if err := j.db.QueryRowContext(ctx, `SELECT count(*) FROM journal`).Scan(&journalRows); err != nil {
		t.Fatal(err)
	}
	if journalRows != 1 {
		t.Fatalf("journal holds %d rows after migration, want 1 (additive migration)", journalRows)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	// A fresh Open reports user_version "4".
	fresh, _ := openTemp(t)
	var freshVersion string
	if err := fresh.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&freshVersion); err != nil {
		t.Fatal(err)
	}
	if freshVersion != "4" {
		t.Fatalf("fresh user_version = %q, want %q", freshVersion, "4")
	}

	// OpenReadOnly opens the migrated database without ErrSchemaTooNew.
	ro, err := OpenReadOnly(ctx, dir)
	if errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("OpenReadOnly on a migrated database: %v, want no ErrSchemaTooNew", err)
	}
	if err != nil {
		t.Fatalf("OpenReadOnly on a migrated database: %v", err)
	}
	if err := ro.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestInsertRefusesInvalid(t *testing.T) {
	j, dir := openTemp(t)
	ctx := t.Context()
	tests := []struct {
		name       string
		keyHash    string
		revision   int
		digest     string
		recordJSON string
	}{
		{name: "empty key hash", keyHash: "", revision: 1, digest: "sha256:abc", recordJSON: `{}`},
		{name: "empty digest", keyHash: "kh", revision: 1, digest: "", recordJSON: `{}`},
		{name: "empty record JSON", keyHash: "kh", revision: 1, digest: "sha256:abc", recordJSON: ""},
		{name: "zero revision", keyHash: "kh", revision: 0, digest: "sha256:abc", recordJSON: `{}`},
		{name: "negative revision", keyHash: "kh", revision: -1, digest: "sha256:abc", recordJSON: `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := j.InsertQualificationRecord(ctx, tt.keyHash, tt.revision, tt.digest, tt.recordJSON)
			if err == nil || err.Error() != "journal: invalid qualification record" {
				t.Fatalf("Insert err = %v, want %q", err, "journal: invalid qualification record")
			}
		})
	}
	// Every refusal wrote nothing.
	if recs, err := j.ListCurrentQualificationRecords(ctx); err != nil || len(recs) != 0 {
		t.Fatalf("List after refused inserts = %v, %v; want empty, nil", recs, err)
	}
	var rows int
	if err := rawOpen(t, filepath.Join(dir, DBName)).QueryRowContext(ctx,
		`SELECT count(*) FROM qualification_records`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("table holds %d rows after refused inserts, want 0", rows)
	}
}

func TestAbsentIsNotFound(t *testing.T) {
	j, _ := openTemp(t)
	recordJSON, revision, err := j.CurrentQualificationRecord(t.Context(), "no-such-key")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Current on an unknown key err = %v, want ErrNotFound", err)
	}
	if recordJSON != "" || revision != 0 {
		t.Fatalf("Current on an unknown key = (%q, %d), want empty (never a zero record)", recordJSON, revision)
	}
}

func TestInsertIfAbsentNoOverwrite(t *testing.T) {
	j, dir := openTemp(t)
	ctx := t.Context()

	inserted, err := j.InsertQualificationRecordIfAbsent(ctx, "kh-absent", 1, "sha256:first", `{"rev":1}`)
	if err != nil {
		t.Fatalf("IfAbsent on an absent key: %v", err)
	}
	if !inserted {
		t.Fatal("IfAbsent on an absent key inserted = false, want true")
	}
	gotJSON, gotRev, err := j.CurrentQualificationRecord(ctx, "kh-absent")
	if err != nil || gotJSON != `{"rev":1}` || gotRev != 1 {
		t.Fatalf("Current after IfAbsent = (%q, %d, %v), want (%q, 1, nil)", gotJSON, gotRev, err, `{"rev":1}`)
	}

	before := fullRowHash(t, dir, "kh-absent", 1)
	inserted, err = j.InsertQualificationRecordIfAbsent(ctx, "kh-absent", 1, "sha256:second", `{"rev":1,"forged":true}`)
	if err != nil {
		t.Fatalf("IfAbsent on a present key: %v", err)
	}
	if inserted {
		t.Fatal("IfAbsent on a present key inserted = true, want false")
	}
	if after := fullRowHash(t, dir, "kh-absent", 1); after != before {
		t.Fatal("IfAbsent on a present key changed the existing row")
	}
	var rows int
	if err := rawOpen(t, filepath.Join(dir, DBName)).QueryRowContext(ctx,
		`SELECT count(*) FROM qualification_records WHERE key_hash = ?`, "kh-absent").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("IfAbsent on a present key left %d rows, want 1 (no new revision)", rows)
	}

	for _, tt := range []struct {
		name       string
		keyHash    string
		revision   int
		digest     string
		recordJSON string
	}{
		{name: "empty key hash", keyHash: "", revision: 1, digest: "sha256:x", recordJSON: `{}`},
		{name: "empty digest", keyHash: "kh", revision: 1, digest: "", recordJSON: `{}`},
		{name: "empty record JSON", keyHash: "kh", revision: 1, digest: "sha256:x", recordJSON: ""},
		{name: "zero revision", keyHash: "kh", revision: 0, digest: "sha256:x", recordJSON: `{}`},
	} {
		t.Run("invalid/"+tt.name, func(t *testing.T) {
			inserted, err := j.InsertQualificationRecordIfAbsent(ctx, tt.keyHash, tt.revision, tt.digest, tt.recordJSON)
			if err == nil || err.Error() != "journal: invalid qualification record" {
				t.Fatalf("IfAbsent err = %v, want %q", err, "journal: invalid qualification record")
			}
			if inserted {
				t.Fatal("IfAbsent with invalid input inserted = true, want false")
			}
		})
	}
}

// fullRowHash hashes every column of one stored row, so any overwrite,
// supersede or timestamp touch changes it.
func fullRowHash(t *testing.T, dir, keyHash string, revision int) [32]byte {
	t.Helper()
	var key, digest, recordJSON, recordedAt string
	var rev int
	var supersededAt sql.NullString
	err := rawOpen(t, filepath.Join(dir, DBName)).QueryRowContext(t.Context(), `SELECT key_hash, revision, digest, record_json, recorded_at, superseded_at
		FROM qualification_records WHERE key_hash = ? AND revision = ?`, keyHash, revision).
		Scan(&key, &rev, &digest, &recordJSON, &recordedAt, &supersededAt)
	if err != nil {
		t.Fatalf("reading row: %v", err)
	}
	bytes, err := json.Marshal([]any{key, rev, digest, recordJSON, recordedAt, supersededAt.String, supersededAt.Valid})
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(bytes)
}
