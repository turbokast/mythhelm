package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/migrate/preview"
	"github.com/turbokast/mythhelm/internal/statedir"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// migrationRoot pins physical directory/database identity across lock acquisition
// and quiescence. Paths alone cannot detect a rename-and-replace race.
type migrationRoot struct{ root, database, owner os.FileInfo }

func inspectMigrationRoot(dir string) (migrationRoot, error) {
	root, err := os.Lstat(dir)
	if err != nil {
		return migrationRoot{}, persistenceError("prepare-root", "inspecting root: "+err.Error())
	}
	if !root.IsDir() {
		return migrationRoot{}, migrationRequired("state root is not a real directory")
	}
	db, err := os.Lstat(filepath.Join(dir, dbName))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return migrationRoot{}, persistenceError("prepare-root", err.Error())
	}
	if err == nil && !db.Mode().IsRegular() {
		return migrationRoot{}, migrationRequired("database is not a regular file")
	}
	owner, err := os.Lstat(filepath.Join(dir, supervisor.OwnerLockFile))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return migrationRoot{}, persistenceError("prepare-root", err.Error())
	}
	if err == nil && (!owner.Mode().IsRegular() || owner.Size() != 0 || (runtime.GOOS != "windows" && owner.Mode().Perm()&0077 != 0)) {
		return migrationRoot{}, migrationRequired("unsafe owner.lock artefact")
	}
	return migrationRoot{root: root, database: db, owner: owner}, nil
}

func (r migrationRoot) verify(dir string) error {
	current, err := inspectMigrationRoot(dir)
	if err != nil {
		return err
	}
	if !os.SameFile(r.root, current.root) || !sameDatabase(r.database, current.database) || (r.owner != nil && !sameDatabase(r.owner, current.owner)) {
		return migrationRequired("root, database or owner-lock identity changed during migration")
	}
	return nil
}

// locked pins the owner.lock created by acquisition if none existed before.
// An already existing lock must still be the same physical file.
func (r migrationRoot) locked(dir string) (migrationRoot, error) {
	if err := r.verify(dir); err != nil {
		return migrationRoot{}, err
	}
	current, err := inspectMigrationRoot(dir)
	if err != nil {
		return migrationRoot{}, err
	}
	r.owner = current.owner
	return r, nil
}

func sameDatabase(a, b os.FileInfo) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return os.SameFile(a, b)
}

func migrationRequired(reason string) error {
	return applyError(v2contract.CodeOwnershipUnresolved, "prepare-service-root", "",
		"review mythhelm migrate --preview, then run mythhelm migrate --apply --yes with no service writer running",
		"migration_required: %s", reason)
}

// ApplyRoot is the explicit CLI migration boundary. It takes root authority and
// quiesces legacy runs before OpenLedger can upgrade a schema or create backups.
// Apply remains available to callers that already own their database lifecycle.
func ApplyRoot(ctx context.Context, stateDir string, plan preview.Plan) (DrainReport, error) {
	identity, err := inspectMigrationRoot(stateDir)
	if err != nil {
		return DrainReport{}, err
	}
	release, err := acquireMigrationLocks("apply", stateDir)
	if err != nil {
		return DrainReport{}, err
	}
	defer release()
	identity, err = identity.locked(stateDir)
	if err != nil {
		return DrainReport{}, err
	}
	if identity.database == nil {
		return DrainReport{}, migrationRequired("explicit migration needs an existing database")
	}
	old, err := sql.Open("sqlite", readOnlyDSN(filepath.Join(stateDir, dbName)))
	if err != nil {
		return DrainReport{}, persistenceError("apply", err.Error())
	}
	defer func() { _ = old.Close() }()
	report, releaseRuns, err := drainLocked(ctx, old, stateDir)
	if err != nil {
		return DrainReport{}, err
	}
	defer releaseRuns()
	if afterDrain != nil {
		afterDrain()
	}
	if err := identity.verify(stateDir); err != nil {
		return DrainReport{}, err
	}
	db, err := control.OpenLedger(ctx, stateDir)
	if err != nil {
		return DrainReport{}, err
	}
	defer func() { _ = db.Close() }()
	return applyQuiesced(ctx, db, plan, stateDir, identity, report)
}

// PrepareServiceRoot initialises only a fresh empty root. Existing unadopted
// work requires the explicit preview/apply command; attach never imports it.
// A safe empty owner.lock is an inert lock artefact, not existing work.
func PrepareServiceRoot(ctx context.Context, stateDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Lstat(stateDir); errors.Is(err, os.ErrNotExist) {
		if err := statedir.Ensure(stateDir); err != nil {
			return err
		}
	} else if err != nil {
		return persistenceError("prepare-service-root", err.Error())
	}
	identity, err := inspectMigrationRoot(stateDir)
	if err != nil {
		return err
	}
	fresh, err := emptyServiceRoot(stateDir)
	if err != nil {
		return err
	}
	release, err := acquireMigrationLocks("prepare-service-root", stateDir)
	if err != nil {
		return err
	}
	defer release()
	identity, err = identity.locked(stateDir)
	if err != nil {
		return err
	}
	if !fresh {
		if identity.database == nil {
			return migrationRequired("existing root contains work")
		}
		db, err := sql.Open("sqlite", readOnlyDSN(filepath.Join(stateDir, dbName)))
		if err != nil {
			return persistenceError("prepare-service-root", err.Error())
		}
		defer func() { _ = db.Close() }()
		phase, _, err := readMigrationState(ctx, db)
		if err != nil {
			return migrationRequired("existing root has no verified adopted phase")
		}
		if phase != string(PhaseAdopted) {
			return migrationRequired("existing root is not adopted")
		}
		_, releaseRuns, err := drainLocked(ctx, db, stateDir)
		if err != nil {
			return err
		}
		defer releaseRuns()
		return identity.verify(stateDir)
	}
	fresh, err = emptyServiceRoot(stateDir)
	if err != nil {
		return err
	}
	if !fresh {
		return migrationRequired("work appeared during initialization")
	}
	db, err := control.OpenLedger(ctx, stateDir)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	// The only permitted new file is our freshly created database; pin it before
	// proceeding through the same drain/backup/import/adopt body as explicit apply.
	created, err := inspectMigrationRoot(stateDir)
	if err != nil {
		return err
	}
	if !os.SameFile(identity.root, created.root) || !sameDatabase(identity.owner, created.owner) {
		return migrationRequired("root identity changed while opening new database")
	}
	identity = created
	report, releaseRuns, err := drainLocked(ctx, db, stateDir)
	if err != nil {
		return err
	}
	defer releaseRuns()
	if afterDrain != nil {
		afterDrain()
	}
	_, err = applyQuiesced(ctx, db, preview.Plan{}, stateDir, identity, report)
	return err
}

func emptyServiceRoot(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, persistenceError("prepare-service-root", err.Error())
	}
	for _, entry := range entries {
		if entry.Name() != supervisor.OwnerLockFile {
			return false, nil
		}
		info, err := entry.Info()
		if err != nil {
			return false, persistenceError("prepare-service-root", err.Error())
		}
		if !info.Mode().IsRegular() || info.Size() != 0 || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
			return false, migrationRequired(fmt.Sprintf("unsafe %s artefact", supervisor.OwnerLockFile))
		}
	}
	return true, nil
}
