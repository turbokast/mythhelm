package migrate

// These tests redirect the per-user lock; environment mutation forbids Parallel.
import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/migrate/preview"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/v2contract"
	"github.com/turbokast/mythhelm/internal/workers"
)

func requireOwnership(t *testing.T, err error) {
	t.Helper()
	var ce *v2contract.ControlError
	if !errors.As(err, &ce) || ce.Code != v2contract.CodeOwnershipUnresolved {
		t.Fatalf("error = %v; want typed ownership_unresolved", err)
	}
}

func TestMigrationHeldOwnerRefuses(t *testing.T) {
	for _, phase := range []Phase{PhaseNotStarted, PhasePreviewed, PhaseDrained, PhaseImported} {
		t.Run(string(phase), func(t *testing.T) {
			isolateInstanceLock(t)
			dir := t.TempDir()
			db := openDrainDB(t, dir)
			insertRun(t, db, "run_a_free", "completed")
			insertRun(t, db, "run_z_held", "completed")
			setPhase(t, db, phase)
			runDir := filepath.Join(dir, "runs", "run_z_held")
			if err := os.MkdirAll(runDir, 0700); err != nil {
				t.Fatal(err)
			}
			release, err := supervisor.AcquireOwner(runDir)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			before := ledgerHash(t, db)
			ctx, cancel := context.WithTimeout(t.Context(), 80*time.Millisecond)
			defer cancel()
			report, err := Apply(ctx, db, preview.Plan{})
			requireOwnership(t, err)
			if !reflect.DeepEqual(report, DrainReport{}) {
				t.Fatalf("partial report = %+v", report)
			}
			if got := ledgerHash(t, db); got != before {
				t.Fatalf("held-owner timeout mutated ledger at phase %s", phase)
			}
			if backups, _ := filepath.Glob(filepath.Join(dir, "*.bak*")); len(backups) != 0 {
				t.Fatalf("timeout created backup: %v", backups)
			}
		})
	}
}

func TestImportedResumeKeepsFence(t *testing.T) {
	for _, phase := range []Phase{PhaseImported, PhaseAdopted} {
		for _, foreign := range []bool{false, true} {
			name := string(phase) + "/same-root"
			if foreign {
				name = string(phase) + "/foreign-root"
			}
			t.Run(name, func(t *testing.T) {
				isolateInstanceLock(t)
				dir := t.TempDir()
				db := openDrainDB(t, dir)
				setPhase(t, db, phase)
				before := ledgerHash(t, db)
				held := dir
				if foreign {
					held = t.TempDir()
				}
				release, err := control.AcquireInstance(held)
				if err != nil {
					t.Fatal(err)
				}
				defer release()
				_, err = Apply(t.Context(), db, preview.Plan{})
				requireOwnership(t, err)
				if ledgerHash(t, db) != before {
					t.Fatal("resume changed phase under live instance")
				}
			})
		}
	}
}

func TestPrepareServiceRootClassification(t *testing.T) {
	for _, kind := range []string{"empty", "missing", "file", "zero-run-db", "partial", "orphan-run", "lock-only"} {
		t.Run(kind, func(t *testing.T) {
			isolateInstanceLock(t)
			dir := t.TempDir()
			switch kind {
			case "missing":
				dir = filepath.Join(dir, "new")
			case "file":
				if err := os.WriteFile(filepath.Join(dir, "sentinel"), []byte("unchanged"), 0600); err != nil {
					t.Fatal(err)
				}
			case "zero-run-db":
				openDrainDB(t, dir)
			case "partial":
				db := openDrainDB(t, dir)
				setPhase(t, db, PhaseImported)
			case "orphan-run":
				if err := os.MkdirAll(filepath.Join(dir, "runs", "run_orphan"), 0700); err != nil {
					t.Fatal(err)
				}
			case "lock-only":
				if err := os.WriteFile(filepath.Join(dir, supervisor.OwnerLockFile), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := PrepareServiceRoot(t.Context(), dir)
			fresh := kind == "empty" || kind == "missing" || kind == "lock-only"
			if !fresh {
				requireOwnership(t, err)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			db := openDrainDB(t, dir)
			phase, backup, err := readMigrationState(t.Context(), db)
			if err != nil {
				t.Fatal(err)
			}
			if phase != string(PhaseAdopted) || backup == "" {
				t.Fatalf("fresh phase=%s backup=%q; want adopted with backup", phase, backup)
			}
			if err := PrepareServiceRoot(t.Context(), dir); err != nil {
				t.Fatalf("adopted root: %v", err)
			}
		})
	}
}

func TestServiceOwnerChangedRoot(t *testing.T) {
	isolateInstanceLock(t)
	dir := t.TempDir()
	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE migration_state SET phase='adopted' WHERE id=1`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	lease, err := control.AcquireInstanceLease(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lease.Close() }()
	owner, err := supervisor.AcquireServiceOwner(t.Context(), j, lease, "run_identity")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close() }()
	moved := dir + "-moved"
	if err := os.Rename(dir, moved); err != nil {
		t.Skipf("platform cannot rename an open fixture: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
		if err := os.Rename(moved, dir); err != nil {
			t.Error(err)
		}
	}()
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	_, err = supervisor.RecoverOwned(t.Context(), j, owner, "run_identity", supervisor.Hooks{})
	if !errors.Is(err, supervisor.ErrOwnership) {
		t.Fatalf("replaced root reached recovery: %v; want ownership refusal", err)
	}
}

func TestServiceRootMigration(t *testing.T) {
	for _, schema := range []int{1, 99} {
		t.Run(fmt.Sprintf("spool-schema-%d", schema), func(t *testing.T) {
			isolateInstanceLock(t)
			dir := t.TempDir()
			db := openDrainDB(t, dir)
			start, err := workers.ProcessStartTime(os.Getpid())
			if err != nil {
				t.Fatal(err)
			}
			insertRun(t, db, "run_live", "executing")
			insertAttempt(t, db, "att_live", "run_live", tokenSHA("token-live"))
			if _, err := db.Exec(`UPDATE attempts SET state='running' WHERE attempt_id='att_live'`); err != nil {
				t.Fatal(err)
			}
			id := workers.Identity{SchemaVersion: 1, RunID: "run_live", AttemptID: "att_live", PID: os.Getpid(), StartTime: start, LaunchToken: "token-live"}
			writeIdentity(t, dir, "run_live", "att_live", id)
			insertLaunchedEvent(t, db, "evt_live", "run_live", "att_live", os.Getpid(), start, 1)
			attemptDir := workers.AttemptDir(dir, "run_live", "att_live")
			ev := journal.Event{SchemaVersion: schema, EventID: "evt_spool", RunID: "run_live", AttemptID: "att_live", ProducerID: "wrk_att_live", ProducerSequence: 1, Generation: 1, ObservedAt: time.Now().UTC(), Type: "attempt.progress", Payload: json.RawMessage(`{"text":"synthetic"}`)}
			spool, err := json.Marshal(ev)
			if err != nil {
				t.Fatal(err)
			}
			spool = append(spool, '\n')
			if err := os.WriteFile(filepath.Join(attemptDir, supervisor.SpoolFile), spool, 0600); err != nil {
				t.Fatal(err)
			}
			identityBefore, err := os.ReadFile(filepath.Join(attemptDir, "worker.json")) // #nosec G304 -- fixed identity file in this test's synthetic attempt directory
			if err != nil {
				t.Fatal(err)
			}
			// The actual production deadline is derived from this immutable
			// run_envelopes clock/ceiling (workers.Launch remains on stdin).
			clock := "2026-11-01T00:00:00Z"
			if _, err := db.Exec(`INSERT INTO run_envelopes (run_id,execution_seconds,repairs,replans,transport_retries,first_start_at,updated_at) VALUES ('run_live',120,1,1,1,?,?)`, clock, clock); err != nil {
				t.Fatal(err)
			}
			held, err := supervisor.AcquireOwner(filepath.Join(dir, "runs", "run_live"))
			if err != nil {
				t.Fatal(err)
			}
			// A lock-probe barrier proves migration owns the root while it waits for
			// legacy release; no sleep assumes the scheduler has run the migrator.
			done := make(chan struct{})
			var report DrainReport
			var applyErr error
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			go func() { defer close(done); report, applyErr = Apply(ctx, db, preview.Plan{}) }()
			deadline := time.NewTimer(3 * time.Second)
			defer deadline.Stop()
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for {
				release, err := supervisor.AcquireOwner(dir)
				if errors.Is(err, supervisor.ErrOwnerHeld) {
					break
				}
				if err != nil {
					held()
					t.Fatal(err)
				}
				release()
				select {
				case <-deadline.C:
					held()
					<-done
					t.Fatal("migration never acquired root")
				case <-done:
					held()
					t.Fatalf("migration returned before owner release: %v", applyErr)
				case <-ticker.C:
				}
			}
			if release, err := supervisor.AcquireOwner(dir); err == nil {
				release()
				held()
				<-done
				t.Fatal("new legacy admission passed migration root fence")
			}
			_, admissionErr := supervisor.Run(t.Context(), admissionDecision(dir), supervisor.Hooks{})
			if !errors.Is(admissionErr, supervisor.ErrOwnership) {
				held()
				<-done
				t.Fatalf("legacy admission passed migration fence: %v", admissionErr)
			}
			held()
			<-done
			if applyErr != nil {
				t.Fatal(applyErr)
			}
			if schema == 1 {
				if !reflect.DeepEqual(report.Adopted, []string{"run_live"}) {
					t.Fatalf("verified worker adoption=%+v", report)
				}
			} else if len(report.Adopted) != 0 || !reflect.DeepEqual(report.Quarantined, []string{"run_live"}) {
				t.Fatalf("unsupported spool was adopted: %+v", report)
			}
			after, err := os.ReadFile(filepath.Join(attemptDir, "worker.json")) // #nosec G304 -- fixed identity file in this test's synthetic attempt directory
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(identityBefore, after) {
				t.Fatal("migration rewrote live launch identity")
			}
			after, err = os.ReadFile(filepath.Join(attemptDir, supervisor.SpoolFile)) // #nosec G304 -- fixed spool in this test's synthetic attempt directory
			if err != nil || !bytes.Equal(spool, after) {
				t.Fatal("migration rewrote spool")
			}
			var firstStart string
			var executionSeconds int64
			if err := db.QueryRow(`SELECT first_start_at,execution_seconds FROM run_envelopes WHERE run_id='run_live'`).Scan(&firstStart, &executionSeconds); err != nil {
				t.Fatal(err)
			}
			if firstStart != clock || executionSeconds != 120 {
				t.Fatalf("migration extended original execution envelope: %s %d", firstStart, executionSeconds)
			}
			if _, err := os.Stat(filepath.Join(attemptDir, "stop.request")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("migration requested stop: %v", err)
			}
			var durableToken string
			if err := db.QueryRow(`SELECT launch_token_sha256 FROM attempts WHERE attempt_id='att_live'`).Scan(&durableToken); err != nil {
				t.Fatal(err)
			}
			if durableToken != tokenSHA("token-live") {
				t.Fatal("migration replaced durable launch identity")
			}
			var launchGeneration int64
			if err := db.QueryRow(`SELECT generation FROM journal WHERE event_id='evt_live'`).Scan(&launchGeneration); err != nil {
				t.Fatal(err)
			}
			if launchGeneration != 1 {
				t.Fatal("migration changed historical launch generation")
			}
			var n int
			if err := db.QueryRow(`SELECT COUNT(*) FROM attempts`).Scan(&n); err != nil || n != 1 {
				t.Fatalf("attempts=%d err=%v", n, err)
			}
			phase, backup, err := readMigrationState(t.Context(), db)
			if err != nil || phase != "adopted" || backup == "" {
				t.Fatalf("phase=%s backup=%s err=%v", phase, backup, err)
			}
		})
	}
}

func TestMigrationRootIdentityBarrier(t *testing.T) {
	for _, replace := range []string{"root", "database", "owner-lock"} {
		t.Run(replace, func(t *testing.T) {
			isolateInstanceLock(t)
			dir := t.TempDir()
			db := openDrainDB(t, dir)
			setPhase(t, db, PhaseImported)
			before := ledgerHash(t, db)
			afterDrain = func() {
				switch replace {
				case "root":
					moved := dir + "-moved"
					if err := os.Rename(dir, moved); err != nil {
						t.Skipf("open directory rename unavailable: %v", err)
					}
					t.Cleanup(func() { _ = os.RemoveAll(dir); _ = os.Rename(moved, dir) })
					if err := os.Mkdir(dir, 0700); err != nil {
						t.Fatal(err)
					}
				case "database":
					name := filepath.Join(dir, "mythhelm.db")
					if err := os.Rename(name, name+".moved"); err != nil {
						t.Skipf("open database rename unavailable: %v", err)
					}
					t.Cleanup(func() { _ = os.Remove(name); _ = os.Rename(name+".moved", name) })
					if err := os.WriteFile(name, []byte("replacement"), 0600); err != nil {
						t.Fatal(err)
					}
				case "owner-lock":
					name := filepath.Join(dir, supervisor.OwnerLockFile)
					if err := os.Rename(name, name+".moved"); err != nil {
						t.Skipf("open lock rename unavailable: %v", err)
					}
					t.Cleanup(func() { _ = os.Remove(name); _ = os.Rename(name+".moved", name) })
					if err := os.WriteFile(name, nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			t.Cleanup(func() { afterDrain = nil })
			_, err := Apply(t.Context(), db, preview.Plan{})
			requireOwnership(t, err)
			if ledgerHash(t, db) != before {
				t.Fatal("identity conflict mutated held database")
			}
		})
	}
}

func TestPrepareServiceRootUnsafeLockRefuses(t *testing.T) {
	for _, kind := range []string{"content", "symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			isolateInstanceLock(t)
			dir := t.TempDir()
			db := openDrainDB(t, dir)
			setPhase(t, db, PhaseAdopted)
			name := filepath.Join(dir, supervisor.OwnerLockFile)
			switch kind {
			case "content":
				if err := os.WriteFile(name, []byte("existing bytes"), 0600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(name, 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				target := filepath.Join(t.TempDir(), "target")
				if err := os.WriteFile(target, nil, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, name); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}
			before := ledgerHash(t, db)
			err := PrepareServiceRoot(t.Context(), dir)
			requireOwnership(t, err)
			if ledgerHash(t, db) != before {
				t.Fatal("unsafe artefact refusal changed adopted state")
			}
		})
	}
}

// The imported shortcut retains the original backup and all import evidence,
// but now owns the same full writer fence as first-time migration.
func TestImportedResumePreservesProvenance(t *testing.T) {
	isolateInstanceLock(t)
	dir := t.TempDir()
	db := openDrainDB(t, dir)
	insertRun(t, db, "run_imported", "completed")
	if _, err := Apply(t.Context(), db, preview.Plan{}); err != nil {
		t.Fatal(err)
	}
	_, backup, err := readMigrationState(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := hashFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM journal WHERE type='migration.imported'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("seed import events=%d", count)
	}
	setPhase(t, db, PhaseImported)
	called := false
	afterDrain = func() {
		called = true
		for _, path := range []string{dir, filepath.Join(dir, "runs", "run_imported")} {
			unlock, err := supervisor.AcquireOwner(path)
			if err == nil {
				unlock()
				t.Errorf("imported resume exposed legacy writer lock %s", path)
			} else if !errors.Is(err, supervisor.ErrOwnerHeld) {
				t.Error(err)
			}
		}
		unlock, err := control.AcquireInstance(dir)
		if err == nil {
			unlock()
			t.Error("imported resume exposed service instance lock")
		} else if !errors.Is(err, control.ErrInstanceHeld) {
			t.Error(err)
		}
	}
	t.Cleanup(func() { afterDrain = nil })
	if _, err := Apply(t.Context(), db, preview.Plan{}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("imported path skipped quiescence barrier")
	}
	phase, nextBackup, err := readMigrationState(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	nextDigest, err := hashFile(nextBackup)
	if err != nil {
		t.Fatal(err)
	}
	if phase != "adopted" || backup != nextBackup || digest != nextDigest {
		t.Fatalf("resume changed provenance: %s %s", phase, nextBackup)
	}
	var next int
	if err := db.QueryRow(`SELECT COUNT(*) FROM journal WHERE type='migration.imported'`).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if next != count {
		t.Fatalf("resume duplicated imports %d -> %d", count, next)
	}
}

// A schema-zero ledger makes a misplaced writable OpenLedger observable. The
// barrier replaces the root after read-only drain but before schema opening.
func TestApplyRootIdentityBeforeSchemaOpen(t *testing.T) {
	isolateInstanceLock(t)
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, dbName))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`PRAGMA user_version=0`); err != nil {
		t.Fatal(err)
	}
	moved := dir + "-moved"
	afterDrain = func() {
		if err := os.Rename(dir, moved); err != nil {
			t.Skipf("open root rename unavailable: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir); _ = os.Rename(moved, dir) })
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { afterDrain = nil })
	_, err = ApplyRoot(t.Context(), dir, preview.Plan{})
	requireOwnership(t, err)
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("changed root upgraded old database to %d", version)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("changed root was opened writable: %v", entries)
	}
}

func TestServiceOwnerPreMintIdentity(t *testing.T) {
	for _, kind := range []string{"old-journal-old-lease", "old-journal-new-lease", "new-journal-old-lease", "readonly-journal", "database-only"} {
		t.Run(kind, func(t *testing.T) {
			isolateInstanceLock(t)
			dir := t.TempDir()
			j, err := journal.Open(t.Context(), dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = j.Close() }()
			adopt := func(j *journal.Journal) {
				if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
					_, err := tx.Exec(`UPDATE migration_state SET phase='adopted' WHERE id=1`)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			adopt(j)
			lease, err := control.AcquireInstanceLease(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = lease.Close() }()
			selected := j
			if kind == "readonly-journal" {
				selected, err = journal.OpenReadOnly(t.Context(), dir)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = selected.Close() }()
			}
			if kind == "database-only" {
				for _, suffix := range []string{"", "-wal", "-shm"} {
					file := filepath.Join(dir, dbName) + suffix
					if err := os.Rename(file, file+".moved"); err != nil && !errors.Is(err, os.ErrNotExist) {
						t.Skipf("open file rename unavailable: %v", err)
					}
				}
			} else {
				moved := dir + "-moved"
				if err := os.Rename(dir, moved); err != nil {
					t.Skipf("open root rename unavailable: %v", err)
				}
				t.Cleanup(func() { _ = os.RemoveAll(dir); _ = os.Rename(moved, dir) })
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			replacement, err := journal.Open(t.Context(), dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = replacement.Close() }()
			adopt(replacement)
			if kind == "new-journal-old-lease" {
				selected = replacement
			}
			if kind == "old-journal-new-lease" {
				if err := lease.Close(); err != nil {
					t.Fatal(err)
				}
				lease, err = control.AcquireInstanceLease(dir)
				if err != nil {
					t.Fatal(err)
				}
			}
			owner, err := supervisor.AcquireServiceOwner(t.Context(), selected, lease, "run_identity")
			if owner != nil {
				_ = owner.Close()
			}
			if !errors.Is(err, supervisor.ErrOwnership) {
				t.Fatalf("stale provenance minted owner: %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, "runs", "run_identity")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("identity refusal created run state: %v", err)
			}
		})
	}
}

func TestSpoolNonregularRefuses(t *testing.T) {
	for _, kind := range []string{"directory", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			attempt := workers.AttemptDir(dir, "run_fifo", "att_fifo")
			if err := os.MkdirAll(attempt, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(attempt, supervisor.SpoolFile)
			if kind == "directory" {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				if supportedSpool(t.Context(), dir, "run_fifo", "att_fifo") {
					t.Fatal("directory accepted as spool")
				}
				return
			}
			if runtime.GOOS == "windows" {
				t.Skip("FIFO fixture requires Unix; directory refusal remains portable")
			}
			binary, err := exec.LookPath("mkfifo")
			if err != nil {
				t.Skip("FIFO fixture tool unavailable; directory refusal remains portable")
			}
			if err := exec.CommandContext(t.Context(), binary, path).Run(); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			defer cancel()
			done := make(chan bool, 1)
			go func() { done <- supportedSpool(ctx, dir, "run_fifo", "att_fifo") }()
			timer := time.NewTimer(150 * time.Millisecond)
			defer timer.Stop()
			select {
			case accepted := <-done:
				if accepted {
					t.Fatal("FIFO accepted as spool")
				}
			case <-timer.C:
				// Own and join the blocked reader before failing the counterfactual.
				writer, err := os.OpenFile(path, os.O_RDWR, 0600) // #nosec G304 -- owned synthetic FIFO; unblocks and joins only this test's reader
				if err != nil {
					t.Fatal(err)
				}
				_ = writer.Close()
				<-done
				t.Fatal("FIFO open blocked past context deadline before file type validation")
			}
		})
	}
}

func TestServiceOwnerIdentityAliasControl(t *testing.T) {
	isolateInstanceLock(t)
	dir := t.TempDir()
	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE migration_state SET phase='adopted' WHERE id=1`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	lease, err := control.AcquireInstanceLease(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lease.Close() }()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, alias); err != nil {
		t.Skipf("same-inode alias fixture unavailable: %v", err)
	}
	readonly, err := journal.OpenReadOnly(t.Context(), alias)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = readonly.Close() }()
	if err := lease.CheckRootIdentity(t.Context(), alias); err != nil {
		t.Fatal(err)
	}
	if err := readonly.CheckStateIdentity(t.Context()); err != nil {
		t.Fatal(err)
	}
	owner, err := supervisor.AcquireServiceOwner(t.Context(), readonly, lease, "run_alias")
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "runs", "run_alias"))
	if err != nil || !info.IsDir() {
		t.Fatalf("valid capability control had no run lock effect: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := lease.CheckRootIdentity(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatalf("lease cancellation=%v", err)
	}
	if err := readonly.CheckStateIdentity(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("journal cancellation=%v", err)
	}
}
