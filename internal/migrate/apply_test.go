package migrate

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/turbokast/mythhelm/internal/control"
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
	if _, err := os.Lstat(filepath.Join(dir, "mythhelm.db.bak-migration-v7")); !errors.Is(err, os.ErrNotExist) {
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
