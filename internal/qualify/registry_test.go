package qualify

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
	_ "modernc.org/sqlite" // registers the "sqlite" driver for crafted databases and row inspection
)

// fileURI builds a file: URI for path, mirroring the test pattern in
// internal/admission/billing_test.go.
func fileURI(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// makeRawDB creates dir/mythhelm.db holding schema at the given
// user_version, for foreign/corrupt/mismatched database tests.
func makeRawDB(t *testing.T, dir string, version int, schema string) {
	t.Helper()
	db, err := sql.Open("sqlite", fileURI(filepath.Join(dir, journal.DBName)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if schema != "" {
		if _, err := db.Exec(schema); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		t.Fatal(err)
	}
}

// hashDir hashes the names and contents of dir's entries, so any created,
// removed or changed file changes it.
func hashDir(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if _, err := fmt.Fprintf(h, "%s\n", e.Name()); err != nil {
			t.Fatal(err)
		}
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // G304: names come from the test's own temp dir listing
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// TestOpenMigratesWithoutSeeding pins that a read-write open migrates the
// table and lists empty: Open never creates qualification evidence — only
// EnsureSeeded adds records.
func TestOpenMigratesWithoutSeeding(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	dir := t.TempDir()

	reg, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, journal.DBName)); err != nil {
		t.Fatalf("Open did not migrate the database into the state dir: %v", err)
	}
	recs, err := reg.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("List after fresh Open = %d records, want empty (Open never seeds)", len(recs))
	}
	if err := reg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// The migrated table accepts the seed through a journal handle, and a
	// later open reads it.
	j, err := journal.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureSeeded(ctx, j); err != nil {
		t.Fatalf("EnsureSeeded: %v", err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	reg2, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open after seeding: %v", err)
	}
	defer func() { _ = reg2.Close() }()
	recs, err = reg2.List(ctx)
	if err != nil {
		t.Fatalf("List after seeding: %v", err)
	}
	if len(recs) != 7 {
		t.Fatalf("List after EnsureSeeded = %d records, want 7", len(recs))
	}
}

// TestOpenReadOnlyWritesNothing pins the A30 lineage: a read-only open over
// a fresh dir migrates and seeds nothing — the directory hash is unchanged
// — lists empty without error, and refuses writes.
func TestOpenReadOnlyWritesNothing(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	dir := t.TempDir()

	before := hashDir(t, dir)
	reg, err := OpenReadOnly(ctx, dir)
	if err != nil {
		t.Fatalf("OpenReadOnly on a fresh dir: %v", err)
	}
	recs, err := reg.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("List over a fresh dir = %d records, want empty", len(recs))
	}
	if err := reg.Record(ctx, Record{}); err == nil {
		t.Error("Record on a read-only registry succeeded, want an error (read-only open cannot mutate)")
	}
	if err := reg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if after := hashDir(t, dir); after != before {
		t.Fatal("OpenReadOnly changed the state dir, want it byte-identical (migrates and seeds nothing)")
	}
}

// TestOpenReadOnlyMissingDirErrors pins the missing-dir failure: the error
// names the dir (doctor keys `unavailable` off it), and List against a
// database without the table wraps ErrSchemaMismatch.
func TestOpenReadOnlyMissingDirErrors(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	missing := filepath.Join(t.TempDir(), "no-such-dir")
	_, err := OpenReadOnly(ctx, missing)
	if err == nil {
		t.Fatal("OpenReadOnly on a missing dir succeeded, want an error naming the dir")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("missing-dir error = %q, want it to name %q", err, missing)
	}
	if !IsMissing(err) {
		t.Error("IsMissing(missing-dir error) = false, want true")
	}
	if IsSchemaMismatch(err) {
		t.Errorf("IsSchemaMismatch(missing-dir error) = true, want false: %v", err)
	}

	dir := t.TempDir()
	makeRawDB(t, dir, journal.SchemaVersion, `CREATE TABLE other (x TEXT)`)
	reg, err := OpenReadOnly(ctx, dir)
	if err != nil {
		t.Fatalf("OpenReadOnly on a table-less v2 database: %v", err)
	}
	defer func() { _ = reg.Close() }()
	if _, err := reg.List(ctx); err == nil {
		t.Error("List against a database without the table succeeded, want an ErrSchemaMismatch-wrapping error")
	} else if !IsSchemaMismatch(err) {
		t.Errorf("List error = %v, want it to wrap ErrSchemaMismatch", err)
	}
	if _, err := reg.Lookup(ctx, Key{Harness: "claude-code"}); !IsSchemaMismatch(err) {
		t.Errorf("Lookup error = %v, want it to wrap ErrSchemaMismatch", err)
	}
}

// TestOpenReadOnlyZeroVersionErrors pins that a database file with
// user_version=0 errors, never an empty registry.
func TestOpenReadOnlyZeroVersionErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	makeRawDB(t, dir, 0, `CREATE TABLE other (x TEXT)`)

	_, err := OpenReadOnly(t.Context(), dir)
	if err == nil {
		t.Fatal("OpenReadOnly on a version-0 database succeeded, want an error, never an empty registry")
	}
	if IsMissing(err) {
		t.Errorf("IsMissing(version-0 error) = true, want false: %v", err)
	}
	if !IsSchemaMismatch(err) {
		t.Errorf("version-0 error = %v, want it to wrap ErrSchemaMismatch", err)
	}
}

// TestOpenReadOnlyVersionMismatchErrors pins that a newer user_version
// surfaces the wrapped ErrSchemaTooNew (doctor keys `unavailable` off it),
// and an older (v1) database surfaces a schema mismatch, never empty.
func TestOpenReadOnlyVersionMismatchErrors(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	dir := t.TempDir()
	makeRawDB(t, dir, journal.SchemaVersion+1, `CREATE TABLE other (x TEXT)`)
	_, err := OpenReadOnly(ctx, dir)
	if err == nil {
		t.Fatal("OpenReadOnly on a newer database succeeded, want the wrapped ErrSchemaTooNew")
	}
	if !errors.Is(err, journal.ErrSchemaTooNew) {
		t.Errorf("newer-db error = %v, want it to wrap ErrSchemaTooNew", err)
	}
	if !IsSchemaMismatch(err) {
		t.Errorf("newer-db error = %v, want it to wrap ErrSchemaMismatch", err)
	}

	olddir := t.TempDir()
	makeRawDB(t, olddir, 1, `CREATE TABLE other (x TEXT)`)
	_, err = OpenReadOnly(ctx, olddir)
	if err == nil {
		t.Fatal("OpenReadOnly on a v1 database succeeded, want a schema-mismatch error")
	}
	if !IsSchemaMismatch(err) {
		t.Errorf("v1 error = %v, want it to wrap ErrSchemaMismatch", err)
	}
	if errors.Is(err, journal.ErrSchemaTooNew) {
		t.Errorf("v1 error wraps ErrSchemaTooNew, want only the mismatch: %v", err)
	}
}

// Fixture digests: realistic sha256 shapes identifying the test builds.
const (
	testExecDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	testCfgDigest  = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

// testKey returns a fully-established qualification key for harness.
func testKey(harness string) Key {
	return Key{
		Harness:          harness,
		Surface:          "native-cli-structured (print, stream-json)",
		ExecutableDigest: testExecDigest,
		AdapterProtocol:  "builtin/claudecode+stream-json",
		OS:               "linux",
		Arch:             "amd64",
		ProviderEndpoint: "first-party-subscription",
		ModelSnapshot:    "2026-09-01-claude-opus-4-6",
		EffortSettings:   "none",
		AuthCategory:     "claude.ai/max",
		ConfigDigest:     testCfgDigest,
		TrustProfile:     "trusted-host",
		WorkspaceClass:   "local-checkout",
		EntitlementClass: "included-plan",
	}
}

// testRecord returns a valid blocked record for harness: not-proven and
// unknown columns, unknown quota, empty Digest (Record derives it) and zero
// Revision (Record assigns it).
func testRecord(harness string) Record {
	return Record{
		SchemaVersion: 2,
		Key:           testKey(harness),
		Progress:      ProgressBlocked,
		Fidelity:      Column{Verdict: Unknown, Evidence: []Evidence{}},
		Entitlement:   Column{Verdict: NotProven, Evidence: []Evidence{}},
		Lifecycle:     Column{Verdict: Unknown, Evidence: []Evidence{}},
		Capabilities:  map[string]Capability{},
		Quota:         Datum{Quantity: "unknown", Label: DatumUnknown, Unit: "unknown", Scope: "unknown", Source: "unknown"},
		NextTest:      "run the authorised fixture suite; authority: maintainer grant",
	}
}

// testEvidence returns one offline-fixture evidence entry with id.
func testEvidence(id string) Evidence {
	return Evidence{
		ID: id, Method: "offline-fixture", Suite: "at03-routes", Result: "pass",
		Uncertainty: "unknown", Label: Observed, Source: "fixture harness",
	}
}

// openRegistry opens a read-write registry over a fresh temp state dir,
// closing it when the test ends.
func openRegistry(t *testing.T) (*Registry, string) {
	t.Helper()
	dir := t.TempDir()
	reg, err := Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reg.Close() })
	return reg, dir
}

// TestLookupRoundTrip pins the record/read contract: Record then Lookup
// returns revision 1 with a recomputing digest — a passed-in Revision 99 is
// ignored — a second store reads revision 2, and unknown keys read
// ErrNotFound, never a zero record.
func TestLookupRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reg, dir := openRegistry(t)

	rec := testRecord("claude-code")
	rec.Revision = 99
	if err := reg.Record(ctx, rec); err != nil {
		t.Fatalf("Record: %v", err)
	}
	got, err := reg.Lookup(ctx, rec.Key)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got.Revision != 1 {
		t.Errorf("Lookup revision = %d, want 1 (passed-in Revision 99 ignored)", got.Revision)
	}
	if recomputed, err := CanonicalDigest(got); err != nil {
		t.Fatalf("CanonicalDigest: %v", err)
	} else if got.Digest != recomputed {
		t.Errorf("stored digest %q does not recompute (want %q)", got.Digest, recomputed)
	}
	if got.Key != rec.Key {
		t.Errorf("Lookup key = %+v, want %+v", got.Key, rec.Key)
	}

	second := got
	second.Digest = ""
	second.NextTest = "retest after new evidence; authority: maintainer grant"
	if err := reg.Record(ctx, second); err != nil {
		t.Fatalf("Record second revision: %v", err)
	}
	got2, err := reg.Lookup(ctx, rec.Key)
	if err != nil {
		t.Fatalf("Lookup after second Record: %v", err)
	}
	if got2.Revision != 2 {
		t.Errorf("Lookup revision = %d, want 2 (Record stores current+1)", got2.Revision)
	}

	if _, err := reg.Lookup(ctx, testKey("no-such-harness")); !errors.Is(err, ErrNotFound) {
		t.Errorf("Lookup unknown key = %v, want ErrNotFound", err)
	}

	// A stored row whose digest does not recompute fails the read: the
	// tampered revision is planted directly through the journal.
	j, err := journal.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	bad := testRecord("codex")
	bad.Digest = "sha256:" + strings.Repeat("0", 64)
	raw, err := json.Marshal(bad)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.InsertQualificationRecord(ctx, KeyHash(bad.Key), 1, bad.Digest, string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Lookup(ctx, bad.Key); err == nil {
		t.Error("Lookup of a tampered row succeeded, want the stored-digest-mismatch error")
	} else if !strings.Contains(err.Error(), "stored record digest mismatch") {
		t.Errorf("Lookup tampered = %v, want the stored-digest-mismatch error", err)
	}
}

// TestRecordRefusesTamperedDigest pins fail-closed writes: a record whose
// Digest does not recompute is refused and writes nothing.
func TestRecordRefusesTamperedDigest(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reg, _ := openRegistry(t)

	rec := testRecord("claude-code")
	rec.Digest = "sha256:" + strings.Repeat("0", 64)
	err := reg.Record(ctx, rec)
	if err == nil {
		t.Fatal("Record with a tampered digest succeeded, want qualify: record digest mismatch")
	}
	if !strings.Contains(err.Error(), "qualify: record digest mismatch:") {
		t.Errorf("Record tampered = %q, want the digest-mismatch error", err)
	}
	if _, err := reg.Lookup(ctx, rec.Key); !errors.Is(err, ErrNotFound) {
		t.Errorf("Lookup after refused Record = %v, want ErrNotFound (writes nothing)", err)
	}
	if recs, err := reg.List(ctx); err != nil || len(recs) != 0 {
		t.Errorf("List after refused Record = %d records, %v; want empty", len(recs), err)
	}
}

// TestRecordRefusesEvidenceFreeProven pins the honesty rule: a column
// valued proven with empty evidence names its column and writes nothing.
func TestRecordRefusesEvidenceFreeProven(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reg, _ := openRegistry(t)

	for _, column := range []string{"fidelity", "entitlement", "lifecycle"} {
		rec := testRecord("claude-code-" + column)
		switch column {
		case "fidelity":
			rec.Fidelity.Verdict = Proven
		case "entitlement":
			rec.Entitlement.Verdict = Proven
		case "lifecycle":
			rec.Lifecycle.Verdict = Proven
		}
		err := reg.Record(ctx, rec)
		if err == nil {
			t.Errorf("Record with %s proven and empty evidence succeeded, want the proven-without-evidence error", column)
			continue
		}
		if want := "qualify: proven verdict without evidence: " + column; !strings.Contains(err.Error(), want) {
			t.Errorf("Record %s = %q, want it to contain %q", column, err, want)
		}
		if _, err := reg.Lookup(ctx, rec.Key); !errors.Is(err, ErrNotFound) {
			t.Errorf("Lookup after refused %s Record = %v, want ErrNotFound", column, err)
		}
	}
}

// TestConsultMatchesStableIgnoresDigests pins Consult: an observed key whose
// digests drifted from the recorded one stable-matches with drifted=true
// and a reason naming the field; an unknown harness reads ErrNotFound; two
// stable matches refuse as ambiguous.
func TestConsultMatchesStableIgnoresDigests(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reg, _ := openRegistry(t)

	rec := testRecord("claude-code")
	if err := reg.Record(ctx, rec); err != nil {
		t.Fatal(err)
	}

	found, drifted, reason, err := reg.Consult(ctx, rec.Key)
	if err != nil {
		t.Fatalf("Consult exact key: %v", err)
	}
	if drifted || reason != "" {
		t.Errorf("Consult exact key drifted=%v reason=%q, want clean", drifted, reason)
	}
	if found.Key != rec.Key || found.Revision != 1 {
		t.Errorf("Consult exact key returns rev %d for %+v, want rev 1 for the recorded key", found.Revision, found.Key)
	}

	observed := rec.Key
	observed.ExecutableDigest = driftExecOther
	observed.ConfigDigest = driftCfgOther
	found, drifted, reason, err = reg.Consult(ctx, observed)
	if err != nil {
		t.Fatalf("Consult with drifted digests: %v", err)
	}
	if !drifted {
		t.Error("Consult with drifted digests reports clean, want drifted=true")
	}
	if !strings.Contains(reason, "executable_digest") {
		t.Errorf("Consult reason %q does not name executable_digest", reason)
	}
	if found.Key != rec.Key {
		t.Errorf("Consult stable match returns %+v, want the recorded key", found.Key)
	}

	cfgOnly := rec.Key
	cfgOnly.ConfigDigest = driftCfgOther
	if _, drifted, reason, err := reg.Consult(ctx, cfgOnly); err != nil {
		t.Fatalf("Consult with drifted config: %v", err)
	} else if !drifted || !strings.Contains(reason, "config_digest") {
		t.Errorf("Consult config drift = (%v, %q), want drifted naming config_digest", drifted, reason)
	}

	unknown := rec.Key
	unknown.Harness = "no-such-harness"
	if _, _, _, err := reg.Consult(ctx, unknown); !errors.Is(err, ErrNotFound) {
		t.Errorf("Consult unknown harness = %v, want ErrNotFound", err)
	}

	twin := rec
	twin.Key.ExecutableDigest = driftExecOther
	if err := reg.Record(ctx, twin); err != nil {
		t.Fatalf("Record twin stable identity: %v", err)
	}
	third := rec.Key
	third.ExecutableDigest = "sha256:" + strings.Repeat("e", 64)
	third.ConfigDigest = "sha256:" + strings.Repeat("f", 64)
	if _, _, _, err := reg.Consult(ctx, third); err == nil {
		t.Fatal("Consult with two stable matches succeeded, want the ambiguous-match error")
	} else if !strings.Contains(err.Error(), "qualify: ambiguous stable match:") {
		t.Errorf("Consult ambiguous = %v, want the ambiguous-match error", err)
	}
}

// storedRow reads one (key_hash, revision) row's record_json, digest and
// superseded_at columns directly, for revision-immutability assertions the
// Registry API cannot observe (it only reads current rows).
func storedRow(t *testing.T, dir, keyHash string, revision int) (recordJSON, digest string, superseded *string) {
	t.Helper()
	db, err := sql.Open("sqlite", fileURI(filepath.Join(dir, journal.DBName))+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var stored, dg string
	var sup sql.NullString
	err = db.QueryRow(`SELECT record_json, digest, superseded_at FROM qualification_records WHERE key_hash = ? AND revision = ?`,
		keyHash, revision).Scan(&stored, &dg, &sup)
	if err != nil {
		t.Fatalf("reading row for revision %d: %v", revision, err)
	}
	if sup.Valid {
		return stored, dg, &sup.String
	}
	return stored, dg, nil
}

// TestInvalidateResetsColumns pins drift invalidation (I20): a new revision
// with affected columns reset, progress blocked and the reason preserved,
// while the old revision's bytes stay unchanged.
func TestInvalidateResetsColumns(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reg, dir := openRegistry(t)

	rec := testRecord("claude-code")
	rec.Progress = ProgressFixtureTested
	rec.Fidelity = Column{Verdict: Proven, Evidence: []Evidence{testEvidence("ev_fixture1")}}
	rec.Entitlement = Column{Verdict: Proven, Evidence: []Evidence{testEvidence("ev_fixture2")}}
	// Lifecycle stays unknown with empty evidence: nothing to invalidate.
	if err := reg.Record(ctx, rec); err != nil {
		t.Fatalf("Record: %v", err)
	}
	keyHash := KeyHash(rec.Key)
	beforeJSON, beforeDigest, beforeSuperseded := storedRow(t, dir, keyHash, 1)
	if beforeSuperseded != nil {
		t.Fatalf("revision 1 superseded before invalidation: %q", *beforeSuperseded)
	}

	reason := "executable_digest drifted: pinned " + testExecDigest + ", observed " + driftExecOther
	if err := reg.InvalidateOnDrift(ctx, rec, reason); err != nil {
		t.Fatalf("InvalidateOnDrift: %v", err)
	}

	got, err := reg.Lookup(ctx, rec.Key)
	if err != nil {
		t.Fatalf("Lookup after invalidation: %v", err)
	}
	if got.Revision != 2 {
		t.Errorf("invalidated revision = %d, want 2 (invalidation stores a new revision)", got.Revision)
	}
	if got.Progress != ProgressBlocked {
		t.Errorf("invalidated progress = %q, want blocked", got.Progress)
	}
	for _, column := range []struct {
		name  string
		col   Column
		prior string
	}{
		{"fidelity", got.Fidelity, "ev_fixture1"},
		{"entitlement", got.Entitlement, "ev_fixture2"},
	} {
		if column.col.Verdict != NotProven {
			t.Errorf("invalidated %s = %q, want not-proven (prior proof no longer establishes it)", column.name, column.col.Verdict)
		}
		if len(column.col.Evidence) != 1 {
			t.Fatalf("invalidated %s carries %d evidence entries, want the 1 invalidation marker", column.name, len(column.col.Evidence))
		}
		marker := column.col.Evidence[0]
		if !strings.Contains(marker.Uncertainty, reason) {
			t.Errorf("invalidated %s marker uncertainty %q does not preserve the reason", column.name, marker.Uncertainty)
		}
		if !strings.Contains(marker.Uncertainty, column.prior) {
			t.Errorf("invalidated %s marker uncertainty %q does not preserve prior evidence %s", column.name, marker.Uncertainty, column.prior)
		}
	}
	if got.Lifecycle.Verdict != Unknown || len(got.Lifecycle.Evidence) != 0 {
		t.Errorf("invalidated lifecycle = (%q, %d evidence), want (unknown, empty): nothing claimed, nothing to reset",
			got.Lifecycle.Verdict, len(got.Lifecycle.Evidence))
	}
	if recomputed, err := CanonicalDigest(got); err != nil {
		t.Fatalf("CanonicalDigest: %v", err)
	} else if got.Digest != recomputed {
		t.Errorf("invalidated digest %q does not recompute (want %q)", got.Digest, recomputed)
	}

	afterJSON, afterDigest, afterSuperseded := storedRow(t, dir, keyHash, 1)
	if afterJSON != beforeJSON {
		t.Error("revision 1 record_json changed under invalidation, want it byte-identical")
	}
	if afterDigest != beforeDigest {
		t.Error("revision 1 digest changed under invalidation, want it unchanged")
	}
	if afterSuperseded == nil {
		t.Error("revision 1 superseded_at unset after invalidation, want it set")
	}

	// A stale argument still invalidates the current row, never forking
	// from the passed content: the second invalidation chains onto
	// revision 2.
	if err := reg.InvalidateOnDrift(ctx, rec, reason+" (again)"); err != nil {
		t.Fatalf("second InvalidateOnDrift: %v", err)
	}
	got3, err := reg.Lookup(ctx, rec.Key)
	if err != nil {
		t.Fatalf("Lookup after second invalidation: %v", err)
	}
	if got3.Revision != 3 {
		t.Errorf("twice-invalidated revision = %d, want 3", got3.Revision)
	}
	if len(got3.Fidelity.Evidence) != 1 || !strings.Contains(got3.Fidelity.Evidence[0].Uncertainty, "ev_drift_invalidation") {
		t.Errorf("twice-invalidated fidelity does not chain onto the prior marker: %+v", got3.Fidelity.Evidence)
	}

	if err := reg.InvalidateOnDrift(ctx, testRecord("no-such-harness"), reason); !errors.Is(err, ErrNotFound) {
		t.Errorf("InvalidateOnDrift unknown key = %v, want ErrNotFound", err)
	}
	if err := reg.InvalidateOnDrift(ctx, rec, ""); err == nil {
		t.Error("InvalidateOnDrift with an empty reason succeeded, want an error")
	}
}

// TestRegistryLookupLatency pins NFR-1: a cold Lookup on a temp-dir
// registry completes in under 1 s. The delayed variant exceeds the bound,
// proving the test discriminates. Not parallel: a wall-clock bound should
// not compete with suite load.
func TestRegistryLookupLatency(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	setup, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	rec := testRecord("claude-code")
	if err := setup.Record(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err := setup.Close(); err != nil {
		t.Fatal(err)
	}

	// A fresh handle makes the measured Lookup cold.
	reg, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reg.Close() }()

	start := time.Now()
	if _, err := reg.Lookup(ctx, rec.Key); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Errorf("cold Lookup took %v, want under 1s (NFR-1)", elapsed)
	}

	// Discriminating variant: the same lookup with a 1.5 s injected delay
	// must exceed the bound — if it did not, the assertion above could
	// never fail.
	start = time.Now()
	time.Sleep(1500 * time.Millisecond)
	if _, err := reg.Lookup(ctx, rec.Key); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if elapsed := time.Since(start); elapsed < time.Second {
		t.Error("Lookup with a 1.5 s injected delay completed within the bound; the latency assertion cannot discriminate")
	}
}
