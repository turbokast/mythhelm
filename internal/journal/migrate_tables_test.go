package journal

import (
	"strings"
	"testing"
)

// The five tables migration 0007 adds (supervisor-migration design §2, D2;
// renumbered from 0004: later specs consumed 0003-0006 first).
var v2ContractTables = []string{"tasks", "task_revisions", "policies", "grants", "migration_state"}

// migrationPhases is the migration_state vocabulary as literals. This
// internal test cannot name migrate.Phase: importing internal/migrate here
// would cycle once migrate imports control, supervisor or journal (Tasks 4
// and 6; design §2 mandates migrate calls control.Mutate). The constants
// stay bound to these words by TestPhaseMatchesMigrationVocabulary in
// internal/migrate/drain_test.go, which asserts both directions.
var migrationPhases = []string{"not_started", "previewed", "drained", "imported", "adopted"}

// The ten v1 tables from 0001_init.sql; no migration may alter their DDL.
var v1Tables = []string{"runs", "attempts", "journal", "producers", "candidates",
	"verifications", "check_results", "trust_grants", "declarations", "applies"}

func TestMigration0007CreatesV2Tables(t *testing.T) {
	dir := t.TempDir()
	v6, err := open(t.Context(), dir, migrations[:6])
	if err != nil {
		t.Fatal(err)
	}
	before := schemaObjects(t, v6.db)
	if err := v6.Close(); err != nil {
		t.Fatal(err)
	}
	for _, table := range v2ContractTables {
		if _, ok := before[table]; ok {
			t.Fatalf("table %s exists before migration 0007", table)
		}
	}

	j, err := Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	after := schemaObjects(t, j.db)
	for _, table := range v2ContractTables {
		if after[table] == "" {
			t.Errorf("table %s missing after migration 0007", table)
		}
	}
	for _, table := range v1Tables {
		if after[table] != before[table] {
			t.Errorf("DDL of v1 table %s changed by migration 0007:\nbefore %q\nafter  %q",
				table, before[table], after[table])
		}
	}
	// Additive only (G16): every new schema object belongs to 0007.
	wantNew := map[string]bool{}
	for _, table := range v2ContractTables {
		wantNew[table] = true
	}
	wantNew["task_revisions_run"] = true // the run_id lookup index on task_revisions
	for name := range after {
		if _, ok := before[name]; ok {
			continue
		}
		if strings.HasPrefix(name, "sqlite_autoindex_") {
			continue
		}
		if !wantNew[name] {
			t.Errorf("unexpected new schema object %q after migration 0007", name)
		}
	}
}

// NOTE: TestMigration0001Untouched already exists in ledger_test.go (added by
// budget-ledger-s1); it is not duplicated here and still guards 0001_init.sql.

func TestMigrationStateStartsNotStarted(t *testing.T) {
	j, _ := openTemp(t)
	var phase, startedAt, backupPath, buildVersion string
	var schemaVersion, rows int
	err := j.db.QueryRowContext(t.Context(), `SELECT phase, started_at, backup_path, schema_version, build_version,
		(SELECT count(*) FROM migration_state) FROM migration_state`).Scan(
		&phase, &startedAt, &backupPath, &schemaVersion, &buildVersion, &rows)
	if err != nil {
		t.Fatalf("reading migration_state: %v", err)
	}
	if rows != 1 {
		t.Fatalf("migration_state holds %d rows, want exactly 1", rows)
	}
	if phase != migrationPhases[0] {
		t.Errorf("phase = %q, want %q", phase, migrationPhases[0])
	}
	if schemaVersion != SchemaVersion {
		t.Errorf("schema_version = %d, want SchemaVersion %d", schemaVersion, SchemaVersion)
	}
	// The migration records the schema identity; the build identity and the
	// start marker stay empty until the Apply step (Task 6) records them —
	// SQL cannot know the Go build that ran it.
	if buildVersion != "" {
		t.Errorf("build_version = %q, want empty until apply", buildVersion)
	}
	if startedAt != "" || backupPath != "" {
		t.Errorf("started_at = %q, backup_path = %q, want both empty before apply", startedAt, backupPath)
	}
}

func TestMigrationStatePhaseVocabulary(t *testing.T) {
	j, _ := openTemp(t)
	ctx := t.Context()
	for _, phase := range migrationPhases {
		if _, err := j.db.ExecContext(ctx, `UPDATE migration_state SET phase = ?`, phase); err != nil {
			t.Errorf("phase %q rejected by the migration_state CHECK: %v", phase, err)
		}
	}
	if _, err := j.db.ExecContext(ctx, `UPDATE migration_state SET phase = 'bogus'`); err == nil {
		t.Error("migration_state accepted phase 'bogus', want the CHECK to reject it")
	}
}

func TestSchemaVersionIs7(t *testing.T) {
	if SchemaVersion != 7 {
		t.Fatalf("SchemaVersion = %d, want 7", SchemaVersion)
	}
	j, dir := openTemp(t)
	var version int
	if err := j.db.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != SchemaVersion {
		t.Fatalf("user_version = %d, want SchemaVersion %d", version, SchemaVersion)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	ro, err := OpenReadOnly(t.Context(), dir)
	if err != nil {
		t.Fatalf("OpenReadOnly on a 0007 database: %v", err)
	}
	if err := ro.Close(); err != nil {
		t.Fatal(err)
	}
}
