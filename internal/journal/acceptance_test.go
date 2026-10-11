package journal

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

// SHA-256 of the released migrations 0003 to 0007 as shipped before schema 8
// (0001 and 0002 are pinned in ledger_test.go): schema 8 is additive, so none
// of them may change (ADR 0016).
var frozenMigrationSHA256 = map[string]string{
	"0003_ledger.sql":           "c9203ba912b7fe187d472edfa56f02fd751ffaddcb12d3eee4c98bf920487aa6",
	"0004_supervisor.sql":       "2078c4755d71cc0d73003f1025209aa3bb1afd234fe992a830056ac9def22910",
	"0005_reservation_host.sql": "d5915d954ce5c3033b87c2e129020fbfaf02517c77bacb894e9355f2ddc81790",
	"0006_evaluator.sql":        "97805eaba1b0d89c7a67bb51f1b173264088496ac2207e79910836656092a5c5",
	"0007_v2contracts.sql":      "f1adfc66751e45808379f83464c8eb4fa629ae86afa4be3a74cbaaca888cd098",
}

const immutableAbort = "is immutable"

func p1Policy() v2contract.PolicyVersion {
	return v2contract.PolicyVersion{
		SchemaVersion: v2contract.SchemaVersion,
		PolicyID:      "P1",
		Version:       1,
		ActionFamily:  "code-change",
		Prompts:       map[string]string{},
		Context:       map[string]string{},
		Review:        map[string]string{"gate": "human-apply"},
		Decomposition: map[string]string{},
		Evidence:      []string{"protected-checks"},
	}
}

func attestationFor(t *testing.T, verificationID string) ([]byte, string) {
	t.Helper()
	att := v2contract.Verification{
		SchemaVersion:            v2contract.SchemaVersion,
		VerificationID:           verificationID,
		Candidate:                v2contract.GitObject{Format: "sha1", Value: strings.Repeat("a", 40)},
		Environment:              "linux/amd64",
		AcceptanceContractDigest: "sha256:" + strings.Repeat("b", 64),
		CheckDefinition:          "sha256:" + strings.Repeat("c", 64),
		CheckVersion:             "1",
		VerifierID:               "mythhelm-checks",
		Results:                  map[string]string{"unit": "passed"},
		Logs:                     []string{"sha256:" + strings.Repeat("d", 64)},
	}
	raw, err := json.Marshal(att)
	if err != nil {
		t.Fatalf("encoding the attestation fixture: %v", err)
	}
	return raw, v2contract.Digest(att)
}

func insertVerificationRow(t *testing.T, j *Journal, v VerificationRow) error {
	t.Helper()
	return j.Transact(t.Context(), func(tx *sql.Tx) error { return InsertVerification(t.Context(), tx, v) })
}

func verificationRow(runID, id string) VerificationRow {
	now := time.Now().UTC()
	return VerificationRow{ID: id, RunID: runID, CandidateCommit: "c", ConfigSHA256: "cfg", Result: "passed", StartedAt: now, FinishedAt: now}
}

// insertSchema7Verification writes a verification the way a schema-7 build
// did: the verifications row and nothing beside it.
func insertSchema7Verification(t *testing.T, db *sql.DB, runID, id string) {
	t.Helper()
	now := formatTime(time.Now().UTC())
	if _, err := db.ExecContext(t.Context(), `INSERT INTO runs (run_id, state, reason, adapter_id, source_repo, source_branch, base_rev, task_sha256, billing_posture, execution_profile, created_at, updated_at)
		VALUES (?, 'ready_for_review', '', 'builtin/fake', '/r', 'main', 'abc', 'sha', 'local-scripted', 'trusted-host', ?, ?)`, runID, now, now); err != nil {
		t.Fatalf("seeding the schema-7 run: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO verifications (verification_id, run_id, candidate_commit, config_sha256, result, started_at, finished_at)
		VALUES (?, ?, 'c', 'cfg', 'passed', ?, ?)`, id, runID, now, now); err != nil {
		t.Fatalf("seeding the schema-7 verification: %v", err)
	}
}

func TestMigration0008AdditiveAndV7DataSurvives(t *testing.T) { // I09 (v2 §2): an absent binding is Bound == false, never a verdict
	ctx := t.Context()
	for name, want := range frozenMigrationSHA256 {
		if got := migrationSHA(t, name); got != want {
			t.Errorf("%s sha256 = %s, want %s: a released migration is never edited", name, got, want)
		}
	}
	if got := migrationSHA(t, "0001_init.sql"); got != sha0001 {
		t.Errorf("0001_init.sql sha256 = %s, want %s", got, sha0001)
	}
	if got := migrationSHA(t, "0002_qualification.sql"); got != sha0002 {
		t.Errorf("0002_qualification.sql sha256 = %s, want %s", got, sha0002)
	}

	dir := t.TempDir()
	v7, err := open(ctx, dir, migrations[:7])
	if err != nil {
		t.Fatal(err)
	}
	before := schemaObjects(t, v7.db)
	if _, ok := before["verification_bindings"]; ok {
		t.Fatal("verification_bindings exists at schema 7")
	}
	insertSchema7Verification(t, v7.db, "run_old", "ver_old")
	if err := v7.Close(); err != nil {
		t.Fatal(err)
	}

	j, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open on a schema-7 database: %v", err)
	}
	defer func() { _ = j.Close() }()
	var version int
	if err := j.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 8 {
		t.Fatalf("user_version = %d after Open, want 8", version)
	}
	var stateSchema int
	if err := j.db.QueryRowContext(ctx, `SELECT schema_version FROM migration_state WHERE id = 1`).Scan(&stateSchema); err != nil {
		t.Fatal(err)
	}
	if stateSchema != SchemaVersion {
		t.Errorf("migration_state.schema_version = %d after the upgrade, want %d", stateSchema, SchemaVersion)
	}
	got, err := j.LatestVerification(ctx, "run_old")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "ver_old" || got.Result != "passed" {
		t.Errorf("the schema-7 verification = %+v, want ver_old passed intact", got)
	}
	if got.Bound || got.Reason != "" || got.Attestation != nil || got.AttestationDigest != "" || got.OverheadMS != nil {
		t.Errorf("schema-7 verification = %+v, want Bound false with no reason, attestation or overhead", got)
	}

	after := schemaObjects(t, j.db)
	for name, ddl := range before {
		if after[name] != ddl {
			t.Errorf("DDL of %s changed by migration 0008:\nbefore %q\nafter  %q", name, ddl, after[name])
		}
	}
	for _, table := range []string{"verifications", "check_results"} {
		if after[table] == "" || after[table] != before[table] {
			t.Errorf("%s DDL in sqlite_master changed or vanished: %q", table, after[table])
		}
	}
	wantNew := map[string]bool{
		"verification_bindings": true, "verification_bindings_no_update": true,
		"verification_bindings_no_delete": true, "verification_bindings_no_replace": true,
		"policies_no_update": true, "policies_no_delete": true, "policies_no_replace": true,
	}
	for name := range wantNew {
		if after[name] == "" {
			t.Errorf("schema object %s missing after migration 0008", name)
		}
	}
	for name := range after {
		if _, ok := before[name]; ok || strings.HasPrefix(name, "sqlite_autoindex_") {
			continue
		}
		if !wantNew[name] {
			t.Errorf("unexpected new schema object %q after migration 0008", name)
		}
	}
}

func TestVerificationBindingRoundTrip(t *testing.T) { // I20 (v2 §2): the binding is a row beside the verification
	ctx := t.Context()
	j, _ := openTemp(t)
	seedRun(t, j, "run_full")
	seedRun(t, j, "run_bare")
	seedRun(t, j, "run_rejected")

	att, digest := attestationFor(t, "ver_full")
	overhead := int64(42)
	full := verificationRow("run_full", "ver_full")
	full.Reason, full.Attestation, full.AttestationDigest, full.OverheadMS = "waived by --no-checks", att, digest, &overhead
	if err := insertVerificationRow(t, j, full); err != nil {
		t.Fatal(err)
	}
	got, err := j.LatestVerification(ctx, "run_full")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Bound || got.Reason != full.Reason || !bytes.Equal(got.Attestation, att) || got.AttestationDigest != digest ||
		got.OverheadMS == nil || *got.OverheadMS != overhead {
		t.Errorf("round trip = %+v, want Bound true with reason %q, the attestation, digest %q and overhead %d", got, full.Reason, digest, overhead)
	}

	if err := insertVerificationRow(t, j, verificationRow("run_bare", "ver_bare")); err != nil {
		t.Fatal(err)
	}
	bare, err := j.LatestVerification(ctx, "run_bare")
	if err != nil {
		t.Fatal(err)
	}
	if !bare.Bound || bare.Reason != "" || bare.Attestation != nil || bare.AttestationDigest != "" || bare.OverheadMS != nil {
		t.Errorf("bare verification = %+v, want Bound true and every binding field empty", bare)
	}

	own, ownDigest := attestationFor(t, "ver_rejected")
	other, otherDigest := attestationFor(t, "ver_other")
	for name, tc := range map[string]struct {
		edit func(*VerificationRow)
	}{
		"attestation without digest": {func(v *VerificationRow) { v.Attestation = own }},
		"digest without attestation": {func(v *VerificationRow) { v.AttestationDigest = ownDigest }},
		"attestation that is not JSON": {func(v *VerificationRow) {
			v.Attestation, v.AttestationDigest = []byte("{not json"), ownDigest
		}},
		"JSON that is not a verification record": {func(v *VerificationRow) {
			v.Attestation, v.AttestationDigest = []byte("{}"), "sha256:"+strings.Repeat("0", 64)
		}},
		"digest that is not the attestation's": {func(v *VerificationRow) {
			v.Attestation, v.AttestationDigest = own, "sha256:"+strings.Repeat("0", 64)
		}},
		"attestation of another verification": {func(v *VerificationRow) {
			v.Attestation, v.AttestationDigest = other, otherDigest
		}},
	} {
		v := verificationRow("run_rejected", "ver_rejected")
		tc.edit(&v)
		if err := insertVerificationRow(t, j, v); err == nil {
			t.Errorf("%s: InsertVerification accepted the row, want it rejected", name)
		}
		if _, err := j.LatestVerification(ctx, "run_rejected"); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: LatestVerification = %v, want ErrNotFound (the verification rolled back with its binding)", name, err)
		}
	}

	good := verificationRow("run_rejected", "ver_rejected")
	good.Attestation, good.AttestationDigest = own, ownDigest
	if err := insertVerificationRow(t, j, good); err != nil {
		t.Errorf("the matching attestation and digest was rejected: %v", err)
	}
}

// The Go checks above reject a malformed binding first; these statements
// reach the schema's own CHECKs directly, so they stay proven (teeth).
func TestBindingChecksRejectMalformedRows(t *testing.T) {
	ctx := t.Context()
	j, _ := openTemp(t)
	seedRun(t, j, "run_1")
	now := formatTime(time.Now().UTC())
	if _, err := j.db.ExecContext(ctx, `INSERT INTO verifications (verification_id, run_id, candidate_commit, config_sha256, result, started_at, finished_at)
		VALUES ('ver_1', 'run_1', 'c', 'cfg', 'passed', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, stmt string }{
		{"attestation without digest", `INSERT INTO verification_bindings (verification_id, attestation) VALUES ('ver_1', '{}')`},
		{"digest without attestation", `INSERT INTO verification_bindings (verification_id, digest) VALUES ('ver_1', 'sha256:x')`},
		{"attestation that is not JSON", `INSERT INTO verification_bindings (verification_id, attestation, digest) VALUES ('ver_1', '{not json', 'sha256:x')`},
	} {
		_, err := j.db.ExecContext(ctx, tc.stmt)
		if err == nil || !strings.Contains(err.Error(), "CHECK constraint failed") {
			t.Errorf("%s: err = %v, want the CHECK constraint to reject it", tc.name, err)
		}
	}
	if _, err := j.db.ExecContext(ctx, `INSERT INTO verification_bindings (verification_id, attestation, digest) VALUES ('ver_1', '{}', 'sha256:x')`); err != nil {
		t.Errorf("a paired, valid-JSON attestation and digest was refused by the schema: %v", err)
	}
}

func TestBindingsAndPoliciesImmutable(t *testing.T) { // I20 (v2 §2): policies and bindings are immutable by trigger
	ctx := t.Context()
	j, _ := openTemp(t)
	seedRun(t, j, "run_1")
	reason := "waived by --no-checks"
	v := verificationRow("run_1", "ver_1")
	v.Reason = reason
	if err := insertVerificationRow(t, j, v); err != nil {
		t.Fatal(err)
	}
	if err := j.Transact(ctx, func(tx *sql.Tx) error { return EnsurePolicy(ctx, tx, p1Policy()) }); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ name, stmt string }{
		{"bindings replace", `INSERT OR REPLACE INTO verification_bindings (verification_id, reason) VALUES ('ver_1', 'changed')`},
		{"bindings plain insert over a key", `INSERT INTO verification_bindings (verification_id, reason) VALUES ('ver_1', 'changed')`},
		{"bindings update", `UPDATE verification_bindings SET reason = 'changed'`},
		{"bindings delete", `DELETE FROM verification_bindings`},
		{"policies replace", `INSERT OR REPLACE INTO policies (policy_id, version, record, digest) VALUES ('P1', 1, '{}', 'sha256:x')`},
		{"policies plain insert over a key", `INSERT INTO policies (policy_id, version, record, digest) VALUES ('P1', 1, '{}', 'sha256:x')`},
		{"policies update", `UPDATE policies SET digest = 'sha256:x'`},
		{"policies delete", `DELETE FROM policies`},
	} {
		_, err := j.db.ExecContext(ctx, tc.stmt)
		if err == nil || !strings.Contains(err.Error(), immutableAbort) {
			t.Errorf("%s: err = %v, want the immutable abort", tc.name, err)
		}
	}

	got, err := j.LatestVerification(ctx, "run_1")
	if err != nil || !got.Bound || got.Reason != reason {
		t.Errorf("binding after the refused writes = %+v, %v; want Bound true with reason %q", got, err, reason)
	}
	var stored string
	if err := j.db.QueryRowContext(ctx, `SELECT digest FROM policies WHERE policy_id = 'P1' AND version = 1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if want := v2contract.Digest(p1Policy()); stored != want {
		t.Errorf("stored P1 digest = %q after the refused writes, want %q", stored, want)
	}

	if _, err := j.db.ExecContext(ctx, `INSERT INTO policies (policy_id, version, record, digest) VALUES ('P1', 2, '{}', 'sha256:y')`); err != nil {
		t.Errorf("a new policy version was refused: %v; the triggers must close only changes to an existing key", err)
	}
}

func TestEnsurePolicyIdempotentAndConflict(t *testing.T) { // I20 (v2 §2)
	ctx := t.Context()
	j, _ := openTemp(t)
	ensure := func(p v2contract.PolicyVersion) error {
		return j.Transact(ctx, func(tx *sql.Tx) error { return EnsurePolicy(ctx, tx, p) })
	}
	count := func() int {
		var n int
		if err := j.db.QueryRowContext(ctx, `SELECT count(*) FROM policies WHERE policy_id = 'P1' AND version = 1`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	for i := range 2 {
		if err := ensure(p1Policy()); err != nil {
			t.Fatalf("EnsurePolicy call %d: %v", i+1, err)
		}
	}
	if n := count(); n != 1 {
		t.Fatalf("policies holds %d rows for P1 v1 after two identical calls, want 1", n)
	}

	var record, digest string
	readRow := func() {
		if err := j.db.QueryRowContext(ctx, `SELECT record, digest FROM policies WHERE policy_id = 'P1' AND version = 1`).Scan(&record, &digest); err != nil {
			t.Fatal(err)
		}
	}
	readRow()
	storedRecord, storedDigest := record, digest
	if storedDigest != v2contract.Digest(p1Policy()) {
		t.Errorf("stored digest = %q, want the v2contract.Digest of the record", storedDigest)
	}

	other := p1Policy()
	other.Evidence = []string{"protected-checks", "extra"}
	err := ensure(other)
	if !errors.Is(err, ErrPolicyConflict) {
		t.Fatalf("EnsurePolicy with another digest = %v, want ErrPolicyConflict", err)
	}
	readRow()
	if record != storedRecord || digest != storedDigest {
		t.Errorf("stored P1 changed by the conflicting call:\nrecord %q -> %q\ndigest %q -> %q", storedRecord, record, storedDigest, digest)
	}
	if n := count(); n != 1 {
		t.Errorf("policies holds %d rows for P1 v1 after the conflict, want 1", n)
	}

	invalid := p1Policy()
	invalid.PolicyID = ""
	if err := ensure(invalid); err == nil || errors.Is(err, ErrPolicyConflict) {
		t.Errorf("EnsurePolicy with an invalid record = %v, want a validation error, not a conflict", err)
	}
}

// Every transaction is BEGIN IMMEDIATE (journal.go pragmas, _txlock), so the
// read-then-insert in EnsurePolicy runs under the write lock and concurrent
// identical callers serialise: all return nil and one row remains.
func TestEnsurePolicyConcurrentIdenticalWritesAreIdempotent(t *testing.T) {
	ctx := t.Context()
	j, _ := openTemp(t)
	const writers = 8
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() {
			errs <- j.Transact(ctx, func(tx *sql.Tx) error { return EnsurePolicy(ctx, tx, p1Policy()) })
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent EnsurePolicy = %v, want nil for an identical record", err)
		}
	}
	var n int
	if err := j.db.QueryRowContext(ctx, `SELECT count(*) FROM policies WHERE policy_id = 'P1' AND version = 1`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("policies holds %d rows for P1 v1 after %d concurrent identical writes, want 1", n, writers)
	}
}
