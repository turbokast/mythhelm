package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/adapters/fake"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/workers"
)

// openPinJournal opens a journal on the state directory the pinning test
// serves, so the lock root and the journal agree.
func openPinJournal(t *testing.T, stateDir string) *journal.Journal {
	t.Helper()
	j, err := journal.Open(t.Context(), stateDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return j
}

// holdLockFor takes the stream-2 instance lock for root in an isolated
// runtime dir and returns its boot generation.
func holdLockFor(t *testing.T, root string) int64 {
	t.Helper()
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	release, err := control.AcquireInstance(root)
	if err != nil {
		t.Fatalf("AcquireInstance: %v", err)
	}
	t.Cleanup(release)
	gen, err := control.CurrentGeneration(root)
	if err != nil {
		t.Fatalf("CurrentGeneration: %v", err)
	}
	return gen
}

func TestSpawnMintsAndJournalsNonce(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()
	wantGen := holdLockFor(t, stateDir)
	if wantGen != 1 {
		t.Fatalf("fresh lock generation = %d, want 1", wantGen)
	}
	j := openPinJournal(t, stateDir)
	prod := NewProducer(ids.New("sup"), 1)
	runID := newRun(t, j, prod)
	taskID := "task_1"

	pin := func(t *testing.T) (string, int64, string) {
		t.Helper()
		attemptID := newAttempt(t, j, prod, runID)
		nonce, gen, err := pinAdmission(ctx, j, prod, stateDir, runID, taskID, attemptID)
		if err != nil {
			t.Fatalf("pinAdmission: %v", err)
		}
		return nonce, gen, attemptID
	}

	nonce, gen, attemptID := pin(t)
	if nonce == "" {
		t.Fatal("pinAdmission minted an empty nonce")
	}
	if gen != wantGen {
		t.Errorf("pinned generation = %d, want the CurrentGeneration value %d", gen, wantGen)
	}
	version, digest, err := control.PinnedAdmission(ctx, j, runID, attemptID)
	if err != nil {
		t.Fatalf("PinnedAdmission: %v", err)
	}
	if version != "stop-ladder/v1" {
		t.Errorf("pinned ladder version = %q, want stop-ladder/v1", version)
	}
	if digest != workers.NonceDigest(nonce) {
		t.Error("the journaled digest is not the NonceDigest of the delivered raw nonce")
	}

	// A second spawn mints a different nonce under its own pin.
	nonce2, _, attemptID2 := pin(t)
	if nonce2 == nonce {
		t.Error("the second spawn minted the same nonce")
	}
	if _, digest2, err := control.PinnedAdmission(ctx, j, runID, attemptID2); err != nil || digest2 != workers.NonceDigest(nonce2) {
		t.Errorf("second pin reads back %q, %v; want the second nonce's digest", digest2, err)
	}

	// The worker the spawn site hands the pin to echoes the raw nonce into
	// worker.json.
	workdir := t.TempDir()
	lp, err := fake.New().Prepare(ctx, adapter.PrepareInput{Workdir: workdir, AttemptID: attemptID, Env: os.Environ(), Scenario: "happy"})
	if err != nil {
		t.Fatal(err)
	}
	probe, err := fake.New().Probe(ctx, adapter.ProbeInput{})
	if err != nil {
		t.Fatal(err)
	}
	prompt := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(prompt, []byte("# Task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	proc, err := workers.Spawn(os.Args[0], stateDir, runID, attemptID, workers.Launch{
		LaunchToken: "tok_" + attemptID, TaskID: taskID, AdapterID: fake.New().Descriptor().ID,
		Path: lp.Spec.Path, NativeSHA256: probe.SHA256, Args: lp.Spec.Args, Dir: lp.Spec.Dir,
		Env: lp.Spec.Env, PromptPath: prompt, StopLadder: lp.StopLadder,
		LadderVersion: "stop-ladder/v1", Nonce: nonce, Generation: gen,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() { _ = proc.Kill(); _, _ = proc.Wait() })
	dir := workers.AttemptDir(stateDir, runID, attemptID)
	deadline := time.Now().Add(30 * time.Second)
	for {
		id, err := workers.ReadIdentity(dir)
		if err == nil {
			if id.Nonce != nonce {
				t.Fatalf("worker.json nonce = %q, want the delivered raw nonce", id.Nonce)
			}
			break
		}
		if time.Now().After(deadline) {
			log, _ := os.ReadFile(filepath.Join(dir, "worker.log")) // #nosec G304 -- test-owned worker diagnostics
			t.Fatalf("worker identity unavailable: %v (log %q)", err, log)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestPinAdmissionWithoutLockPinsUnfenced(t *testing.T) {
	ctx := context.Background()
	// No supervisor holds a lock here: the runtime dir is empty.
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	stateDir := t.TempDir()
	j := openPinJournal(t, stateDir)
	prod := NewProducer(ids.New("sup"), 1)
	runID := newRun(t, j, prod)
	attemptID := newAttempt(t, j, prod, runID)

	nonce, gen, err := pinAdmission(ctx, j, prod, stateDir, runID, "task_1", attemptID)
	if err != nil {
		t.Fatalf("pinAdmission without a lock = %v; want the admission to proceed unfenced", err)
	}
	if gen != 0 {
		t.Errorf("unfenced generation = %d, want 0", gen)
	}
	version, digest, err := control.PinnedAdmission(ctx, j, runID, attemptID)
	if err != nil {
		t.Fatalf("PinnedAdmission: %v", err)
	}
	if version != "stop-ladder/v1" || digest != workers.NonceDigest(nonce) {
		t.Errorf("unfenced pin = (%q, digest ok %v), want the version and the nonce digest",
			version, digest == workers.NonceDigest(nonce))
	}
}

func TestPinAdmissionForeignLockPinsUnfenced(t *testing.T) {
	ctx := context.Background()
	// The lock serves another root, so this root pins unfenced.
	holdLockFor(t, t.TempDir())
	stateDir := t.TempDir()
	j := openPinJournal(t, stateDir)
	prod := NewProducer(ids.New("sup"), 1)
	runID := newRun(t, j, prod)
	attemptID := newAttempt(t, j, prod, runID)

	_, gen, err := pinAdmission(ctx, j, prod, stateDir, runID, "task_1", attemptID)
	if err != nil {
		t.Fatalf("pinAdmission under a foreign lock = %v; want the admission to proceed unfenced", err)
	}
	if gen != 0 {
		t.Errorf("unfenced generation = %d, want 0", gen)
	}
}

func TestPinAdmissionCorruptLockFailsAdmission(t *testing.T) {
	ctx := context.Background()
	// The lock file is present but unparseable: a torn read must never
	// look like an absent holder, so the admission fails instead of
	// pinning unfenced.
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	path, err := control.LockPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{torn"), 0o600); err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	j := openPinJournal(t, stateDir)
	prod := NewProducer(ids.New("sup"), 1)
	runID := newRun(t, j, prod)
	attemptID := newAttempt(t, j, prod, runID)

	if _, _, err := pinAdmission(ctx, j, prod, stateDir, runID, "task_1", attemptID); err == nil {
		t.Fatal("pinAdmission with a corrupt lock = nil, want the admission to fail")
	}
	if _, _, err := control.PinnedAdmission(ctx, j, runID, attemptID); err == nil {
		t.Error("a failed admission journaled attempt.admission_pinned, want no pin")
	}
}
