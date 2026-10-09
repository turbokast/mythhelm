package migrate_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	_ "modernc.org/sqlite" // raw fixture handles in this test

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/migrate"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// TestMigrateWritesRunInMutate // NFR-2 one-transaction-per-mutation scope
// (supervisor-migration design §8): every ledger-mutating write in
// internal/migrate runs inside control.Mutate. Structural: no sql.Open and
// no *sql.DB Exec (nor Begin/Prepare, the other write-capable handles)
// outside it. Allowed: tx.* calls inside the Mutate fn; read-only Query;
// the preview subpackage (this walk is top-level only — preview
// hermeticity is pinned by TestPreviewHermetic); Backup's single VACUUM
// INTO Exec (a file-creating copy, not a ledger mutation); and the
// read-only mode=ro integrity opens (Task 5 scratchpad note — Restore
// verifies its copies through them, mutating nothing).
func TestMigrateWritesRunInMutate(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	dir := filepath.Dir(file)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	mutateSites := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			recv, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			pos := fset.Position(call.Pos())
			switch {
			case recv.Name == "sql" && sel.Sel.Name == "Open":
				if !isReadOnlyOpen(call) {
					t.Errorf("%s: sql.Open outside the read-only integrity opens; ledger writes belong inside control.Mutate", pos)
				}
			case recv.Name == "tx":
				// tx.* calls inside the Mutate fn are legitimate.
			case recv.Name == "control" && sel.Sel.Name == "Mutate":
				mutateSites++
			case isWriteCapable(sel.Sel.Name):
				if !isVacuumInto(sel.Sel.Name, call) {
					t.Errorf("%s: *sql.DB %s outside control.Mutate; ledger writes belong inside it", pos, sel.Sel.Name)
				}
			}
			return true
		})
	}
	if mutateSites == 0 {
		t.Error("no control.Mutate call sites found; the scope pin is vacuous")
	}
}

// isWriteCapable reports the *sql.DB methods that can mutate the ledger:
// Exec plus the transaction/statement handles a direct write would use.
func isWriteCapable(sel string) bool {
	switch sel {
	case "Exec", "ExecContext", "Begin", "BeginTx", "Prepare", "PrepareContext":
		return true
	default:
		return false
	}
}

// isReadOnlyOpen reports a sql.Open whose DSN is provably read-only: built
// by readOnlyDSN or a literal carrying mode=ro.
func isReadOnlyOpen(call *ast.CallExpr) bool {
	if len(call.Args) < 2 {
		return false
	}
	dsn := call.Args[1]
	if fn, ok := dsn.(*ast.CallExpr); ok {
		if id, ok := fn.Fun.(*ast.Ident); ok && id.Name == "readOnlyDSN" {
			return true
		}
	}
	if lit, ok := dsn.(*ast.BasicLit); ok && strings.Contains(lit.Value, "mode=ro") {
		return true
	}
	return false
}

// isVacuumInto reports Backup's excluded file copy: an Exec whose query is
// the VACUUM INTO string (VACUUM INTO cannot run inside a transaction and
// mutates no ledger row).
func isVacuumInto(sel string, call *ast.CallExpr) bool {
	if sel != "Exec" && sel != "ExecContext" {
		return false
	}
	query := 0
	if sel == "ExecContext" {
		query = 1
	}
	if len(call.Args) <= query {
		return false
	}
	lit, ok := call.Args[query].(*ast.BasicLit)
	return ok && strings.Contains(lit.Value, "VACUUM INTO")
}

// TestIntegrityCheckOnBackupRestore // NFR-2 integrity checks (design §8):
// a corrupt-source backup and a corrupt-copy restore each abort with
// persistence_unavailable and write nothing.
func TestIntegrityCheckOnBackupRestore(t *testing.T) {
	t.Run("corrupt source backup aborts", func(t *testing.T) {
		dir, _ := fixtureWithV1(t)
		dbPath := filepath.Join(dir, journal.DBName)
		if err := os.WriteFile(dbPath, corruptCopy(t, dbPath), 0o600); err != nil {
			t.Fatal(err)
		}
		// The header survives the corruption, so Backup reaches its
		// source integrity_check rather than failing the version read.
		db := openRaw(t, dir)
		var version int
		if err := db.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version); err != nil {
			t.Fatalf("user_version of the corrupt source: %v", err)
		}
		if version != journal.SchemaVersion {
			t.Fatalf("user_version = %d, want %d", version, journal.SchemaVersion)
		}
		before := hashDir(t, dir)
		_, err := migrate.Backup(t.Context(), db, dir)
		if code := codeOf(t, err); code != v2contract.CodePersistenceUnavailable {
			t.Errorf("code = %q, want %q", code, v2contract.CodePersistenceUnavailable)
		}
		if cause := migrateCause(t, err); !strings.Contains(cause, "integrity_check") {
			t.Errorf("migrate/cause = %q, want it to name the integrity_check failure", cause)
		}
		if after := hashDir(t, dir); after != before {
			t.Error("the refused backup changed the state dir")
		}
		target := filepath.Join(dir, fmt.Sprintf("%s.bak-migration-v%d", journal.DBName, journal.SchemaVersion))
		if _, statErr := os.Stat(target); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("backup target stat = %v, want nothing written", statErr)
		}
	})

	t.Run("corrupt copy restore aborts", func(t *testing.T) {
		dir, _ := fixtureWithV1(t)
		info, err := migrate.Backup(t.Context(), openRaw(t, dir), dir)
		if err != nil {
			t.Fatalf("Backup: %v", err)
		}
		corrupt := corruptCopy(t, info.Path)
		if err := os.WriteFile(info.Path, corrupt, 0o600); err != nil {
			t.Fatal(err)
		}
		// The digest follows the corrupted bytes, so the restore reaches
		// its backup integrity_check rather than the digest refusal.
		sum := sha256.Sum256(corrupt)
		info.SHA256 = hex.EncodeToString(sum[:])
		restoreDir := t.TempDir()
		before := hashDir(t, restoreDir)
		err = migrate.Restore(t.Context(), info, restoreDir)
		if code := codeOf(t, err); code != v2contract.CodePersistenceUnavailable {
			t.Errorf("code = %q, want %q", code, v2contract.CodePersistenceUnavailable)
		}
		if cause := migrateCause(t, err); !strings.Contains(cause, "integrity_check") {
			t.Errorf("migrate/cause = %q, want it to name the integrity_check failure", cause)
		}
		if after := hashDir(t, restoreDir); after != before {
			t.Error("the refused restore changed the target dir")
		}
	})
}

// migrateCause returns the namespaced migrate/cause detail of a migrate
// error, failing the test when the error is not a valid ControlError.
func migrateCause(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var cerr *v2contract.ControlError
	if !errors.As(err, &cerr) {
		t.Fatalf("error is not a *v2contract.ControlError: %T %v", err, err)
	}
	if err := cerr.Validate(); err != nil {
		t.Fatalf("ControlError fails Validate: %v (%v)", err, cerr)
	}
	return cerr.Detail["migrate/cause"]
}

// corruptCopy returns the bytes of the SQLite file at path with one flipped
// byte past the first page: the header (and user_version) stays intact
// while PRAGMA integrity_check fails. It fails the test when no single
// flipped byte trips the check, so the fixture never silently passes.
func corruptCopy(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // G304: the test's own fixture file
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 100 || string(raw[:16]) != "SQLite format 3\x00" {
		t.Fatalf("%s is not a SQLite file of at least 100 bytes", path)
	}
	pageSize := int(raw[16])<<8 | int(raw[17])
	if pageSize == 1 {
		pageSize = 65536
	}
	for page := 1; page*pageSize < len(raw); page++ {
		for _, delta := range []int{64, 128, 512, 1024} {
			off := page*pageSize + delta
			if off >= len(raw) {
				continue
			}
			trial := make([]byte, len(raw))
			copy(trial, raw)
			trial[off] ^= 0xff
			if !integrityOK(t, trial) {
				return trial
			}
		}
	}
	t.Fatal("no single flipped byte past the first page trips integrity_check")
	return nil
}

// integrityOK reports whether file passes PRAGMA integrity_check through a
// read-only scratch open.
func integrityOK(t *testing.T, file []byte) bool {
	t.Helper()
	probe := filepath.Join(t.TempDir(), "probe.db")
	if err := os.WriteFile(probe, file, 0o600); err != nil { //nolint:gosec // G703: the test's own temp path
		t.Fatal(err)
	}
	p := filepath.ToSlash(probe)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	dsn := (&url.URL{Scheme: "file", Path: p, RawQuery: "mode=ro"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	rows, err := db.QueryContext(t.Context(), "PRAGMA integrity_check")
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return false
		}
		if line != "ok" {
			return false
		}
	}
	return rows.Err() == nil
}

// sqliteWALFixFloor is the shipped-engine WAL-reset fix floor (design §8,
// sourced from the synthesis SRC-16 row): SELECT sqlite_version() from the
// built binary must meet it, or a backport must be pinned in scratchpad.md.
var sqliteWALFixFloor = [3]int{3, 51, 3}

// TestSQLiteEngineHasWALFix // OQ-8: SELECT sqlite_version() from the built
// binary meets the WAL-reset fix floor. It records the OQ-8 verdict; the
// observed version is pinned in scratchpad.md by this task.
func TestSQLiteEngineHasWALFix(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go command not on PATH; cannot build cmd/mythhelm")
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	bin := filepath.Join(t.TempDir(), "mythhelm-oq8")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command(goBin, "build", "-o", bin, "./cmd/mythhelm")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command(bin, "version", "--format", "jsonl").Output()
	if err != nil {
		t.Fatalf("mythhelm version: %v", err)
	}
	var got struct {
		Type          string `json:"type"`
		SQLiteVersion string `json:"sqlite_version"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("version output is not a version object: %v: %q", err, out)
	}
	if got.Type != "version" || got.SQLiteVersion == "" {
		t.Fatalf("version object = %+v, want type version with a sqlite_version", got)
	}
	floor := sqliteWALFixFloor
	if !engineAtLeast(got.SQLiteVersion, floor) {
		t.Errorf("sqlite_version() = %s, want >= %d.%d.%d (WAL-reset fix floor; else pin a backport in scratchpad.md)",
			got.SQLiteVersion, floor[0], floor[1], floor[2])
	}
	t.Logf("OQ-8 verdict: shipped SQLite engine %s meets the %d.%d.%d floor",
		got.SQLiteVersion, floor[0], floor[1], floor[2])
}

// engineAtLeast parses a dotted-triple engine version and reports whether
// it meets the floor. Anything unparseable fails closed: an unknown engine
// never passes the fix gate.
func engineAtLeast(version string, floor [3]int) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	var got [3]int
	for i := range 3 {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return false
		}
		got[i] = n
	}
	for i := range 3 {
		if got[i] != floor[i] {
			return got[i] > floor[i]
		}
	}
	return true
}
