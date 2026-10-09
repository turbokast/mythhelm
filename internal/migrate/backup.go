package migrate

import (
	"context"
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
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver for read-only integrity checks

	"github.com/turbokast/mythhelm/internal/buildinfo"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// dbName and supportedSchemaVersion mirror dbName and
// supportedSchemaVersion. They are copies rather than imports because
// internal/journal's tests import internal/migrate, so importing journal
// from this package would be an import cycle in test builds.
// TestBackupPinsMatchJournal fails when the mirrors drift; a schema bump
// must sweep this file too.
const (
	dbName                 = "mythhelm.db"
	supportedSchemaVersion = 7
)

// BackupInfo is the sidecar recorded beside every pre-migration backup
// (supervisor-migration design §5): schema and build identities, the backup
// digest and the creation timestamp.
type BackupInfo struct {
	Path          string `json:"path"`
	SchemaVersion int    `json:"schema_version"`
	BuildVersion  string `json:"build_version"`
	SHA256        string `json:"sha256"`
	CreatedAt     string `json:"created_at"`
}

// Backup writes a VACUUM INTO copy of the state database in dir plus a JSON
// sidecar, and writes nothing else. The copy lands at
// <dir>/mythhelm.db.bak-migration-v<N> with the sidecar beside it at
// <copy>.json; the name is distinct from the <path>.bak-v<N> copies Open
// writes during migration so the two never collide. VACUUM INTO cannot run
// inside a transaction, so Backup runs off-transaction like journal.go's
// backup; it mutates no ledger row, which is why Task 7's Mutate-scope
// assertion excludes this single Exec.
//
// Failure cases: persistence_unavailable (I/O, or the source/copy failing
// PRAGMA integrity_check); invalid_contract (the target already holds a
// backup — never overwritten).
func Backup(ctx context.Context, db *sql.DB, dir string) (BackupInfo, error) {
	const op = "backup"
	if db == nil {
		return BackupInfo{}, ctlErr(v2contract.CodeInvalidContract, op,
			"pass the open state database handle", errors.New("nil database handle"))
	}
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return BackupInfo{}, ctlErr(v2contract.CodePersistenceUnavailable, op,
			"retry once the database answers; see detail migrate/cause", err)
	}
	dst := filepath.Join(dir, fmt.Sprintf("%s.bak-migration-v%d", dbName, version))
	sidecar := dst + ".json"
	for _, path := range []string{dst, sidecar} {
		if _, err := os.Lstat(path); err == nil {
			return BackupInfo{}, ctlErr(v2contract.CodeInvalidContract, op,
				"move the existing backup aside; mythhelm never overwrites one",
				fmt.Errorf("%s already holds a backup", path))
		} else if !errors.Is(err, os.ErrNotExist) {
			return BackupInfo{}, ctlErr(v2contract.CodePersistenceUnavailable, op,
				"retry once the backup target answers; see detail migrate/cause", err)
		}
	}
	// Source deemed healthy before copying (design §8).
	if err := integrityCheck(ctx, db); err != nil {
		return BackupInfo{}, ctlErr(v2contract.CodePersistenceUnavailable, op,
			"repair or re-seed the source database; the backup was not taken",
			fmt.Errorf("source integrity_check: %w", err))
	}
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", dst); err != nil {
		// Nothing else creates dst, so a file there now is this call's
		// partial output; leaving it would block every retry.
		_ = os.Remove(dst)
		return BackupInfo{}, ctlErr(v2contract.CodePersistenceUnavailable, op,
			"retry once the backup target answers; see detail migrate/cause",
			fmt.Errorf("VACUUM INTO %s: %w", dst, err))
	}
	digest, err := hashFile(dst)
	if err != nil {
		_ = os.Remove(dst)
		return BackupInfo{}, ctlErr(v2contract.CodePersistenceUnavailable, op,
			"retry once the backup target answers; see detail migrate/cause", err)
	}
	if err := integrityCheckFile(ctx, dst); err != nil {
		_ = os.Remove(dst)
		return BackupInfo{}, ctlErr(v2contract.CodePersistenceUnavailable, op,
			"repair or re-seed the source database; the backup was not taken",
			fmt.Errorf("backup integrity_check: %w", err))
	}
	info := BackupInfo{
		Path:          dst,
		SchemaVersion: version,
		BuildVersion:  buildinfo.Get().Version,
		SHA256:        digest,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	raw, err := json.Marshal(info)
	if err != nil {
		_ = os.Remove(dst)
		return BackupInfo{}, ctlErr(v2contract.CodePersistenceUnavailable, op,
			"retry the backup; see detail migrate/cause", err)
	}
	// O_EXCL: a raced sidecar refuses rather than overwrites.
	f, err := os.OpenFile(sidecar, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: Backup builds the sidecar path from the state dir and schema version
	if err != nil {
		_ = os.Remove(dst)
		if errors.Is(err, os.ErrExist) {
			return BackupInfo{}, ctlErr(v2contract.CodeInvalidContract, op,
				"move the existing backup aside; mythhelm never overwrites one",
				fmt.Errorf("%s already holds a backup", sidecar))
		}
		return BackupInfo{}, ctlErr(v2contract.CodePersistenceUnavailable, op,
			"retry once the backup target answers; see detail migrate/cause", err)
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		_ = os.Remove(sidecar)
		_ = os.Remove(dst)
		return BackupInfo{}, ctlErr(v2contract.CodePersistenceUnavailable, op,
			"retry once the backup target answers; see detail migrate/cause", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(sidecar)
		_ = os.Remove(dst)
		return BackupInfo{}, ctlErr(v2contract.CodePersistenceUnavailable, op,
			"retry once the backup target answers; see detail migrate/cause", err)
	}
	return info, nil
}

// Restore copies the backup named by info into the state directory dir after
// verifying the sidecar digest and schema identity. Every refusal path
// returns before writing anything to dir (AT-42). On success the stale WAL
// sidecars are dropped (the backup is a clean single-file copy) and the
// restored file is re-opened read-only and integrity-checked before any
// writer touches it.
//
// Failure cases: schema_too_new (the sidecar or the backup file itself is
// newer than this binary — upgrade, nothing written); invalid_contract
// (digest mismatch, or the sidecar disagreeing with the backup file's own
// version — unrestorable, nothing written); persistence_unavailable (I/O,
// a corrupt backup/restored copy failing integrity_check, or a live WAL
// at the target that must checkpoint first).
func Restore(ctx context.Context, info BackupInfo, dir string) error {
	const op = "restore"
	if info.SchemaVersion > supportedSchemaVersion {
		return ctlErr(v2contract.CodeSchemaTooNew, op,
			fmt.Sprintf("upgrade mythhelm: backup schema is v%d, this binary supports up to v%d",
				info.SchemaVersion, supportedSchemaVersion),
			fmt.Errorf("backup schema v%d newer than supported v%d",
				info.SchemaVersion, supportedSchemaVersion))
	}
	if info.SchemaVersion <= 0 {
		return ctlErr(v2contract.CodeInvalidContract, op,
			"restore from a backup whose sidecar names its schema version",
			fmt.Errorf("backup names no schema version (schema_version %d)", info.SchemaVersion))
	}
	raw, err := os.ReadFile(info.Path)
	if err != nil {
		return ctlErr(v2contract.CodePersistenceUnavailable, op,
			"retry once the backup file answers; see detail migrate/cause", err)
	}
	sum := sha256.Sum256(raw)
	if info.SHA256 == "" || hex.EncodeToString(sum[:]) != info.SHA256 {
		return ctlErr(v2contract.CodeInvalidContract, op,
			"restore from an untampered backup; this copy fails its sidecar digest",
			fmt.Errorf("backup %s digest mismatch", info.Path))
	}
	// A corrupt copy with a matching digest is unrestorable: refuse before
	// writing, with the I/O code Task 7's integrity assertion expects.
	if err := integrityCheckFile(ctx, info.Path); err != nil {
		return ctlErr(v2contract.CodePersistenceUnavailable, op,
			"restore from an intact backup; this copy fails integrity_check",
			fmt.Errorf("backup integrity_check: %w", err))
	}
	// The sidecar digest covers the database bytes, not the sidecar
	// fields, so a lowered schema_version would sail through the checks
	// above. The backup file itself is authoritative for its version.
	actual, err := readUserVersion(ctx, info.Path)
	if err != nil {
		return ctlErr(v2contract.CodePersistenceUnavailable, op,
			"restore from an intact backup; see detail migrate/cause",
			fmt.Errorf("backup user_version: %w", err))
	}
	if actual > supportedSchemaVersion {
		return ctlErr(v2contract.CodeSchemaTooNew, op,
			fmt.Sprintf("upgrade mythhelm: backup schema is v%d, this binary supports up to v%d",
				actual, supportedSchemaVersion),
			fmt.Errorf("backup database is schema v%d, newer than supported v%d",
				actual, supportedSchemaVersion))
	}
	if actual != info.SchemaVersion {
		return ctlErr(v2contract.CodeInvalidContract, op,
			"restore from a backup whose sidecar matches its database",
			fmt.Errorf("backup database is schema v%d, sidecar says v%d",
				actual, info.SchemaVersion))
	}
	dst := filepath.Join(dir, dbName)
	// A non-empty WAL may hold committed frames not yet checkpointed into
	// the live database. Only an empty or absent WAL is safe to drop, so
	// a live one refuses before anything is staged.
	if st, err := os.Stat(dst + "-wal"); err == nil && st.Size() > 0 {
		return ctlErr(v2contract.CodePersistenceUnavailable, op,
			"close every handle on the state database (last close checkpoints the WAL) and retry",
			fmt.Errorf("%s holds %d uncheckpointed bytes", dst+"-wal", st.Size()))
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ctlErr(v2contract.CodePersistenceUnavailable, op,
			"retry once the state directory answers; see detail migrate/cause", err)
	}
	// Write-then-rename: the replacement is fully staged before the live
	// sidecars are touched, so a failed stage leaves the dir byte-identical.
	tmp, err := os.CreateTemp(dir, ".mythhelm.db.restore-*")
	if err != nil {
		return ctlErr(v2contract.CodePersistenceUnavailable, op,
			"retry once the state directory answers; see detail migrate/cause", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return ctlErr(v2contract.CodePersistenceUnavailable, op,
			"retry once the state directory answers; see detail migrate/cause", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return ctlErr(v2contract.CodePersistenceUnavailable, op,
			"retry once the state directory answers; see detail migrate/cause", err)
	}
	if err := tmp.Close(); err != nil {
		return ctlErr(v2contract.CodePersistenceUnavailable, op,
			"retry once the state directory answers; see detail migrate/cause", err)
	}
	// The replacement is staged: drop the provably empty sidecars, then swap.
	for _, sidecar := range []string{dst + "-wal", dst + "-shm"} {
		if err := os.Remove(sidecar); err != nil && !errors.Is(err, os.ErrNotExist) {
			return ctlErr(v2contract.CodePersistenceUnavailable, op,
				"retry once the state directory answers; see detail migrate/cause", err)
		}
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return ctlErr(v2contract.CodePersistenceUnavailable, op,
			"retry once the state directory answers; see detail migrate/cause", err)
	}
	// Re-open read-only first: the copy is verified before any writer opens it.
	if err := integrityCheckFile(ctx, dst); err != nil {
		return ctlErr(v2contract.CodePersistenceUnavailable, op,
			"restore from an intact backup; the copied file fails integrity_check",
			fmt.Errorf("restored integrity_check: %w", err))
	}
	return nil
}

// integrityCheck runs PRAGMA integrity_check over an open handle: read-only
// Query, never a ledger mutation.
func integrityCheck(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return err
		}
		if line != "ok" {
			_ = rows.Close()
			return fmt.Errorf("integrity_check: %s", line)
		}
	}
	return rows.Err()
}

// integrityCheckFile opens path read-only and runs PRAGMA integrity_check.
// The read-only open mutates nothing; Task 7's Mutate-scope assertion must
// exclude these integrity opens exactly as it excludes Backup's VACUUM INTO.
func integrityCheckFile(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", readOnlyDSN(path))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return integrityCheck(ctx, db)
}

// readUserVersion reads PRAGMA user_version through a read-only handle: the
// backup file is authoritative for its own schema version, never the sidecar.
func readUserVersion(ctx context.Context, path string) (int, error) {
	db, err := sql.Open("sqlite", readOnlyDSN(path))
	if err != nil {
		return 0, err
	}
	defer func() { _ = db.Close() }()
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("reading user_version of %s: %w", path, err)
	}
	return version, nil
}

// readOnlyDSN builds a mode=ro SQLite URI for path, mirroring journal's DSN.
func readOnlyDSN(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p, RawQuery: "mode=ro"}).String()
}

// hashFile returns the lowercase hex SHA-256 of the file at path.
func hashFile(path string) (string, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // G304: Backup's own target, built from the state dir and schema version
	if err != nil {
		return "", fmt.Errorf("hashing %s: %w", path, err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// ctlErr builds the catalogue ControlError for a migrate failure. Owner is
// the package, the operation is backup or restore, the disposition is the
// code default (never widened), and the cause sits under the namespaced
// migrate/cause detail key, so every error passes ControlError.Validate.
func ctlErr(code v2contract.Code, op, nextAction string, cause error) *v2contract.ControlError {
	detail := "unknown"
	if cause != nil {
		detail = cause.Error()
	}
	return &v2contract.ControlError{
		Code:        code,
		Owner:       "migrate",
		OperationID: op,
		Disposition: code.DefaultDisposition(),
		NextAction:  nextAction,
		Detail:      map[string]string{"migrate/cause": detail},
	}
}
