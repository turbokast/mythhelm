package supervisor

// The migration admission guard returns the state-directory lock held, so
// Run and Recover keep it until the run holds its own owner lock and no
// Drain can enumerate between the check and the admission. These tests pin
// that contract: the lock is held on success, the phase is read while it
// is held, and every refusal path releases it.

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/journal"
)

func openGuardJournal(t *testing.T) *journal.Journal {
	t.Helper()
	j, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return j
}

func setGuardPhase(t *testing.T, j *journal.Journal, phase string) {
	t.Helper()
	err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(),
			`UPDATE migration_state SET phase = ? WHERE id = 1`, phase)
		return err
	})
	if err != nil {
		t.Fatalf("set phase %q: %v", phase, err)
	}
}

// requireLockFree proves no guard result still holds dir's owner lock: a
// leaked hold would fail here with ErrOwnerHeld before any GC could hide
// it, since the check runs immediately after the guarded call returns.
func requireLockFree(t *testing.T, dir string) {
	t.Helper()
	release, err := AcquireOwner(dir)
	if err != nil {
		t.Fatalf("AcquireOwner(%s) = %v; want the migration lock released", dir, err)
	}
	release()
}

func requireLockHeld(t *testing.T, dir string) {
	t.Helper()
	release, err := AcquireOwner(dir)
	if !errors.Is(err, ErrOwnerHeld) {
		if err == nil {
			release()
		}
		t.Fatalf("AcquireOwner(%s) = %v; want ErrOwnerHeld while the guard holds the lock", dir, err)
	}
}

// TestCheckMigrationClearReturnsHeldLock: a clear guard hands the caller
// the state-directory lock still held; only the caller's release frees it.
func TestCheckMigrationClearReturnsHeldLock(t *testing.T) {
	for _, phase := range []string{"not_started", "previewed"} {
		t.Run(phase, func(t *testing.T) {
			j := openGuardJournal(t)
			setGuardPhase(t, j, phase)
			release, err := checkMigrationClear(t.Context(), j, "start a new run")
			if err != nil {
				t.Fatalf("checkMigrationClear = %v; want clear at %q", err, phase)
			}
			if release == nil {
				t.Fatal("checkMigrationClear returned a nil release; want the held lock")
			}
			requireLockHeld(t, j.StateDir())
			release()
			requireLockFree(t, j.StateDir())
		})
	}
}

// TestCheckMigrationClearRefusalsRelease: every refusal — a phase past
// previewed, or the lock already held — releases whatever the guard took
// and names ownership_unresolved.
func TestCheckMigrationClearRefusalsRelease(t *testing.T) {
	t.Run("past previewed", func(t *testing.T) {
		j := openGuardJournal(t)
		setGuardPhase(t, j, "drained")
		release, err := checkMigrationClear(t.Context(), j, "start a new run")
		if release != nil {
			release()
			t.Error("checkMigrationClear returned a release with an error; want nil")
		}
		if !errors.Is(err, ErrOwnership) {
			t.Fatalf("checkMigrationClear = %v; want ownership refusal", err)
		}
		if !strings.Contains(err.Error(), "ownership_unresolved") || !strings.Contains(err.Error(), "drained") {
			t.Errorf("refusal %q names no ownership_unresolved phase", err)
		}
		requireLockFree(t, j.StateDir())
	})

	t.Run("lock held", func(t *testing.T) {
		j := openGuardJournal(t)
		held, err := AcquireOwner(j.StateDir())
		if err != nil {
			t.Fatalf("hold migration lock: %v", err)
		}
		t.Cleanup(held)
		release, err := checkMigrationClear(t.Context(), j, "start a new run")
		if release != nil {
			release()
			t.Error("checkMigrationClear returned a release with an error; want nil")
		}
		if !errors.Is(err, ErrOwnership) {
			t.Fatalf("checkMigrationClear = %v; want ownership refusal", err)
		}
		if !strings.Contains(err.Error(), "ownership_unresolved") {
			t.Errorf("refusal %q names no ownership_unresolved", err)
		}
		// The guard never acquired, so it released nothing: the test's
		// hold is intact.
		requireLockHeld(t, j.StateDir())
	})
}
