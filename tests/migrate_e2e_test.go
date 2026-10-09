// Package e2e exercises the final packaged mythhelm binary end to end
// for the v1 to v2 state migration (supervisor-migration task 6):
// fixture v1 dir → preview → apply → v2 assertions → restore →
// byte-identical v1 projections. TestMain builds cmd/mythhelm once;
// the test runs that binary against a temporary state directory.
package e2e

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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite" // raw fixture handles in this test

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/migrate"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// mythhelmBin is the packaged binary under test, built once by TestMain.
var mythhelmBin string

func TestMain(m *testing.M) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "e2e: cannot locate the test directory")
		os.Exit(1)
	}
	root := filepath.Dir(filepath.Dir(file))
	out, err := os.MkdirTemp("", "mythhelm-migrate-e2e-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: staging build dir:", err)
		os.Exit(1)
	}
	name := "mythhelm-migrate-e2e"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(out, name)
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/mythhelm")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: building mythhelm: %v\n%s", err, out)
		os.Exit(1)
	}
	mythhelmBin = bin
	code := m.Run()
	_ = os.RemoveAll(out)
	os.Exit(code)
}

func migrateRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	return filepath.Dir(filepath.Dir(file))
}

func runBin(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(mythhelmBin, args...)
	cmd.Env = append(os.Environ(),
		"MYTHHELM_HOME="+dir,
		"XDG_RUNTIME_DIR="+t.TempDir(),
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			code = exit.ExitCode()
		} else {
			t.Fatalf("run %v: %v", args, err)
		}
	}
	return stdout.String(), stderr.String(), code
}

func openE2ERaw(t *testing.T, path string) *sql.DB {
	t.Helper()
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	dsn := (&url.URL{Scheme: "file", Path: p, RawQuery: "_pragma=busy_timeout(5000)"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func hashE2EState(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || strings.HasSuffix(path, "-shm") || strings.HasSuffix(path, "-wal") {
			return nil
		}
		// #nosec G304 G122 -- hashing the test's own fixture tree, which holds no symlinks
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func requireSameE2EState(t *testing.T, dir string, before map[string]string) {
	t.Helper()
	after := hashE2EState(t, dir)
	if len(after) != len(before) {
		t.Fatalf("state file set changed: %d files before, %d after", len(before), len(after))
	}
	for name, sum := range before {
		if after[name] != sum {
			t.Fatalf("state file %s changed under the packaged preview", name)
		}
	}
}

func e2eV1Projections(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query(`SELECT event_id, run_id, producer_id, producer_sequence,
		run_sequence, generation, type, payload FROM journal
		WHERE schema_version = 1 ORDER BY event_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var b strings.Builder
	for rows.Next() {
		var eventID, runID, producer, typ, payload string
		var prodSeq, runSeq, generation int64
		if err := rows.Scan(&eventID, &runID, &producer, &prodSeq, &runSeq, &generation, &typ, &payload); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "%s\x00%s\x00%s\x00%d\x00%d\x00%d\x00%s\x00%s\n",
			eventID, runID, producer, prodSeq, runSeq, generation, typ, payload)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// e2eRestoreWithRetry runs Restore, retrying briefly on failure (see
// restoreWithRetry in internal/cli/migrate_test.go: Windows CI runners
// hold freshly closed files open for scanning, and the copy-back
// rename fails until they release it).
func e2eRestoreWithRetry(t *testing.T, info migrate.BackupInfo, dir string) {
	t.Helper()
	const attempts = 12
	var err error
	for i := range attempts {
		if err = migrate.Restore(context.Background(), info, dir); err == nil {
			return
		}
		t.Logf("restore attempt %d: %v", i+1, err)
		time.Sleep(300 * time.Millisecond)
	}
	if ce, ok := errors.AsType[*v2contract.ControlError](err); ok {
		t.Logf("restore detail: code=%s operation=%s next=%q detail=%v",
			ce.Code, ce.OperationID, ce.NextAction, ce.Detail)
	}
	t.Fatalf("Restore: %v", err)
}

// TestMigrateE2EPackagedBinary: fixture v1 dir → preview → apply → v2
// assertions → restore → byte-identical v1 projections, against the
// built binary.
func TestMigrateE2EPackagedBinary(t *testing.T) {
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	raw, err := os.ReadFile(filepath.Join(migrateRoot(t), "internal", "journal", "migrations", "0001_init.sql"))
	if err != nil {
		t.Fatalf("read 0001_init.sql: %v", err)
	}
	db := openE2ERaw(t, filepath.Join(dir, journal.DBName))
	if _, err := db.Exec(string(raw)); err != nil {
		t.Fatalf("exec 0001_init.sql: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, run := range [][2]string{{"run_e2e_done", "completed"}, {"run_e2e_live", "executing"}} {
		if _, err := db.Exec(`INSERT INTO runs
			(run_id, state, reason, adapter_id, source_repo, source_branch, base_rev,
			 task_sha256, billing_posture, execution_profile, created_at, updated_at)
			VALUES (?, ?, NULL, 'builtin/fake', '/nowhere/src', NULL, NULL,
			 'abc123', 'subscription-declared', 'restricted', ?, ?)`,
			run[0], run[1], now, now); err != nil {
			t.Fatalf("insert %s: %v", run[0], err)
		}
	}
	if _, err := db.Exec(`INSERT INTO attempts
		(attempt_id, run_id, task_id, attempt_number, state, reason,
		 launch_token_sha256, worker_pid, worker_start_time,
		 native_pid, native_pgid, native_session_id, workspace_path, spool_offset)
		VALUES ('att_e2e1', 'run_e2e_live', 'task_e2e', 1, 'running', NULL,
		 'toksha', NULL, NULL, NULL, NULL, NULL, '/nowhere/work', 0)`); err != nil {
		t.Fatal(err)
	}
	observed := time.Date(2026, 10, 1, 0, 0, 1, 0, time.UTC).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO journal
		(event_id, schema_version, run_id, task_id, attempt_id, producer_id,
		 producer_sequence, run_sequence, generation, caused_by, observed_at, type, payload)
		VALUES ('evt-e2e-1', 1, 'run_e2e_done', NULL, NULL, 'worker-e2e',
		 1, 1, 0, NULL, ?, 'decision', '{"choice":"a"}')`, observed); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO producers (producer_id, last_sequence, generation)
		VALUES ('worker-e2e', 1, 0)`); err != nil {
		t.Fatal(err)
	}

	before := hashE2EState(t, dir)
	stdout, stderr, code := runBin(t, dir, "migrate", "--preview")
	if code != 0 {
		t.Fatalf("preview exit %d, stderr %q", code, stderr)
	}
	for _, want := range []string{"run_e2e_done", "run_e2e_live", "task_e2e",
		"internal/supervisor/pipeline.go", "0007_v2contracts.sql", "mythhelm.db.bak-migration-v7"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("preview output names no %q:\n%s", want, stdout)
		}
	}
	requireSameE2EState(t, dir, before)

	stdout, stderr, code = runBin(t, dir, "migrate", "--apply", "--yes")
	if code != 0 {
		t.Fatalf("apply exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "migration adopted") || !strings.Contains(stdout, "quarantined run_e2e_live") {
		t.Errorf("receipt is not the adopted quarantine story:\n%s", stdout)
	}
	var phase, backupPath string
	if err := db.QueryRow(`SELECT phase, backup_path FROM migration_state WHERE id = 1`).
		Scan(&phase, &backupPath); err != nil {
		t.Fatal(err)
	}
	if phase != "adopted" {
		t.Errorf("phase = %q; want adopted", phase)
	}
	var revisions, imported, quarantined int
	if err := db.QueryRow(`SELECT COUNT(*) FROM task_revisions`).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM journal WHERE type = 'migration.imported'`).Scan(&imported); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM journal WHERE type = 'migration.quarantined'`).Scan(&quarantined); err != nil {
		t.Fatal(err)
	}
	if revisions != 2 || imported != 2 || quarantined != 1 {
		t.Errorf("revisions=%d imported=%d quarantined=%d; want 2/2/1", revisions, imported, quarantined)
	}
	liveV1 := e2eV1Projections(t, db)

	sidecar, err := os.ReadFile(backupPath + ".json") //nolint:gosec // G304: the sidecar the migration under test just wrote in its temp dir
	if err != nil {
		t.Fatal(err)
	}
	var info migrate.BackupInfo
	if err := json.Unmarshal(sidecar, &info); err != nil {
		t.Fatal(err)
	}
	// Restore requires every handle closed: the last close checkpoints
	// the WAL (no extra checkpoint handle — less file churn on Windows).
	_ = db.Close()
	e2eRestoreWithRetry(t, info, dir)
	restored := openE2ERaw(t, filepath.Join(dir, journal.DBName))
	if got := e2eV1Projections(t, restored); got != liveV1 {
		t.Errorf("restored v1 projections differ:\n%s\n%s", got, liveV1)
	}
}
