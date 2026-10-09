package cli

// Migration CLI tests (supervisor-migration task 6): `migrate --preview`
// hermeticity and read-only behaviour, --apply orchestration, quarantine
// persistence, backup reuse and resume. Fixtures are true v1 databases —
// 0001_init.sql executed on a raw handle with user_version 1 — so apply
// tests also cover the production migrate-on-open path.
//
// Every test redirects XDG_RUNTIME_DIR (the per-user instance lock) and
// MYTHHELM_HOME into temp dirs; t.Setenv forbids t.Parallel throughout.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite" // raw fixture handles in these tests

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/migrate"
	"github.com/turbokast/mythhelm/internal/migrate/preview"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// isolateMigLock redirects the per-user instance lock into a temp dir,
// so Drain/Apply and the control suite never share the live lock.
func isolateMigLock(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
}

// useMigHome points MYTHHELM_HOME at dir for one CLI invocation.
func useMigHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("MYTHHELM_HOME", dir)
}

// openMigRaw opens a read-write raw handle on the state database in dir
// for fixtures and assertions. Callers close every handle before Restore
// (a live WAL refuses the copy-back).
func openMigRaw(t *testing.T, dir string) *sql.DB {
	t.Helper()
	p := filepath.ToSlash(filepath.Join(dir, journal.DBName))
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

// seedV1 creates a true v1 state database in a fresh temp dir: 0001 only,
// user_version 1, no v2 tables. Every apply test starts here, so journal
// migration runs exactly as in production. The dir is symlink-resolved
// (macOS /var → /private/var, Windows 8.3 short names): SQLite reports
// the resolved path, so every comparison uses it.
func seedV1(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "internal", "journal", "migrations", "0001_init.sql"))
	if err != nil {
		t.Fatalf("read 0001_init.sql: %v", err)
	}
	db := openMigRaw(t, dir)
	if _, err := db.Exec(string(raw)); err != nil {
		t.Fatalf("exec 0001_init.sql: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	return dir
}

// insertMigRun inserts a v1 runs row; state must be a v1 run word (or a
// deliberately corrupt one for failure fixtures).
func insertMigRun(t *testing.T, db *sql.DB, runID, state string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO runs
		(run_id, state, reason, adapter_id, source_repo, source_branch, base_rev,
		 task_sha256, billing_posture, execution_profile, created_at, updated_at)
		VALUES (?, ?, NULL, 'builtin/fake', '/nowhere/src', NULL, NULL,
		 'abc123', 'subscription-declared', 'restricted', ?, ?)`,
		runID, state, now, now); err != nil {
		t.Fatalf("insert run %s: %v", runID, err)
	}
}

// insertMigAttempt inserts a v1 attempts row with a valid attempt state.
func insertMigAttempt(t *testing.T, db *sql.DB, attemptID, runID, taskID string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO attempts
		(attempt_id, run_id, task_id, attempt_number, state, reason,
		 launch_token_sha256, worker_pid, worker_start_time,
		 native_pid, native_pgid, native_session_id, workspace_path, spool_offset)
		VALUES (?, ?, ?, 1, 'running', NULL, 'toksha', NULL, NULL,
		 NULL, NULL, NULL, '/nowhere/work', 0)`,
		attemptID, runID, taskID); err != nil {
		t.Fatalf("insert attempt %s: %v", attemptID, err)
	}
}

// insertMigJournalV1 appends one hand-rolled schema_version-1 journal row
// plus its producer advance: journal.Append would migrate the fixture,
// so v1 seeds write the rows directly.
func insertMigJournalV1(t *testing.T, db *sql.DB, eventID, runID, producer string, prodSeq, runSeq int64) {
	t.Helper()
	observed := time.Date(2026, 10, 1, 0, 0, int(runSeq), 0, time.UTC).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO journal
		(event_id, schema_version, run_id, task_id, attempt_id, producer_id,
		 producer_sequence, run_sequence, generation, caused_by, observed_at, type, payload)
		VALUES (?, 1, ?, NULL, NULL, ?, ?, ?, 0, NULL, ?, 'decision', '{"choice":"a"}')`,
		eventID, runID, producer, prodSeq, runSeq, observed); err != nil {
		t.Fatalf("insert journal %s: %v", eventID, err)
	}
	if _, err := db.Exec(`INSERT INTO producers (producer_id, last_sequence, generation) VALUES (?, ?, 0)
		ON CONFLICT (producer_id) DO UPDATE SET last_sequence = excluded.last_sequence`,
		producer, prodSeq); err != nil {
		t.Fatalf("advance producer %s: %v", producer, err)
	}
}

// hashStateData hashes every file under dir except SQLite's -shm/-wal
// sidecars, which any WAL reader may create. Preview must leave this map
// identical: no user byte changes, no new files.
func hashStateData(t *testing.T, dir string) map[string]string {
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

func requireSameState(t *testing.T, dir string, before map[string]string) {
	t.Helper()
	after := hashStateData(t, dir)
	if len(after) != len(before) {
		t.Fatalf("state file set changed: before %v, after %v", keys(before), keys(after))
	}
	for name, sum := range before {
		if after[name] != sum {
			t.Fatalf("state file %s changed under a read-only command", name)
		}
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// v1Projections snapshots the v1 journal content: every schema_version-1
// row in event_id order, the byte-identity Restore must preserve.
func v1Projections(t *testing.T, db *sql.DB) string {
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
		b.WriteString(strings.Join([]string{eventID, runID, producer,
			strconv.FormatInt(prodSeq, 10), strconv.FormatInt(runSeq, 10),
			strconv.FormatInt(generation, 10), typ, payload}, "\x00") + "\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func queryPhase(t *testing.T, db *sql.DB) (phase, backupPath string) {
	t.Helper()
	if err := db.QueryRow(`SELECT phase, backup_path FROM migration_state WHERE id = 1`).
		Scan(&phase, &backupPath); err != nil {
		t.Fatal(err)
	}
	return phase, backupPath
}

// TestPreviewWritesNothing // I13 (v2 §2): --preview exits 0 and the
// state bytes are unchanged; the output names every run, owner site,
// schema step and backup target, in plain and jsonl.
func TestPreviewWritesNothing(t *testing.T) {
	isolateMigLock(t)
	dir := seedV1(t)
	db := openMigRaw(t, dir)
	insertMigRun(t, db, "run_alpha", "completed")
	insertMigJournalV1(t, db, "evt-alpha-1", "run_alpha", "worker-alpha", 1, 1)
	insertMigRun(t, db, "run_beta", "executing")
	insertMigAttempt(t, db, "att_beta1", "run_beta", "task_beta")
	for _, format := range []string{"plain", "jsonl"} {
		before := hashStateData(t, dir)
		useMigHome(t, dir)
		args := []string{"migrate", "--preview"}
		if format == "jsonl" {
			args = append(args, "--format", "jsonl")
		}
		code, stdout, stderr := runMain(args...)
		if code != 0 {
			t.Fatalf("%s exit %d, stderr %q", format, code, stderr)
		}
		for _, want := range []string{
			"run_alpha", "run_beta", "task_beta", "subscription-declared",
			"internal/cli/apply.go", "internal/cli/tui.go",
			"internal/supervisor/pipeline.go", "internal/supervisor/recover.go",
			"0002_qualification.sql", "0007_v2contracts.sql",
			"mythhelm.db.bak-migration-v7",
		} {
			if !strings.Contains(stdout, want) {
				t.Errorf("%s output names no %q:\n%s", format, want, stdout)
			}
		}
		if format == "jsonl" {
			var decoded struct {
				Type string `json:"type"`
				Runs []struct {
					RunID   string `json:"run_id"`
					TaskID  string `json:"task_id"`
					Posture string `json:"posture"`
				} `json:"runs"`
				Owners   []string `json:"owners"`
				Steps    []string `json:"steps"`
				BackupTo string   `json:"backup_to"`
			}
			if err := json.Unmarshal([]byte(stdout), &decoded); err != nil {
				t.Fatalf("jsonl output does not parse: %v\n%s", err, stdout)
			}
			if decoded.Type != "migrate.preview" || len(decoded.Runs) != 2 ||
				len(decoded.Owners) != 4 || len(decoded.Steps) != 6 {
				t.Errorf("jsonl preview shape wrong: %+v", decoded)
			}
			// The state dir compares decoded, never as a raw
			// substring: jsonl escapes Windows backslashes.
			if decoded.BackupTo != filepath.Join(dir, "mythhelm.db.bak-migration-v7") {
				t.Errorf("jsonl backup_to = %q; want under %q", decoded.BackupTo, dir)
			}
		} else {
			var target string
			for line := range strings.Lines(stdout) {
				if rest, ok := strings.CutPrefix(line, "backup target: "); ok {
					target = strings.TrimSpace(rest)
				}
			}
			if target != filepath.Join(dir, "mythhelm.db.bak-migration-v7") {
				t.Errorf("plain backup target = %q; want under %q", target, dir)
			}
		}
		requireSameState(t, dir, before)
		var version int
		if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 1 {
			t.Errorf("user_version = %d, %v; want 1 untouched", version, err)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM sqlite_master
			WHERE type = 'table' AND name = 'migration_state'`); n != 0 {
			t.Errorf("preview created migration_state")
		}
		if matches, _ := filepath.Glob(filepath.Join(dir, "mythhelm.db.bak-migration-*")); len(matches) != 0 {
			t.Errorf("preview wrote backup files %v", matches)
		}
	}
}

// TestPreviewHermetic // I13 (v2 §2): the preview planner imports
// neither internal/control nor network packages, and its output is
// byte-identical under the full environment and an emptied one.
func TestPreviewHermetic(t *testing.T) {
	isolateMigLock(t)
	root := repoRoot(t)
	cmd := exec.Command("go", "list", "-deps", "./internal/migrate/preview")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	for dep := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if dep == "net" || strings.HasPrefix(dep, "net/") ||
			strings.HasSuffix(dep, "/internal/control") {
			t.Errorf("preview depends on %s", dep)
		}
	}

	dir := seedV1(t)
	db := openMigRaw(t, dir)
	insertMigRun(t, db, "run_alpha", "completed")
	insertMigAttempt(t, db, "att_alpha1", "run_alpha", "task_alpha")
	// Decoy environment the plan must ignore.
	t.Setenv("HOME", "/nonexistent-home")
	t.Setenv("TZ", "Pacific/Kiritimati")
	t.Setenv("MYTHHELM_HOME", dir)
	useMigHome(t, dir)
	planFull, err := preview.PreviewPlan(context.Background(), db)
	if err != nil {
		t.Fatalf("PreviewPlan: %v", err)
	}
	rawFull, err := json.Marshal(planFull)
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Environ()
	os.Clearenv()
	t.Cleanup(func() {
		os.Clearenv()
		for _, kv := range saved {
			if name, value, ok := strings.Cut(kv, "="); ok {
				//nolint:usetesting // t.Setenv cannot run in Cleanup (the test has finished); restoring needs os.Setenv.
				_ = os.Setenv(name, value)
			}
		}
	})
	dbEmpty := openMigRaw(t, dir)
	planEmpty, err := preview.PreviewPlan(context.Background(), dbEmpty)
	if err != nil {
		t.Fatalf("PreviewPlan under empty env: %v", err)
	}
	rawEmpty, err := json.Marshal(planEmpty)
	if err != nil {
		t.Fatal(err)
	}
	if string(rawFull) != string(rawEmpty) {
		t.Errorf("plan differs under emptied environment:\n%s\n%s", rawFull, rawEmpty)
	}
}

// TestPreviewRefusesNewerSchema: previewing a database newer than the
// binary — by user_version or by the migration_state row — returns
// schema_too_new and writes nothing.
func TestPreviewRefusesNewerSchema(t *testing.T) {
	isolateMigLock(t)
	t.Run("user version", func(t *testing.T) {
		dir := seedV1(t)
		db := openMigRaw(t, dir)
		insertMigRun(t, db, "run_alpha", "completed")
		if _, err := db.Exec(`PRAGMA user_version = 999`); err != nil {
			t.Fatal(err)
		}
		before := hashStateData(t, dir)
		useMigHome(t, dir)
		code, _, stderr := runMain("migrate", "--preview")
		if code != 2 {
			t.Fatalf("exit %d; want 2 newer-schema refusal", code)
		}
		if !strings.Contains(stderr, "schema_too_new") {
			t.Errorf("stderr %q names no schema_too_new", stderr)
		}
		requireSameState(t, dir, before)
		_, err := preview.PreviewPlan(context.Background(), db)
		var ce *v2contract.ControlError
		if !errors.As(err, &ce) || ce.Code != v2contract.CodeSchemaTooNew {
			t.Fatalf("PreviewPlan = %v; want schema_too_new ControlError", err)
		}
		if err := ce.Validate(); err != nil {
			t.Errorf("ControlError invalid: %v", err)
		}
	})
	t.Run("migration_state", func(t *testing.T) {
		dir := seedV1(t)
		j, err := journal.Open(context.Background(), dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := j.Transact(context.Background(), func(tx *sql.Tx) error {
			_, err := tx.Exec(`UPDATE migration_state SET schema_version = 999 WHERE id = 1`)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if err := j.Close(); err != nil {
			t.Fatal(err)
		}
		before := hashStateData(t, dir)
		useMigHome(t, dir)
		code, _, stderr := runMain("migrate", "--preview")
		if code != 2 {
			t.Fatalf("exit %d; want 2 newer-schema refusal", code)
		}
		if !strings.Contains(stderr, "schema_too_new") {
			t.Errorf("stderr %q names no schema_too_new", stderr)
		}
		requireSameState(t, dir, before)
	})
}

// TestApplyWithoutYesPreviewsFirst: --apply without --yes prints the
// preview and writes nothing.
func TestApplyWithoutYesPreviewsFirst(t *testing.T) {
	isolateMigLock(t)
	dir := seedV1(t)
	db := openMigRaw(t, dir)
	insertMigRun(t, db, "run_alpha", "completed")
	before := hashStateData(t, dir)
	useMigHome(t, dir)
	code, stdout, stderr := runMain("migrate", "--apply")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q; want 0 preview", code, stderr)
	}
	for _, want := range []string{"migration preview", "run_alpha", "0007_v2contracts.sql"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout names no %q:\n%s", want, stdout)
		}
	}
	if !strings.Contains(stderr, "--yes") {
		t.Errorf("stderr %q does not ask for --yes", stderr)
	}
	requireSameState(t, dir, before)
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 1 {
		t.Errorf("user_version = %d, %v; want 1 untouched", version, err)
	}
}

// openBackupReadOnly opens a raw read-only handle on path for asserting
// backup contents without touching the live database.
func openBackupReadOnly(t *testing.T, path string) *sql.DB {
	t.Helper()
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	dsn := (&url.URL{Scheme: "file", Path: p, RawQuery: "mode=ro"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// closeHandles closes every test handle on dir. Restore requires them
// all closed: the last close checkpoints the WAL (the backup_test.go
// pattern — no extra checkpoint handle, which only adds file churn on
// Windows runners).
func closeHandles(dbs ...*sql.DB) {
	for _, db := range dbs {
		_ = db.Close()
	}
}

// dumpRestoreFailure logs the catalogue detail (hidden from the Error
// string) and the state dir contents, so a CI-only Restore failure
// names its cause instead of just its code.
func dumpRestoreFailure(t *testing.T, dir string, err error) {
	t.Helper()
	var ce *v2contract.ControlError
	if errors.As(err, &ce) {
		t.Logf("restore detail: code=%s owner=%s operation=%s next=%q detail=%v",
			ce.Code, ce.Owner, ce.OperationID, ce.NextAction, ce.Detail)
	}
	entries, listErr := os.ReadDir(dir)
	if listErr != nil {
		t.Logf("read dir: %v", listErr)
		return
	}
	for _, entry := range entries {
		info, infoErr := entry.Info()
		size := int64(-1)
		if infoErr == nil {
			size = info.Size()
		}
		t.Logf("dir entry: %s size=%d dir=%v", entry.Name(), size, entry.IsDir())
	}
}

// TestBackupHoldsDrainedWrites: a run completing during the drain quiesce
// lands in the backup — restore after the (deliberately failed) apply
// shows its writes — while quarantine and import envelopes land after
// the backup and are absent from it.
func TestBackupHoldsDrainedWrites(t *testing.T) {
	isolateMigLock(t)
	dir := seedV1(t)
	db := openMigRaw(t, dir)
	writer := openMigRaw(t, dir)
	insertMigRun(t, db, "run_a_done", "completed")
	insertMigJournalV1(t, db, "evt-a-1", "run_a_done", "worker-a", 1, 1)
	insertMigRun(t, db, "run_b_flight", "executing")
	insertMigRun(t, db, "run_z_bad", "bogus")
	for _, id := range []string{"run_a_done", "run_b_flight", "run_z_bad"} {
		if err := os.MkdirAll(filepath.Join(dir, "runs", id), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	useMigHome(t, dir)

	held, err := supervisor.AcquireOwner(filepath.Join(dir, "runs", "run_b_flight"))
	if err != nil {
		t.Fatalf("hold run lock: %v", err)
	}
	type outcome struct {
		code        int
		stdout, err string
	}
	done := make(chan outcome, 1)
	go func() {
		code, stdout, stderr := runMain("migrate", "--apply", "--yes")
		done <- outcome{code, stdout, stderr}
	}()
	// Let the drain reach its quiesce wait, then complete the in-flight
	// run through the second handle and release its lock. Either
	// interleaving leaves the completion committed before the backup.
	time.Sleep(500 * time.Millisecond)
	if _, err := writer.Exec(`UPDATE runs SET state = 'completed' WHERE run_id = 'run_b_flight'`); err != nil {
		t.Errorf("complete run_b_flight: %v", err)
	}
	insertMigJournalV1(t, writer, "evt-b-1", "run_b_flight", "worker-late", 1, 1)
	held()
	var res outcome
	select {
	case res = <-done:
	case <-time.After(2 * time.Minute):
		t.Fatal("apply did not finish after the lock released")
	}
	if res.code == 0 {
		t.Fatalf("apply exit 0; want failure on run_z_bad:\n%s", res.stdout)
	}
	if !strings.Contains(res.err, "invalid_contract") {
		t.Errorf("stderr %q names no invalid_contract", res.err)
	}
	phase, backupPath := queryPhase(t, db)
	if phase != string(migrate.PhaseDrained) {
		t.Errorf("phase = %q; want drained after the failed import", phase)
	}
	liveV1 := v1Projections(t, db)
	if !strings.Contains(liveV1, "evt-b-1") {
		t.Fatalf("live ledger lacks the drained completion:\n%s", liveV1)
	}

	backup := openBackupReadOnly(t, backupPath)
	var state string
	if err := backup.QueryRow(`SELECT state FROM runs WHERE run_id = 'run_b_flight'`).Scan(&state); err != nil || state != "completed" {
		t.Errorf("backup run_b_flight state = %q, %v; want completed", state, err)
	}
	if got := v1Projections(t, backup); got != liveV1 {
		t.Errorf("backup v1 projections differ from live:\n%s\n%s", got, liveV1)
	}
	if n := countRows(t, backup, `SELECT COUNT(*) FROM journal WHERE type LIKE 'migration.%'`); n != 0 {
		t.Errorf("backup holds %d migration envelopes; want none pre-persist", n)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM journal WHERE type LIKE 'migration.%'`); n == 0 {
		t.Errorf("live ledger holds no migration envelopes; want quarantine+import post-backup")
	}

	raw, err := os.ReadFile(backupPath + ".json") //nolint:gosec // G304: the sidecar the migration under test just wrote in its temp dir
	if err != nil {
		t.Fatal(err)
	}
	var info migrate.BackupInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	closeHandles(db, writer, backup)
	if err := migrate.Restore(context.Background(), info, dir); err != nil {
		dumpRestoreFailure(t, dir, err)
		t.Fatalf("Restore: %v", err)
	}
	restored := openMigRaw(t, dir)
	if got := v1Projections(t, restored); got != liveV1 {
		t.Errorf("restored v1 projections differ:\n%s\n%s", got, liveV1)
	}
}

// TestApplyPersistsQuarantinePostBackup: a quarantined run from the
// DrainReport gains its migration.quarantined envelope during apply, and
// the pre-persist backup holds none.
func TestApplyPersistsQuarantinePostBackup(t *testing.T) {
	isolateMigLock(t)
	dir := seedV1(t)
	db := openMigRaw(t, dir)
	insertMigRun(t, db, "run_ok", "completed")
	insertMigJournalV1(t, db, "evt-ok-1", "run_ok", "worker-ok", 1, 1)
	insertMigRun(t, db, "run_q", "executing")
	useMigHome(t, dir)
	code, stdout, stderr := runMain("migrate", "--apply", "--yes")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "quarantined run_q") {
		t.Errorf("receipt names no quarantined run_q:\n%s", stdout)
	}
	phase, backupPath := queryPhase(t, db)
	if phase != string(migrate.PhaseAdopted) {
		t.Errorf("phase = %q; want adopted", phase)
	}
	var payload string
	err := db.QueryRow(`SELECT payload FROM journal WHERE event_id = 'migration-quarantined-run_q'`).Scan(&payload)
	if err != nil {
		t.Fatalf("quarantine envelope: %v", err)
	}
	var decoded struct {
		RunID  string `json:"run_id"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.RunID != "run_q" || decoded.Reason == "" {
		t.Errorf("quarantine payload = %s; want run id and reason", payload)
	}
	backup := openBackupReadOnly(t, backupPath)
	if n := countRows(t, backup, `SELECT COUNT(*) FROM journal WHERE type LIKE 'migration.%'`); n != 0 {
		t.Errorf("backup holds %d migration envelopes; want none pre-persist", n)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM journal
		WHERE event_id = 'migration-imported-run_q'`); n != 1 {
		t.Errorf("quarantined run has %d import envelopes; want 1 (quarantine never skips import)", n)
	}
}

// TestApplyResumesAfterFailure: a failed mid-import apply resumes without
// duplicating imported runs — markers skip, envelopes dedupe, the backup
// is reused — and adopts.
func TestApplyResumesAfterFailure(t *testing.T) {
	isolateMigLock(t)
	dir := seedV1(t)
	db := openMigRaw(t, dir)
	insertMigRun(t, db, "run_a", "completed")
	insertMigJournalV1(t, db, "evt-a-1", "run_a", "worker-a", 1, 1)
	insertMigRun(t, db, "run_z", "bogus")
	useMigHome(t, dir)
	code, _, stderr := runMain("migrate", "--apply", "--yes")
	if code == 0 {
		t.Fatal("first apply exit 0; want failure on run_z")
	}
	if !strings.Contains(stderr, "invalid_contract") {
		t.Fatalf("stderr %q names no invalid_contract", stderr)
	}
	phase, backupPath := queryPhase(t, db)
	if phase != string(migrate.PhaseDrained) {
		t.Fatalf("phase = %q; want drained for the resume", phase)
	}
	backupSum := fileSHA(t, backupPath)
	if n := countRows(t, db, `SELECT COUNT(*) FROM task_revisions`); n != 1 {
		t.Fatalf("task_revisions = %d; want run_a imported before the failure", n)
	}
	if _, err := db.Exec(`UPDATE runs SET state = 'completed' WHERE run_id = 'run_z'`); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runMain("migrate", "--apply", "--yes")
	if code != 0 {
		t.Fatalf("resume exit %d, stderr %q", code, stderr)
	}
	phase, backupPath2 := queryPhase(t, db)
	if phase != string(migrate.PhaseAdopted) {
		t.Errorf("phase = %q; want adopted", phase)
	}
	if backupPath2 != backupPath || fileSHA(t, backupPath2) != backupSum {
		t.Errorf("backup was re-taken instead of reused")
	}
	for _, q := range []struct {
		want  int
		query string
	}{
		{2, `SELECT COUNT(*) FROM task_revisions`},
		{1, `SELECT COUNT(*) FROM task_revisions WHERE run_id = 'run_a'`},
		{1, `SELECT COUNT(*) FROM journal WHERE event_id = 'migration-imported-run_a'`},
		{1, `SELECT COUNT(*) FROM journal WHERE event_id = 'migration-imported-run_z'`},
	} {
		if n := countRows(t, db, q.query); n != q.want {
			t.Errorf("%s = %d; want %d (duplicated on resume)", q.query, n, q.want)
		}
	}
	if !strings.Contains(stdout, "migration adopted") {
		t.Errorf("receipt does not adopt:\n%s", stdout)
	}
}

// TestApplyRefusesWhenRecordedBackupVanished: a resume whose recorded
// pre-migration backup is gone refuses with invalid_contract instead of
// taking a fresh copy over partly migrated state.
func TestApplyRefusesWhenRecordedBackupVanished(t *testing.T) {
	isolateMigLock(t)
	dir := seedV1(t)
	db := openMigRaw(t, dir)
	insertMigRun(t, db, "run_a", "completed")
	insertMigRun(t, db, "run_z", "bogus")
	useMigHome(t, dir)
	if code, _, _ := runMain("migrate", "--apply", "--yes"); code == 0 {
		t.Fatal("first apply exit 0; want failure on run_z")
	}
	phase, backupPath := queryPhase(t, db)
	if phase != string(migrate.PhaseDrained) || backupPath == "" {
		t.Fatalf("phase = %q backup = %q; want drained with a recorded backup", phase, backupPath)
	}
	if err := os.Remove(backupPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(backupPath + ".json"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE runs SET state = 'completed' WHERE run_id = 'run_z'`); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runMain("migrate", "--apply", "--yes")
	if code == 0 {
		t.Fatal("resume exit 0; want refusal on the vanished backup")
	}
	if !strings.Contains(stderr, "invalid_contract") {
		t.Errorf("stderr %q names no invalid_contract", stderr)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "mythhelm.db.bak-migration-*")); len(matches) != 0 {
		t.Errorf("resume took a fresh backup %v over migrated state", matches)
	}
}

func fileSHA(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // G304: the backup the migration under test just wrote in its temp dir
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// TestApplyAdoptedIsIdempotent: applying an adopted ledger succeeds
// without duplicating anything.
func TestApplyAdoptedIsIdempotent(t *testing.T) {
	isolateMigLock(t)
	dir := seedV1(t)
	db := openMigRaw(t, dir)
	insertMigRun(t, db, "run_ok", "completed")
	useMigHome(t, dir)
	if code, _, stderr := runMain("migrate", "--apply", "--yes"); code != 0 {
		t.Fatalf("first apply exit %d, stderr %q", code, stderr)
	}
	if code, stdout, stderr := runMain("migrate", "--apply", "--yes"); code != 0 {
		t.Fatalf("second apply exit %d, stderr %q", code, stderr)
	} else if !strings.Contains(stdout, "migration adopted") {
		t.Errorf("second receipt does not adopt:\n%s", stdout)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM task_revisions`); n != 1 {
		t.Errorf("task_revisions = %d; want 1", n)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM journal WHERE type LIKE 'migration.%'`); n != 1 {
		t.Errorf("migration envelopes = %d; want 1", n)
	}
}

// TestApplyRequiresMigratedLedger: Apply fails fast on a database
// without migration_state instead of guessing at phases.
func TestApplyRequiresMigratedLedger(t *testing.T) {
	isolateMigLock(t)
	dir := seedV1(t)
	db := openMigRaw(t, dir)
	insertMigRun(t, db, "run_ok", "completed")
	_, err := migrate.Apply(context.Background(), db, preview.Plan{})
	var ce *v2contract.ControlError
	if !errors.As(err, &ce) || ce.Code != v2contract.CodeInvalidContract {
		t.Fatalf("Apply = %v; want invalid_contract fail-fast", err)
	}
	if err := ce.Validate(); err != nil {
		t.Errorf("ControlError invalid: %v", err)
	}
}

// TestPreviewMirrorsPinned: the preview's mirrored journal identities —
// database name, schema version, migration filenames — and its owner
// list track the sources they copy.
func TestPreviewMirrorsPinned(t *testing.T) {
	isolateMigLock(t)
	t.Run("backup target", func(t *testing.T) {
		dir := seedV1(t)
		db := openMigRaw(t, dir)
		plan, err := preview.PreviewPlan(context.Background(), db)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(dir, journal.DBName+".bak-migration-v"+strconv.Itoa(journal.SchemaVersion))
		if plan.BackupTo != want {
			t.Errorf("BackupTo = %q; want %q", plan.BackupTo, want)
		}
	})
	t.Run("schema steps", func(t *testing.T) {
		dir := seedV1(t)
		db := openMigRaw(t, dir)
		plan, err := preview.PreviewPlan(context.Background(), db)
		if err != nil {
			t.Fatal(err)
		}
		files, err := filepath.Glob(filepath.Join(repoRoot(t), "internal", "journal", "migrations", "*.sql"))
		if err != nil {
			t.Fatal(err)
		}
		var want []string
		for _, f := range files[1:] {
			want = append(want, filepath.Base(f))
		}
		if !slices.Equal(plan.Steps, want) {
			t.Errorf("Steps = %q; want %q", plan.Steps, want)
		}
	})
	t.Run("owner sites", func(t *testing.T) {
		if len(preview.Owners) != 4 {
			t.Fatalf("Owners = %q; want the 4 drain sites", preview.Owners)
		}
		for _, owner := range preview.Owners {
			raw, err := os.ReadFile(filepath.Join(repoRoot(t), owner)) //nolint:gosec // G304: the repo's own source tree under test
			if err != nil {
				t.Errorf("owner %s: %v", owner, err)
				continue
			}
			if !strings.Contains(string(raw), "AcquireOwner") {
				t.Errorf("owner %s holds no AcquireOwner site", owner)
			}
		}
	})
}

// TestMigrateUsage: the migrate flags parse strictly and help exits 0.
func TestMigrateUsage(t *testing.T) {
	isolateMigLock(t)
	useMigHome(t, t.TempDir())
	if code, _, _ := runMain("migrate", "-h"); code != 0 {
		t.Errorf("migrate -h exit %d; want 0", code)
	}
	for _, args := range [][]string{
		{"migrate", "--preview", "--apply"},
		{"migrate", "run_extra"},
		{"migrate", "--format", "yaml"},
	} {
		if code, _, _ := runMain(args...); code != 2 {
			t.Errorf("%v exit %d; want 2", args, code)
		}
	}
}

// runApplyMain drives `mythhelm apply` against dir and reports its exit
// code with stderr.
func runApplyMain(t *testing.T, dir, runID, branch string) (int, string) {
	t.Helper()
	t.Setenv("MYTHHELM_HOME", dir)
	code, _, stderr := runMain("apply", runID, "--to-branch", branch)
	return code, stderr
}

// TestApplyRefusedDuringMigration // I05 (v2 §2): applying a run past the
// previewed phase — or while migration holds the state directory — exits
// with ownership_unresolved naming the phase; otherwise apply proceeds
// into its own checks.
//
// Moved from Task 4's internal/migrate/drain_test.go by migration task
// 6, assertions unchanged: the migrate command made package cli import
// package migrate, so migrate's internal test files can no longer drive
// cli.Main (import cycle). Helpers are local copies of the drain test's.
func TestApplyRefusedDuringMigration(t *testing.T) {
	isolateMigLock(t)
	tests := []struct {
		name     string
		phase    migrate.Phase
		holdLock bool
		refuse   bool
	}{
		{name: "drained refuses", phase: migrate.PhaseDrained, refuse: true},
		{name: "imported refuses", phase: migrate.PhaseImported, refuse: true},
		{name: "adopted refuses", phase: migrate.PhaseAdopted, refuse: true},
		{name: "not started proceeds", phase: migrate.PhaseNotStarted},
		{name: "previewed proceeds", phase: migrate.PhasePreviewed},
		{name: "held migration lock refuses", phase: migrate.PhaseNotStarted, holdLock: true, refuse: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			db, err := control.OpenLedger(t.Context(), dir)
			if err != nil {
				t.Fatalf("OpenLedger: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if _, err := db.Exec(`UPDATE migration_state SET phase = ? WHERE id = 1`, string(tt.phase)); err != nil {
				t.Fatalf("set phase %s: %v", tt.phase, err)
			}
			insertMigRun(t, db, "run_apply1", "created")
			if err := os.MkdirAll(filepath.Join(dir, "runs", "run_apply1"), 0o700); err != nil {
				t.Fatalf("mkdir run dir: %v", err)
			}
			if tt.holdLock {
				held, err := supervisor.AcquireOwner(dir)
				if err != nil {
					t.Fatalf("hold migration lock: %v", err)
				}
				t.Cleanup(held)
			}
			code, stderr := runApplyMain(t, dir, "run_apply1", "feature/x")
			if !tt.refuse {
				// Past the guard, apply fails on the bare fixture: the
				// source checkout is not a repository, so the branch is
				// invalid there.
				if code != 2 {
					t.Fatalf("apply exit = %d (%s); want 2 past the guard", code, stderr)
				}
				if strings.Contains(stderr, "ownership_unresolved") {
					t.Fatalf("apply stderr %q; want no ownership refusal", stderr)
				}
				return
			}
			if code != 6 {
				t.Fatalf("apply exit = %d (%s); want 6 ownership_unresolved", code, stderr)
			}
			if !strings.Contains(stderr, "ownership_unresolved") {
				t.Errorf("apply stderr %q names no ownership_unresolved", stderr)
			}
			if !tt.holdLock && !strings.Contains(stderr, string(tt.phase)) {
				t.Errorf("apply stderr %q does not name phase %q", stderr, tt.phase)
			}
		})
	}
}
