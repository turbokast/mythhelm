package journal

import (
	"database/sql"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/migrate/preview"
)

const epochMigration = "0010_service_assignment_epochs.sql"

// mirrorDisagreements names every independent schema mirror that differs
// from want, in a stable order.
func mirrorDisagreements(mirrors map[string]int, want int) []string {
	var out []string
	for name, got := range mirrors {
		if got != want {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func TestAssignmentEpochMigration(t *testing.T) { // I09/I20/I23 (v2 sections 5/6): additive schema, historical unknowns stay unknown.
	t.Parallel()
	t.Run("schema9 legacy rows survive without backfill", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		legacy, err := open(t.Context(), dir, migrations[:9])
		if err != nil {
			t.Fatal(err)
		}
		seedRun(t, legacy, "legacy")
		seedRun(t, legacy, "unassigned")
		attachmentTx(t, legacy, func(tx *sql.Tx) {
			attachmentSQL(t, tx, `INSERT INTO operations VALUES ('old', 'service.adopt', 'legacy', 'digest', 'done', 4, '{"unknown":null}', 'then', 'then')`)
			attachmentSQL(t, tx, `INSERT INTO run_assignments VALUES ('legacy', 'supervisor', 'then', 'old')`)
		})
		before := schemaObjects(t, legacy.db)
		var revisionBefore uint64
		attachmentTx(t, legacy, func(tx *sql.Tx) { revisionBefore = attachmentRevision(t, tx, "legacy") })
		if err := legacy.Close(); err != nil {
			t.Fatal(err)
		}
		j, err := Open(t.Context(), dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = j.Close() }()
		after := schemaObjects(t, j.db)
		for name, ddl := range before {
			if after[name] != ddl {
				t.Errorf("released schema object %s changed", name)
			}
		}
		if ddl := after["service_assignment_epochs"]; !strings.Contains(ddl, "STRICT") || !strings.Contains(ddl, "DEFERRABLE INITIALLY DEFERRED") {
			t.Fatalf("epoch table DDL = %q, want STRICT with deferred assignment FK", ddl)
		}
		var version, stateSchema int
		if err := j.db.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version); err != nil {
			t.Fatal(err)
		}
		if err := j.db.QueryRowContext(t.Context(), `SELECT schema_version FROM migration_state`).Scan(&stateSchema); err != nil {
			t.Fatal(err)
		}
		if version != 10 || stateSchema != 10 {
			t.Fatalf("user_version=%d migration_state=%d, want 10", version, stateSchema)
		}
		var assignment, result string
		var epochs int
		if err := j.db.QueryRowContext(t.Context(), `SELECT run_id||'|'||owner||'|'||assigned_at||'|'||operation_id FROM run_assignments`).Scan(&assignment); err != nil {
			t.Fatal(err)
		}
		if err := j.db.QueryRowContext(t.Context(), `SELECT result FROM operations WHERE operation_id='old'`).Scan(&result); err != nil {
			t.Fatal(err)
		}
		if err := j.db.QueryRowContext(t.Context(), `SELECT count(*) FROM service_assignment_epochs`).Scan(&epochs); err != nil {
			t.Fatal(err)
		}
		if assignment != "legacy|supervisor|then|old" || result != `{"unknown":null}` || epochs != 0 {
			t.Fatalf("assignment=%q result=%q epochs=%d, want legacy rows unchanged and no backfill", assignment, result, epochs)
		}
		attachmentTx(t, j, func(tx *sql.Tx) {
			if got := attachmentRevision(t, tx, "legacy"); got != revisionBefore {
				t.Errorf("migration revision=%d, want unchanged %d", got, revisionBefore)
			}
			for _, run := range []string{"legacy", "unassigned"} {
				if got, ok, err := ReadAssignmentEpoch(t.Context(), tx, run); err != nil || ok {
					t.Errorf("%s epoch = %+v %t %v, want unknown", run, got, ok, err)
				}
			}
			// Released four-column positional INSERT still works without epochs.
			attachmentSQL(t, tx, `INSERT INTO run_assignments VALUES ('unassigned', 'supervisor', 'now', 'old')`)
			attachmentSQL(t, tx, `UPDATE run_assignments SET owner = 'tui' WHERE run_id = 'legacy'`)
		})
	})
	t.Run("migration order and reservation", func(t *testing.T) {
		t.Parallel()
		names, err := fs.Glob(migrationFS, "migrations/*.sql")
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(names)
		if len(names) != 10 || names[9] != "migrations/"+epochMigration {
			t.Fatalf("migrations = %v, want %s as the tenth", names, epochMigration)
		}
		for _, name := range names {
			if strings.HasPrefix(filepath.Base(name), "0011") {
				t.Fatalf("%s is reserved for the binding migration", name)
			}
		}
		sqlText, err := migrationFS.ReadFile("migrations/" + epochMigration)
		if err != nil {
			t.Fatal(err)
		}
		upper := strings.ToUpper(string(sqlText))
		for _, banned := range []string{"OR REPLACE", "OR IGNORE", "INSERT INTO SERVICE_ASSIGNMENT_EPOCHS SELECT", "DEFAULT 1"} {
			if strings.Contains(upper, banned) {
				t.Errorf("migration contains %q", banned)
			}
		}
	})
	t.Run("independent mirrors agree", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		legacy, err := open(t.Context(), dir, migrations[:9])
		if err != nil {
			t.Fatal(err)
		}
		if err := legacy.Close(); err != nil {
			t.Fatal(err)
		}
		raw := rawOpen(t, filepath.Join(dir, DBName))
		plan, err := preview.PreviewPlan(t.Context(), raw)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(plan.Steps, []string{epochMigration}) {
			t.Fatalf("preview steps from schema9 = %v, want [%s]", plan.Steps, epochMigration)
		}
		backupVersion, err := strconv.Atoi(strings.TrimPrefix(filepath.Base(plan.BackupTo), DBName+".bak-migration-v"))
		if err != nil {
			t.Fatal(err)
		}
		j, err := Open(t.Context(), dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = j.Close() }()
		var stateSchema int
		if err := j.db.QueryRowContext(t.Context(), `SELECT schema_version FROM migration_state`).Scan(&stateSchema); err != nil {
			t.Fatal(err)
		}
		mirrors := map[string]int{
			"journal.SchemaVersion": SchemaVersion,
			"journal.migrations":    len(migrations),
			"migration_state":       stateSchema,
			"preview.steps":         9 + len(plan.Steps),
			"preview.backup_target": backupVersion,
		}
		if got := mirrorDisagreements(mirrors, 10); len(got) != 0 {
			t.Fatalf("mirrors disagree with schema 10: %v", got)
		}
		// Omit-one-mirror control: each lagging mirror alone is named.
		for name := range mirrors {
			mutant := maps.Clone(mirrors)
			mutant[name] = 9
			if got := mirrorDisagreements(mutant, 10); !slices.Equal(got, []string{name}) {
				t.Errorf("omitted mirror %s reported as %v", name, got)
			}
		}
	})
	t.Run("newer schema leaves bytes unchanged", func(t *testing.T) {
		t.Parallel()
		j, dir := openTemp(t)
		if _, err := j.db.ExecContext(t.Context(), `PRAGMA user_version=11`); err != nil {
			t.Fatal(err)
		}
		if err := j.Close(); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, DBName)
		before := fileSHA256(t, path)
		if newer, err := Open(t.Context(), dir); !errors.Is(err, ErrSchemaTooNew) {
			if newer != nil {
				_ = newer.Close()
			}
			t.Fatalf("schema 11 open = %v, want ErrSchemaTooNew", err)
		}
		if fileSHA256(t, path) != before {
			t.Fatal("newer database bytes changed")
		}
		if _, err := os.Stat(path + ".bak-v11"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("newer refusal created backup: %v", err)
		}
	})
}
