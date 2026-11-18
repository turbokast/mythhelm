package migrate

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/migrate/preview"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

func countBackupRuns(t *testing.T, path string) int {
	t.Helper()
	copyDB, err := sql.Open("sqlite", readOnlyDSN(path))
	if err != nil {
		t.Fatalf("open backup %s: %v", path, err)
	}
	defer func() { _ = copyDB.Close() }()
	var n int
	if err := copyDB.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&n); err != nil {
		t.Fatalf("count backup runs: %v", err)
	}
	return n
}

// TestApplyRefusesStaleBackupAtDefaultPath // AC-7.4, I05: a backup sits at
// the default path but none is recorded for this apply (a restore reset
// migration_state, or a crash fell between the backup and its record). The
// copy predates run_after, so reusing it would lose that run on the next
// restore: Apply refuses, takes nothing over it and advances no phase.
func TestApplyRefusesStaleBackupAtDefaultPath(t *testing.T) {
	isolateInstanceLock(t)
	dir := t.TempDir()
	db := openDrainDB(t, dir)
	insertRun(t, db, "run_before", "completed")
	stale, err := Backup(t.Context(), db, dir)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	insertRun(t, db, "run_after", "completed")

	_, err = Apply(t.Context(), db, preview.Plan{})
	var ce *v2contract.ControlError
	if !errors.As(err, &ce) || ce.Code != v2contract.CodeInvalidContract {
		t.Fatalf("Apply over a stale backup = %v; want invalid_contract refusal", err)
	}
	if n := countBackupRuns(t, stale.Path); n != 1 {
		t.Errorf("stale backup rows = %d; want it untouched at 1", n)
	}
	phase, recorded, err := readMigrationState(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	if phase != string(PhasePreviewed) || recorded != "" {
		t.Errorf("after refusal phase=%q backup_path=%q; want previewed with no backup recorded", phase, recorded)
	}

	for _, f := range []string{stale.Path, stale.Path + ".json"} {
		if err := os.Remove(f); err != nil {
			t.Fatalf("move stale backup aside: %v", err)
		}
	}
	if _, err := Apply(t.Context(), db, preview.Plan{}); err != nil {
		t.Fatalf("Apply after moving the stale backup aside: %v", err)
	}
	if n := countBackupRuns(t, stale.Path); n != 2 {
		t.Errorf("fresh backup rows = %d; want both runs (2)", n)
	}
}

// TestApplyHoldsLocksAcrossDrainAndBackup // AC-7.3, I05, I18: between the
// drain and the backup every lock a legacy recover or apply needs is still
// held — the instance lock, the state-directory lock and the run's owner
// lock — so neither can pass its guard in that window.
func TestApplyHoldsLocksAcrossDrainAndBackup(t *testing.T) {
	isolateInstanceLock(t)
	dir := t.TempDir()
	db := openDrainDB(t, dir)
	insertRun(t, db, "run_existing", "completed")
	runDir := filepath.Join(dir, "runs", "run_existing")

	called := false
	afterDrain = func() {
		called = true
		if release, err := supervisor.AcquireOwner(runDir); err == nil {
			release()
			t.Error("run owner lock is free between drain and apply; a legacy recover or apply would pass")
		} else if !errors.Is(err, supervisor.ErrOwnerHeld) {
			t.Errorf("run owner lock: %v; want ErrOwnerHeld", err)
		}
		if release, err := supervisor.AcquireOwner(dir); err == nil {
			release()
			t.Error("state-directory lock is free between drain and apply")
		} else if !errors.Is(err, supervisor.ErrOwnerHeld) {
			t.Errorf("state-directory lock: %v; want ErrOwnerHeld", err)
		}
		if release, err := control.AcquireInstance(dir); err == nil {
			release()
			t.Error("instance lock is free between drain and apply")
		} else if !errors.Is(err, control.ErrInstanceHeld) {
			t.Errorf("instance lock: %v; want ErrInstanceHeld", err)
		}
	}
	t.Cleanup(func() { afterDrain = nil })

	if _, err := Apply(t.Context(), db, preview.Plan{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !called {
		t.Fatal("afterDrain never ran")
	}
	if release, err := supervisor.AcquireOwner(runDir); err != nil {
		t.Errorf("run owner lock after Apply: %v; want it released", err)
	} else {
		release()
	}
}

// TestApplyRefusesMissingRecordedBackup // AC-7.4: a resume whose recorded
// backup is gone cannot take a replacement, since the ledger may already
// hold migration rows: Apply refuses, creates nothing at the default path
// and leaves the phase where it was.
func TestApplyRefusesMissingRecordedBackup(t *testing.T) {
	isolateInstanceLock(t)
	dir := t.TempDir()
	db := openDrainDB(t, dir)
	insertRun(t, db, "run_existing", "completed")
	recorded := filepath.Join(dir, "gone.bak")
	if _, err := db.Exec(`UPDATE migration_state SET phase = 'drained', backup_path = ? WHERE id = 1`, recorded); err != nil {
		t.Fatalf("record backup: %v", err)
	}

	_, err := Apply(t.Context(), db, preview.Plan{})
	var ce *v2contract.ControlError
	if !errors.As(err, &ce) || ce.Code != v2contract.CodeInvalidContract {
		t.Fatalf("Apply with a missing recorded backup = %v; want invalid_contract", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, fmt.Sprintf("%s.bak-migration-v%d", journal.DBName, journal.SchemaVersion))); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("default-path backup stat = %v; want none taken", err)
	}
	phase, _, err := readMigrationState(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	if phase != string(PhaseDrained) {
		t.Errorf("phase = %q; want drained unchanged", phase)
	}
}

// I23 (v2 §5.2): backup publication must close legacy admission atomically.
func TestApplyBackupAndDrainPublicationAtomic(t *testing.T) {
	isolateInstanceLock(t)
	dir := t.TempDir()
	db := openDrainDB(t, dir)
	insertRun(t, db, "run_before", "completed")
	if _, err := db.Exec(`CREATE TRIGGER refuse_drained BEFORE UPDATE OF phase ON migration_state WHEN NEW.phase = 'drained' BEGIN SELECT RAISE(ABORT, 'synthetic drain publication failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := Apply(t.Context(), db, preview.Plan{})
	var ce *v2contract.ControlError
	if !errors.As(err, &ce) || ce.Code != v2contract.CodePersistenceUnavailable {
		t.Fatalf("publication failure = %v; want persistence_unavailable", err)
	}
	phase, recorded, err := readMigrationState(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	if phase != string(PhasePreviewed) || recorded != "" {
		t.Errorf("failed publication left phase=%q backup=%q; want previewed and no published backup", phase, recorded)
	}
	path := filepath.Join(dir, fmt.Sprintf("%s.bak-migration-v%d", journal.DBName, journal.SchemaVersion))
	before := backupPairDigests(t, path)
	if n := countBackupRuns(t, path); n != 1 {
		t.Fatalf("pre-write backup contains %d runs; want 1", n)
	}
	if _, err := db.Exec(`DROP TRIGGER refuse_drained`); err != nil {
		t.Fatal(err)
	}
	// Previewed still permits legacy writes after Apply releases its locks.
	insertRun(t, db, "run_after", "completed")
	_, err = Apply(t.Context(), db, preview.Plan{})
	if !errors.As(err, &ce) || ce.Code != v2contract.CodeInvalidContract {
		t.Errorf("resume after a later legacy write = %v; want stale backup refusal", err)
	}
	if got := backupPairDigests(t, path); got != before {
		t.Errorf("refused resume changed backup bytes: %v -> %v", before, got)
	}
}

func backupPairDigests(t *testing.T, path string) [2]string {
	t.Helper()
	var out [2]string
	for i, name := range []string{path, path + ".json"} {
		var err error
		out[i], err = hashFile(name)
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// I23 (v2 §5.2): old ambiguous records cannot certify a current backup.
func TestApplyRecordedBackupRequiresDrainedPhase(t *testing.T) {
	for _, phase := range []Phase{PhaseNotStarted, PhasePreviewed, PhaseDrained} {
		t.Run(string(phase), func(t *testing.T) {
			isolateInstanceLock(t)
			dir := t.TempDir()
			db := openDrainDB(t, dir)
			insertRun(t, db, "run_before", "completed")
			backup, err := Backup(t.Context(), db, dir)
			if err != nil {
				t.Fatal(err)
			}
			before := backupPairDigests(t, backup.Path)
			if _, err := db.Exec(`UPDATE migration_state SET phase = ?, backup_path = ?, build_version = 'original-build', started_at = 'original-start' WHERE id = 1`, string(phase), backup.Path); err != nil {
				t.Fatal(err)
			}
			if phase != PhaseDrained {
				insertRun(t, db, "run_after", "completed")
			}
			_, err = Apply(t.Context(), db, preview.Plan{})
			var ce *v2contract.ControlError
			if phase == PhaseDrained {
				if err != nil {
					t.Fatalf("safe drained resume: %v", err)
				}
			} else if !errors.As(err, &ce) || ce.Code != v2contract.CodeInvalidContract {
				t.Errorf("unsafe recorded backup resume = %v; want invalid_contract", err)
			}
			gotPhase, gotPath, err := readMigrationState(t.Context(), db)
			if err != nil {
				t.Fatal(err)
			}
			wantPhase := string(phase)
			if phase == PhaseDrained {
				wantPhase = string(PhaseAdopted)
			}
			if gotPhase != wantPhase || gotPath != backup.Path {
				t.Errorf("phase/path = %q/%q; want %q/%q", gotPhase, gotPath, wantPhase, backup.Path)
			}
			var build, started string
			if err := db.QueryRow(`SELECT build_version, started_at FROM migration_state WHERE id = 1`).Scan(&build, &started); err != nil {
				t.Fatal(err)
			}
			if build != "original-build" || started != "original-start" {
				t.Errorf("resume rewrote migration origin: build=%q started=%q", build, started)
			}
			if got := backupPairDigests(t, backup.Path); got != before {
				t.Errorf("resume changed backup bytes: %v -> %v", before, got)
			}
			var imports int
			if err := db.QueryRow(`SELECT COUNT(*) FROM journal WHERE type = 'migration.imported'`).Scan(&imports); err != nil {
				t.Fatal(err)
			}
			wantImports := 0
			if phase == PhaseDrained {
				wantImports = 1
			}
			if imports != wantImports {
				t.Errorf("imported envelopes=%d; want %d", imports, wantImports)
			}
		})
	}
}
