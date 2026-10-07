// TestRecordRevisionImmutability lives in the external test package: it
// decodes stored rows through qualify, and qualify imports journal for
// EnsureSeeded, so an internal (package journal) test cannot import it
// ("import cycle not allowed in test").
package journal_test

import (
	"database/sql"
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/qualify"
	_ "modernc.org/sqlite" // registers the "sqlite" driver for row inspection
)

// openStored opens dbPath read-only for inspecting stored rows. It mirrors
// journal's DSN construction (percent-encoding, leading slash) without
// reaching into unexported helpers.
func openStored(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	p := filepath.ToSlash(dbPath)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	uri := (&url.URL{Scheme: "file", Path: p, RawQuery: "mode=ro&_pragma=busy_timeout(5000)"}).String()
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type storedRow struct {
	keyHash      string
	revision     int
	digest       string
	recordJSON   string
	recordedAt   string
	supersededAt sql.NullString
}

func readRow(t *testing.T, db *sql.DB, keyHash string, revision int) storedRow {
	t.Helper()
	var row storedRow
	err := db.QueryRowContext(t.Context(), `SELECT key_hash, revision, digest, record_json, recorded_at, superseded_at
		FROM qualification_records WHERE key_hash = ? AND revision = ?`, keyHash, revision).
		Scan(&row.keyHash, &row.revision, &row.digest, &row.recordJSON, &row.recordedAt, &row.supersededAt)
	if err != nil {
		t.Fatalf("reading row (%s, %d): %v", keyHash, revision, err)
	}
	return row
}

// revisionRecord builds a fully explicit record for key: every scale value
// is set, so the stored bytes never depend on decode defaults.
func revisionRecord(key qualify.Key, revision int, progress qualify.Progress, nextTest string) qualify.Record {
	empty := qualify.Column{Verdict: qualify.Unknown, Evidence: []qualify.Evidence{}}
	return qualify.Record{
		SchemaVersion: 2,
		Revision:      revision,
		Key:           key,
		Progress:      progress,
		Fidelity:      empty,
		Entitlement:   empty,
		Lifecycle:     empty,
		Capabilities:  map[string]qualify.Capability{},
		Quota: qualify.Datum{
			Quantity: "unknown",
			Label:    qualify.DatumUnknown,
			Unit:     "unknown",
			Scope:    "unknown",
			Source:   "unknown",
		},
		NextTest: nextTest,
	}
}

func TestRecordRevisionImmutability(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	j, err := journal.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	key := qualify.Key{
		Harness:          "claude-code",
		Surface:          "native-cli-structured (print, stream-json)",
		ExecutableDigest: "sha256:1111",
		AdapterProtocol:  "builtin/claudecode+stream-json",
		OS:               "linux",
		Arch:             "amd64",
		ProviderEndpoint: "unknown",
		ModelSnapshot:    "unknown",
		EffortSettings:   "none",
		AuthCategory:     "unknown",
		ConfigDigest:     "unknown",
		TrustProfile:     "unknown",
		WorkspaceClass:   "unknown",
		EntitlementClass: "unknown",
	}
	keyHash := qualify.KeyHash(key)
	stored := make([]storedRow, 0, 2)
	for _, rev := range []struct {
		revision int
		progress qualify.Progress
		nextTest string
	}{
		{revision: 1, progress: qualify.ProgressPlanned, nextTest: "enumerate the first evidence step; authority: maintainer"},
		{revision: 2, progress: qualify.ProgressBlocked, nextTest: "run the authorised fixture suite; authority: maintainer grant"},
	} {
		rec := revisionRecord(key, rev.revision, rev.progress, rev.nextTest)
		digest, err := qualify.CanonicalDigest(rec)
		if err != nil {
			t.Fatal(err)
		}
		rec.Digest = digest
		raw, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		if err := j.InsertQualificationRecord(ctx, keyHash, rev.revision, digest, string(raw)); err != nil {
			t.Fatalf("Insert revision %d: %v", rev.revision, err)
		}
		stored = append(stored, storedRow{digest: digest, recordJSON: string(raw)})
	}

	// Current returns revision 2.
	gotJSON, gotRev, err := j.CurrentQualificationRecord(ctx, keyHash)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if gotRev != 2 || gotJSON != stored[1].recordJSON {
		t.Fatalf("Current = revision %d, want 2 with the revision-2 bytes", gotRev)
	}

	// Revision 1's bytes and digest are unchanged; only the superseded_at
	// metadata moved.
	raw := openStored(t, filepath.Join(dir, journal.DBName))
	row := readRow(t, raw, keyHash, 1)
	if row.recordJSON != stored[0].recordJSON {
		t.Fatal("revision 1 record_json changed after inserting revision 2")
	}
	if row.digest != stored[0].digest {
		t.Fatal("revision 1 digest changed after inserting revision 2")
	}
	if !row.supersededAt.Valid {
		t.Fatal("revision 1 superseded_at is still NULL after inserting revision 2")
	}

	// Both rows' digests recompute (NFR-2): two readers of the same
	// revision always see the same record.
	for _, rev := range []int{1, 2} {
		r := readRow(t, raw, keyHash, rev)
		decoded, err := qualify.DecodeRecord([]byte(r.recordJSON))
		if err != nil {
			t.Fatalf("DecodeRecord revision %d: %v", rev, err)
		}
		recomputed, err := qualify.CanonicalDigest(decoded)
		if err != nil {
			t.Fatalf("CanonicalDigest revision %d: %v", rev, err)
		}
		if recomputed != r.digest {
			t.Fatalf("revision %d digest does not recompute: stored %s, recomputed %s", rev, r.digest, recomputed)
		}
	}

	// The recompute check discriminates: changed content yields a changed digest.
	var tampered map[string]any
	if err := json.Unmarshal([]byte(stored[0].recordJSON), &tampered); err != nil {
		t.Fatal(err)
	}
	tampered["progress"] = string(qualify.ProgressExperimental)
	rawTampered, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	decodedTampered, err := qualify.DecodeRecord(rawTampered)
	if err != nil {
		t.Fatalf("DecodeRecord tampered copy: %v", err)
	}
	recomputedTampered, err := qualify.CanonicalDigest(decodedTampered)
	if err != nil {
		t.Fatalf("CanonicalDigest tampered copy: %v", err)
	}
	if recomputedTampered == stored[0].digest {
		t.Fatal("changed content recomputes to the unchanged digest")
	}
}
