package migrate

// Drain tests never touch the real per-user instance lock or state
// directory: every test redirects XDG_RUNTIME_DIR (and MYTHHELM_HOME where
// it drives cli.Main) into temp dirs. t.Setenv forbids t.Parallel, so no
// test here is parallel.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/cli"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/v2contract"
	"github.com/turbokast/mythhelm/internal/workers"
)

// admissionDecision is the barest decision that reaches the migration
// guard: no run directory was admitted, so a run that passes the guard
// fails later at recording, proving the guard let it through.
func admissionDecision(stateDir string) admission.Decision {
	return admission.Decision{StateDir: stateDir}
}

// TestMain lets this package's tests spawn a child that exits at once, to
// mint a deterministically dead PID for quarantine fixtures.
func TestMain(m *testing.M) {
	if slices.Contains(os.Args, "--test-helper-true") {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// isolateInstanceLock redirects the per-user instance lock into a temp dir,
// so Drain's lock and the control package's parallel suite never meet.
func isolateInstanceLock(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
}

// openDrainDB migrates a fresh state database and returns Drain's handle.
func openDrainDB(t *testing.T, dir string) *sql.DB {
	t.Helper()
	db, err := control.OpenLedger(t.Context(), dir)
	if err != nil {
		t.Fatalf("OpenLedger: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func insertRun(t *testing.T, db *sql.DB, runID, state string) {
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

func insertAttempt(t *testing.T, db *sql.DB, attemptID, runID, tokenSHA string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO attempts
		(attempt_id, run_id, task_id, attempt_number, state, reason,
		 launch_token_sha256, worker_pid, worker_start_time,
		 native_pid, native_pgid, native_session_id, workspace_path, spool_offset)
		VALUES (?, ?, 'task_1', 1, 'executing', NULL, ?, NULL, NULL,
		 NULL, NULL, NULL, '/nowhere/work', 0)`,
		attemptID, runID, tokenSHA); err != nil {
		t.Fatalf("insert attempt %s: %v", attemptID, err)
	}
}

// writeIdentity stores id as the attempt's worker.json, the launch identity
// Drain adopts or quarantines.
func writeIdentity(t *testing.T, stateDir, runID, attemptID string, id workers.Identity) {
	t.Helper()
	dir := workers.AttemptDir(stateDir, runID, attemptID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir attempt dir: %v", err)
	}
	raw, err := json.Marshal(id)
	if err != nil {
		t.Fatalf("marshal identity: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worker.json"), raw, 0o600); err != nil {
		t.Fatalf("write worker.json: %v", err)
	}
}

// insertLaunchedEvent journals the attempt.launched observation Drain matches
// the identity file against, mirroring the supervisor's recorded shape.
func insertLaunchedEvent(t *testing.T, db *sql.DB, eventID, runID, attemptID string, pid int, start time.Time, runSeq int64) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"worker_pid":        pid,
		"worker_start_time": start.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("marshal launched payload: %v", err)
	}
	insertEvent(t, db, eventID, runID, attemptID, "attempt.launched", string(payload), runSeq)
}

// insertEvent journals one evidence event for the run.
func insertEvent(t *testing.T, db *sql.DB, eventID, runID, attemptID, typ, payload string, runSeq int64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO journal
		(event_id, schema_version, run_id, task_id, attempt_id, producer_id,
		 producer_sequence, run_sequence, generation, caused_by, observed_at, type, payload)
		VALUES (?, 1, ?, 'task_1', ?, ?, 1, ?, 1, NULL, ?, ?, ?)`,
		eventID, runID, attemptID, "prod_"+eventID, runSeq, now, typ, payload); err != nil {
		t.Fatalf("insert %s event: %v", typ, err)
	}
}

func setPhase(t *testing.T, db *sql.DB, phase Phase) {
	t.Helper()
	if _, err := db.Exec(`UPDATE migration_state SET phase = ? WHERE id = 1`, string(phase)); err != nil {
		t.Fatalf("set phase %s: %v", phase, err)
	}
}

// tokenSHA binds a worker.json launch token to its attempts row.
func tokenSHA(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// deadPID returns a PID no live process holds: a reaped helper child,
// polled until the process table agrees.
func deadPID(t *testing.T) int {
	t.Helper()
	child := exec.Command(os.Args[0], "--test-helper-true") //nolint:gosec // test binary with a fixed argv
	if err := child.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	pid := child.Process.Pid
	if err := child.Wait(); err != nil {
		t.Fatalf("wait helper: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := workers.ProcessStartTime(pid); errors.Is(err, workers.ErrNoProcess) {
			return pid
		}
		if time.Now().After(deadline) {
			t.Fatalf("helper pid %d still resolves", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// ledgerHash digests the schema and every row of every table, so any ledger
// write changes it (see the kept-violating case below).
func ledgerHash(t *testing.T, db *sql.DB) string {
	t.Helper()
	h := sha256.New()
	rows, err := db.Query(`SELECT type, name, sql FROM sqlite_master
		WHERE name NOT LIKE 'sqlite_%' ORDER BY type, name`)
	if err != nil {
		t.Fatalf("dump schema: %v", err)
	}
	var tables []string
	for rows.Next() {
		var typ, name string
		var sqlText sql.NullString
		if err := rows.Scan(&typ, &name, &sqlText); err != nil {
			_ = rows.Close()
			t.Fatalf("scan schema: %v", err)
		}
		_, _ = fmt.Fprintf(h, "%s/%s=%s\n", typ, name, sqlText.String)
		if typ == "table" {
			tables = append(tables, name)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		t.Fatalf("read schema: %v", err)
	}
	_ = rows.Close()
	for _, table := range tables {
		cols, err := db.Query(fmt.Sprintf("SELECT * FROM %q ORDER BY rowid", table))
		if err != nil {
			t.Fatalf("dump %s: %v", table, err)
		}
		for cols.Next() {
			n, err := cols.Columns()
			if err != nil {
				_ = cols.Close()
				t.Fatalf("column names %s: %v", table, err)
			}
			vals := make([]any, len(n))
			scan := make([]any, len(n))
			for i := range vals {
				scan[i] = &vals[i]
			}
			if err := cols.Scan(scan...); err != nil {
				_ = cols.Close()
				t.Fatalf("scan %s: %v", table, err)
			}
			for _, v := range vals {
				_, _ = fmt.Fprintf(h, "%T=%v|", v, v)
			}
			_, _ = fmt.Fprintln(h)
		}
		if err := cols.Err(); err != nil {
			_ = cols.Close()
			t.Fatalf("read %s: %v", table, err)
		}
		_ = cols.Close()
	}
	return hex.EncodeToString(h.Sum(nil))
}

// The matching kept-violating case is TestLedgerHashDetectsWrites, which
// mutates one row and watches the digest change.

// TestDrainRefusesWhileLockHeld // I05 (v2 §2), I18 (v2 §2): a run whose
// owner never releases past the deadline aborts the drain with
// ownership_unresolved, discards any partial outcome, and leaves both the
// instance lock and the migration lock free.
func TestDrainRefusesWhileLockHeld(t *testing.T) {
	isolateInstanceLock(t)
	dir := t.TempDir()
	db := openDrainDB(t, dir)

	insertRun(t, db, "run_adoptable", "executing")
	self, err := workers.ProcessStartTime(os.Getpid())
	if err != nil {
		t.Fatalf("own start time: %v", err)
	}
	insertAttempt(t, db, "att_adoptable", "run_adoptable", tokenSHA("token-adopt"))
	writeIdentity(t, dir, "run_adoptable", "att_adoptable", workers.Identity{
		SchemaVersion: 1, RunID: "run_adoptable", AttemptID: "att_adoptable",
		PID: os.Getpid(), StartTime: self, LaunchToken: "token-adopt",
	})
	insertLaunchedEvent(t, db, "ev_adoptable", "run_adoptable", "att_adoptable", os.Getpid(), self, 1)

	insertRun(t, db, "run_stuck", "executing")
	stuckDir := filepath.Join(dir, "runs", "run_stuck")
	if err := os.MkdirAll(stuckDir, 0o700); err != nil {
		t.Fatalf("mkdir stuck run dir: %v", err)
	}
	held, err := supervisor.AcquireOwner(stuckDir)
	if err != nil {
		t.Fatalf("hold stuck lock: %v", err)
	}
	t.Cleanup(held)

	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	got, err := Drain(ctx, db)
	if err == nil {
		t.Fatalf("Drain with a held lock = %+v, nil; want ownership_unresolved", got)
	}
	var ctrl *v2contract.ControlError
	if !errors.As(err, &ctrl) || ctrl.Code != v2contract.CodeOwnershipUnresolved {
		t.Fatalf("Drain error = %v; want code ownership_unresolved", err)
	}
	if verr := ctrl.Validate(); verr != nil {
		t.Errorf("refusal fails ControlError.Validate: %v", verr)
	}
	if !strings.Contains(ctrl.NextAction, "run_stuck") && ctrl.Object != "run_stuck" {
		t.Errorf("refusal names neither run in %q (object %q); want run_stuck", ctrl.NextAction, ctrl.Object)
	}
	if len(got.Adopted) != 0 || len(got.Drained) != 0 || len(got.Quarantined) != 0 {
		t.Fatalf("Drain report on failure = %+v; want zero, nothing half-adopted", got)
	}
	if release, err := control.AcquireInstance(dir); err != nil {
		t.Fatalf("instance lock after failed Drain: %v; want it free for migration", err)
	} else {
		release()
	}
	if release, err := supervisor.AcquireOwner(dir); err != nil {
		t.Fatalf("migration lock after failed Drain: %v; want it free", err)
	} else {
		release()
	}
}

// TestLedgerHashDetectsWrites is the kept violating case for the
// writes-nothing checks: one inserted row changes the digest, so a Drain
// that wrote could not pass TestDrainWritesNothing by accident.
func TestLedgerHashDetectsWrites(t *testing.T) {
	isolateInstanceLock(t)
	dir := t.TempDir()
	db := openDrainDB(t, dir)
	before := ledgerHash(t, db)
	insertRun(t, db, "run_probe", "completed")
	if after := ledgerHash(t, db); after == before {
		t.Fatalf("ledger hash unchanged after an insert; the digest cannot see writes")
	}
}

// TestDrainWritesNothing // I23 (v2 §5.1): a full quiesce — a finished run,
// a live worker, a dead worker — reports every outcome yet leaves the
// ledger bytes unchanged; quarantine envelopes persist post-backup.
func TestDrainWritesNothing(t *testing.T) {
	isolateInstanceLock(t)
	dir := t.TempDir()
	db := openDrainDB(t, dir)

	insertRun(t, db, "run_done", "completed")

	self, err := workers.ProcessStartTime(os.Getpid())
	if err != nil {
		t.Fatalf("own start time: %v", err)
	}
	insertRun(t, db, "run_live", "executing")
	insertAttempt(t, db, "att_live", "run_live", tokenSHA("token-live"))
	writeIdentity(t, dir, "run_live", "att_live", workers.Identity{
		SchemaVersion: 1, RunID: "run_live", AttemptID: "att_live",
		PID: os.Getpid(), StartTime: self, LaunchToken: "token-live",
	})
	insertLaunchedEvent(t, db, "ev_live", "run_live", "att_live", os.Getpid(), self, 1)

	deadStart := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	dead := deadPID(t)
	insertRun(t, db, "run_dead", "executing")
	insertAttempt(t, db, "att_dead", "run_dead", tokenSHA("token-dead"))
	writeIdentity(t, dir, "run_dead", "att_dead", workers.Identity{
		SchemaVersion: 1, RunID: "run_dead", AttemptID: "att_dead",
		PID: dead, StartTime: deadStart, LaunchToken: "token-dead",
	})
	insertLaunchedEvent(t, db, "ev_dead", "run_dead", "att_dead", dead, deadStart, 1)

	before := ledgerHash(t, db)
	got, err := Drain(t.Context(), db)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if !slices.Equal(got.Drained, []string{"run_done"}) {
		t.Errorf("Drained = %v; want [run_done]", got.Drained)
	}
	if !slices.Equal(got.Adopted, []string{"run_live"}) {
		t.Errorf("Adopted = %v; want [run_live]", got.Adopted)
	}
	if !slices.Equal(got.Quarantined, []string{"run_dead"}) {
		t.Errorf("Quarantined = %v; want [run_dead]", got.Quarantined)
	}
	if after := ledgerHash(t, db); after != before {
		t.Errorf("ledger hash changed across Drain; the drain wrote")
	}
	var owned int
	if err := db.QueryRow(`SELECT COUNT(*) FROM journal
		WHERE type = 'migration.quarantined' OR type = 'migration.imported'`).Scan(&owned); err != nil {
		t.Fatalf("count migration envelopes: %v", err)
	}
	if owned != 0 {
		t.Errorf("journal holds %d migration envelopes; Drain persists none", owned)
	}
}

// TestDrainQuarantinesDeadWorker // I12 (v2 §2): an active run whose worker
// is dead — or never verifiable — is reported quarantined with its launch
// identity and evidence left for post-backup persistence; no replacement
// launch spawns.
func TestDrainQuarantinesDeadWorker(t *testing.T) {
	isolateInstanceLock(t)
	dir := t.TempDir()
	db := openDrainDB(t, dir)

	deadStart := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	dead := deadPID(t)
	insertRun(t, db, "run_dead", "executing")
	insertAttempt(t, db, "att_dead", "run_dead", tokenSHA("token-dead"))
	writeIdentity(t, dir, "run_dead", "att_dead", workers.Identity{
		SchemaVersion: 1, RunID: "run_dead", AttemptID: "att_dead",
		PID: dead, StartTime: deadStart, LaunchToken: "token-dead",
	})
	insertLaunchedEvent(t, db, "ev_qdead", "run_dead", "att_dead", dead, deadStart, 1)
	insertEvent(t, db, "ev_qprog", "run_dead", "att_dead", "attempt.progress", `{"done":false}`, 2)
	identityBefore, err := os.ReadFile(filepath.Join(workers.AttemptDir(dir, "run_dead", "att_dead"), "worker.json"))
	if err != nil {
		t.Fatalf("read worker.json: %v", err)
	}

	insertRun(t, db, "run_noattempt", "executing")

	spawns := 0
	oldSpawn := spawnWorker
	spawnWorker = func(string, string, string, string, workers.Launch) (*os.Process, error) {
		spawns++
		return nil, errors.New("quarantine must not launch")
	}
	t.Cleanup(func() { spawnWorker = oldSpawn })

	got, err := Drain(t.Context(), db)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if !slices.Equal(got.Quarantined, []string{"run_dead", "run_noattempt"}) {
		t.Errorf("Quarantined = %v; want [run_dead run_noattempt]", got.Quarantined)
	}
	if len(got.Adopted) != 0 || len(got.Drained) != 0 {
		t.Errorf("Drain also reported adopted=%v drained=%v; want none", got.Adopted, got.Drained)
	}
	if spawns != 0 {
		t.Errorf("quarantine spawned %d launches; want none", spawns)
	}
	identityAfter, err := os.ReadFile(filepath.Join(workers.AttemptDir(dir, "run_dead", "att_dead"), "worker.json"))
	if err != nil {
		t.Fatalf("read worker.json after: %v", err)
	}
	if string(identityAfter) != string(identityBefore) {
		t.Errorf("worker.json changed across Drain; the launch identity must survive for post-backup persistence")
	}
	var evidence int
	if err := db.QueryRow(`SELECT COUNT(*) FROM journal WHERE run_id = 'run_dead'`).Scan(&evidence); err != nil {
		t.Fatalf("count evidence: %v", err)
	}
	if evidence != 2 {
		t.Errorf("evidence events for run_dead = %d; want 2", evidence)
	}
}

// TestDrainAdoptsSameLaunch // I12 (v2 §2), I18 (v2 §2): a live worker with
// a matching launch identity is adopted under that identity — never
// relaunched, keeping its pid — while a live pid with a mismatched
// identity quarantines: liveness alone is not identity.
func TestDrainAdoptsSameLaunch(t *testing.T) {
	isolateInstanceLock(t)
	dir := t.TempDir()
	db := openDrainDB(t, dir)

	self, err := workers.ProcessStartTime(os.Getpid())
	if err != nil {
		t.Fatalf("own start time: %v", err)
	}
	insertRun(t, db, "run_live", "executing")
	insertAttempt(t, db, "att_live", "run_live", tokenSHA("token-live"))
	writeIdentity(t, dir, "run_live", "att_live", workers.Identity{
		SchemaVersion: 1, RunID: "run_live", AttemptID: "att_live",
		PID: os.Getpid(), StartTime: self, LaunchToken: "token-live",
	})
	insertLaunchedEvent(t, db, "ev_alive", "run_live", "att_live", os.Getpid(), self, 1)
	identityBefore, err := os.ReadFile(filepath.Join(workers.AttemptDir(dir, "run_live", "att_live"), "worker.json"))
	if err != nil {
		t.Fatalf("read worker.json: %v", err)
	}

	insertRun(t, db, "run_forged", "executing")
	insertAttempt(t, db, "att_forged", "run_forged", tokenSHA("token-real"))
	writeIdentity(t, dir, "run_forged", "att_forged", workers.Identity{
		SchemaVersion: 1, RunID: "run_forged", AttemptID: "att_forged",
		PID: os.Getpid(), StartTime: self, LaunchToken: "token-forged",
	})
	insertLaunchedEvent(t, db, "ev_aforged", "run_forged", "att_forged", os.Getpid(), self, 1)

	attemptDirs := func() []string {
		entries, err := filepath.Glob(filepath.Join(dir, "runs", "*", "attempts", "*"))
		if err != nil {
			t.Fatalf("glob attempts: %v", err)
		}
		return entries
	}
	dirsBefore := attemptDirs()
	var attemptsBefore int
	if err := db.QueryRow(`SELECT COUNT(*) FROM attempts`).Scan(&attemptsBefore); err != nil {
		t.Fatalf("count attempts: %v", err)
	}

	spawns := 0
	oldSpawn := spawnWorker
	spawnWorker = func(string, string, string, string, workers.Launch) (*os.Process, error) {
		spawns++
		return nil, errors.New("adopt must not launch")
	}
	t.Cleanup(func() { spawnWorker = oldSpawn })

	got, err := Drain(t.Context(), db)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if !slices.Equal(got.Adopted, []string{"run_live"}) {
		t.Errorf("Adopted = %v; want [run_live]", got.Adopted)
	}
	if !slices.Equal(got.Quarantined, []string{"run_forged"}) {
		t.Errorf("Quarantined = %v; want [run_forged]", got.Quarantined)
	}
	if len(got.Drained) != 0 {
		t.Errorf("Drained = %v; want none", got.Drained)
	}
	if spawns != 0 {
		t.Errorf("adopt spawned %d launches; want none", spawns)
	}
	if !slices.Equal(attemptDirs(), dirsBefore) {
		t.Errorf("attempt dirs changed across Drain; adoption creates no attempt")
	}
	var attemptsAfter int
	if err := db.QueryRow(`SELECT COUNT(*) FROM attempts`).Scan(&attemptsAfter); err != nil {
		t.Fatalf("count attempts after: %v", err)
	}
	if attemptsAfter != attemptsBefore {
		t.Errorf("attempt rows %d -> %d; adoption writes no attempt", attemptsBefore, attemptsAfter)
	}
	identityAfter, err := os.ReadFile(filepath.Join(workers.AttemptDir(dir, "run_live", "att_live"), "worker.json"))
	if err != nil {
		t.Fatalf("read worker.json after: %v", err)
	}
	if string(identityAfter) != string(identityBefore) {
		t.Errorf("worker.json changed across Drain; the launch identity is reused, not rewritten")
	}
	again, err := workers.ProcessStartTime(os.Getpid())
	if err != nil || !again.Equal(self) {
		t.Errorf("adopted worker pid changed identity %v %v; the run keeps its pid", again, err)
	}
}

// TestDrainFinishingRunDrains // I06 (v2 §2): an owner that releases
// mid-drain lets the run quiesce normally — the wait ends in classification,
// not in a timeout. It is the kept opposite of TestDrainRefusesWhileLockHeld.
func TestDrainFinishingRunDrains(t *testing.T) {
	isolateInstanceLock(t)
	dir := t.TempDir()
	db := openDrainDB(t, dir)

	insertRun(t, db, "run_moving", "ready_for_review")
	runDir := filepath.Join(dir, "runs", "run_moving")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	held, err := supervisor.AcquireOwner(runDir)
	if err != nil {
		t.Fatalf("hold lock: %v", err)
	}
	// The holder is already holding when Drain starts, so the first probe
	// always waits; the sleep only delays the release, while the assertion
	// below waits on Drain's own return with a wide deadline margin.
	released := make(chan struct{})
	go func() {
		defer close(released)
		time.Sleep(200 * time.Millisecond)
		held()
	}()
	t.Cleanup(func() { <-released })

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	got, err := Drain(ctx, db)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if !slices.Equal(got.Drained, []string{"run_moving"}) {
		t.Errorf("Drained = %v; want [run_moving]", got.Drained)
	}
}

// TestRunRefusedPastPreviewed // I05 (v2 §2): starting a new run past the
// previewed phase — or while migration holds the state directory — is
// refused with ownership_unresolved; at not_started or previewed with the
// lock free, admission proceeds past the guard.
func TestRunRefusedPastPreviewed(t *testing.T) {
	isolateInstanceLock(t)
	tests := []struct {
		name      string
		phase     Phase
		holdLock  bool
		wantCode  string
		wantPhase bool
	}{
		{name: "drained refuses", phase: PhaseDrained, wantCode: "ownership_unresolved", wantPhase: true},
		{name: "imported refuses", phase: PhaseImported, wantCode: "ownership_unresolved", wantPhase: true},
		{name: "adopted refuses", phase: PhaseAdopted, wantCode: "ownership_unresolved", wantPhase: true},
		{name: "not started proceeds", phase: PhaseNotStarted},
		{name: "previewed proceeds", phase: PhasePreviewed},
		{name: "held migration lock refuses", phase: PhaseNotStarted, holdLock: true, wantCode: "ownership_unresolved"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			db := openDrainDB(t, dir)
			setPhase(t, db, tt.phase)
			if tt.holdLock {
				held, err := supervisor.AcquireOwner(dir)
				if err != nil {
					t.Fatalf("hold migration lock: %v", err)
				}
				t.Cleanup(held)
			}
			_, err := supervisor.Run(t.Context(),
				admissionDecision(dir), supervisor.Hooks{})
			if tt.wantCode == "" {
				// Past the guard, the bare decision fails later: no run
				// directory was admitted, so the run cannot be recorded.
				var blocked *admission.BlockedError
				if !errors.As(err, &blocked) || blocked.Code != "persistence_unavailable" {
					t.Fatalf("Run = %v; want to proceed past the guard to persistence_unavailable", err)
				}
				if strings.Contains(err.Error(), "ownership_unresolved") {
					t.Fatalf("Run = %v; want no ownership refusal", err)
				}
				return
			}
			if !errors.Is(err, supervisor.ErrOwnership) {
				t.Fatalf("Run = %v; want ownership refusal", err)
			}
			if !strings.Contains(err.Error(), tt.wantCode) {
				t.Errorf("Run error %q names no %s", err, tt.wantCode)
			}
			if tt.wantPhase && !strings.Contains(err.Error(), string(tt.phase)) {
				t.Errorf("Run error %q does not name phase %q", err, tt.phase)
			}
		})
	}
}

// TestRecoverRefusedPastPreviewed // I05 (v2 §2): recovery, the last owner
// site to drain, is refused with ownership_unresolved past previewed or
// while migration holds the state directory; otherwise it proceeds into
// reconciliation.
func TestRecoverRefusedPastPreviewed(t *testing.T) {
	isolateInstanceLock(t)
	tests := []struct {
		name     string
		phase    Phase
		holdLock bool
		refuse   bool
	}{
		{name: "drained refuses", phase: PhaseDrained, refuse: true},
		{name: "adopted refuses", phase: PhaseAdopted, refuse: true},
		{name: "not started proceeds", phase: PhaseNotStarted},
		{name: "held migration lock refuses", phase: PhaseNotStarted, holdLock: true, refuse: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			db := openDrainDB(t, dir)
			setPhase(t, db, tt.phase)
			insertRun(t, db, "run_rec", "executing")
			if err := os.MkdirAll(filepath.Join(dir, "runs", "run_rec"), 0o700); err != nil {
				t.Fatalf("mkdir run dir: %v", err)
			}
			if tt.holdLock {
				held, err := supervisor.AcquireOwner(dir)
				if err != nil {
					t.Fatalf("hold migration lock: %v", err)
				}
				t.Cleanup(held)
			}
			j, err := journal.Open(t.Context(), dir)
			if err != nil {
				t.Fatalf("open journal: %v", err)
			}
			t.Cleanup(func() { _ = j.Close() })
			_, err = supervisor.Recover(t.Context(), j, "run_rec")
			if !tt.refuse {
				// Past the guard, reconciliation fails on the bare
				// fixture: no attempt was ever recorded. That error
				// still wraps ErrOwnership but never names the v2 code.
				if err == nil || !errors.Is(err, supervisor.ErrOwnership) {
					t.Fatalf("Recover = %v; want to proceed past the guard into reconciliation", err)
				}
				if strings.Contains(err.Error(), "ownership_unresolved") {
					t.Fatalf("Recover = %v; want no ownership refusal", err)
				}
				return
			}
			if !errors.Is(err, supervisor.ErrOwnership) {
				t.Fatalf("Recover = %v; want ownership refusal", err)
			}
			if !strings.Contains(err.Error(), "ownership_unresolved") {
				t.Errorf("Recover error %q names no ownership_unresolved", err)
			}
			if !tt.holdLock && !strings.Contains(err.Error(), string(tt.phase)) {
				t.Errorf("Recover error %q does not name phase %q", err, tt.phase)
			}
		})
	}
}

// runApplyMain drives `mythhelm apply` against dir and reports its exit
// code with stderr.
func runApplyMain(t *testing.T, dir, runID, branch string) (int, string) {
	t.Helper()
	t.Setenv("MYTHHELM_HOME", dir)
	var stdout, stderr bytes.Buffer
	code := cli.Main([]string{"apply", runID, "--to-branch", branch},
		cli.Stdio{Out: &stdout, Err: &stderr})
	return code, stderr.String()
}

// TestApplyRefusedDuringMigration // I05 (v2 §2): applying a run past the
// previewed phase — or while migration holds the state directory — exits
// with ownership_unresolved naming the phase; otherwise apply proceeds
// into its own checks.
func TestApplyRefusedDuringMigration(t *testing.T) {
	isolateInstanceLock(t)
	tests := []struct {
		name     string
		phase    Phase
		holdLock bool
		refuse   bool
	}{
		{name: "drained refuses", phase: PhaseDrained, refuse: true},
		{name: "imported refuses", phase: PhaseImported, refuse: true},
		{name: "adopted refuses", phase: PhaseAdopted, refuse: true},
		{name: "not started proceeds", phase: PhaseNotStarted},
		{name: "previewed proceeds", phase: PhasePreviewed},
		{name: "held migration lock refuses", phase: PhaseNotStarted, holdLock: true, refuse: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			db := openDrainDB(t, dir)
			setPhase(t, db, tt.phase)
			insertRun(t, db, "run_apply1", "created")
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
