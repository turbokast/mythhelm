package control

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// leaseRuntime isolates the per-user instance lock in a private runtime dir.
// t.Setenv forbids t.Parallel.
func leaseRuntime(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	return t.TempDir()
}

func requireInstanceHeld(t *testing.T, dir string) {
	t.Helper()
	release, err := AcquireInstance(dir)
	if !errors.Is(err, ErrInstanceHeld) {
		if err == nil {
			release()
		}
		t.Fatalf("AcquireInstance while the lease holds = %v; want ErrInstanceHeld", err)
	}
}

func requireInstanceFree(t *testing.T, dir string) {
	t.Helper()
	release, err := AcquireInstance(dir)
	if err != nil {
		t.Fatalf("AcquireInstance after lease close = %v; want the OS lock released", err)
	}
	release()
}

// TestInstanceLeaseProof: only a successful OS acquisition mints a live lease,
// which carries the recorded boot generation and canonical root; a copy shares
// one lifetime, and forged or closed leases prove nothing (I18 (v2 §3)).
func TestInstanceLeaseProof(t *testing.T) {
	dir := leaseRuntime(t)
	lease, err := AcquireInstanceLease(dir)
	if err != nil {
		t.Fatalf("AcquireInstanceLease: %v", err)
	}
	if lease.Generation() <= 0 {
		t.Fatalf("Generation() = %d; want a positive boot generation", lease.Generation())
	}
	path, err := LockPath()
	if err != nil {
		t.Fatal(err)
	}
	meta, err := parseMetadata(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Generation > math.MaxInt64 {
		t.Fatalf("recorded boot generation %d overflows int64", meta.Generation)
		return
	}
	if lease.Generation() <= 0 || lease.Generation() != int64(meta.Generation) {
		t.Fatalf("Generation() = %d; want the recorded positive boot generation %d", lease.Generation(), meta.Generation)
	}
	if lease.Root() != canonicalRoot(dir) {
		t.Fatalf("Root() = %q; want canonical %q", lease.Root(), canonicalRoot(dir))
	}
	requireInstanceHeld(t, dir)
	if _, err := AcquireInstanceLease(dir); !errors.Is(err, ErrInstanceHeld) {
		t.Fatalf("second AcquireInstanceLease = %v; want ErrInstanceHeld, never a second proof", err)
	}

	for name, forged := range map[string]*InstanceLease{"zero": {}, "nil": nil} {
		if forged.Generation() != 0 || forged.Root() != "" {
			t.Errorf("%s lease reports generation %d root %q; want none", name, forged.Generation(), forged.Root())
		}
		if _, err := forged.Hold(); !errors.Is(err, ErrLeaseClosed) {
			t.Errorf("%s lease Hold = %v; want ErrLeaseClosed", name, err)
		}
		if err := forged.Close(); !errors.Is(err, ErrLeaseClosed) {
			t.Errorf("%s lease Close = %v; want ErrLeaseClosed", name, err)
		}
	}
	requireInstanceHeld(t, dir) // a forged Close released nothing

	copied := *lease
	if err := copied.Close(); err != nil {
		t.Fatalf("Close through a copy: %v", err)
	}
	if lease.Generation() != 0 {
		t.Fatalf("original reports generation %d after its copy closed; want 0", lease.Generation())
	}
	if _, err := lease.Hold(); !errors.Is(err, ErrLeaseClosed) {
		t.Fatalf("Hold after Close = %v; want ErrLeaseClosed", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("second Close = %v; want idempotent", err)
	}
	requireInstanceFree(t, dir)
}

// TestInstanceLeaseCloseWaitsForHolds: Close refuses new holds at once but
// keeps the OS lock until every outstanding hold is released, so a capability
// verified under a hold never outlives the instance lock.
func TestInstanceLeaseCloseWaitsForHolds(t *testing.T) {
	dir := leaseRuntime(t)
	lease, err := AcquireInstanceLease(dir)
	if err != nil {
		t.Fatalf("AcquireInstanceLease: %v", err)
	}
	release, err := lease.Hold()
	if err != nil {
		t.Fatalf("Hold: %v", err)
	}
	closed, done := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(done)
		closed <- lease.Close()
	}()
	t.Cleanup(func() {
		release()
		<-done
	})

	// Barrier: Close has begun once new holds are refused.
	deadline := time.Now().Add(5 * time.Second)
	for {
		extra, err := lease.Hold()
		if errors.Is(err, ErrLeaseClosed) {
			break
		}
		if err != nil {
			t.Fatalf("Hold while closing = %v", err)
		}
		extra()
		if time.Now().After(deadline) {
			t.Fatal("Close never began refusing new holds")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-closed:
		t.Fatalf("Close returned %v while a hold was outstanding", err)
	default:
	}
	if lease.Generation() <= 0 {
		t.Fatal("a held lease reports no generation while Close waits")
	}
	requireInstanceHeld(t, dir)

	release()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return after the last hold was released")
	}
	if lease.Generation() != 0 {
		t.Fatalf("closed lease reports generation %d", lease.Generation())
	}
	requireInstanceFree(t, dir)
}

// seedGeneration records a synthetic previous holder with generation gen and
// returns the exact lock-file bytes.
func seedGeneration(t *testing.T, gen string) []byte {
	t.Helper()
	path, err := LockPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureLockDir(path); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	raw := []byte(`{"root":"/tmp/synthetic-root","pid":1,"started_at":"2026-01-01T00:00:00Z","generation":` + gen + "}\n")
	if err := root.WriteFile(filepath.Base(path), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestInstanceGenerationExhaustionRefuses: the last int64 generation is still
// minted, but a recorded generation at or past MaxInt64 refuses every
// acquisition with the metadata bytes unchanged and the OS lock released:
// no wrap, reset or second authority (I18 (v2 §3)).
func TestInstanceGenerationExhaustionRefuses(t *testing.T) {
	dir := leaseRuntime(t)
	seedGeneration(t, strconv.FormatInt(math.MaxInt64-1, 10))
	lease, err := AcquireInstanceLease(dir)
	if err != nil {
		t.Fatalf("AcquireInstanceLease after generation MaxInt64-1: %v", err)
	}
	if lease.Generation() != math.MaxInt64 {
		t.Fatalf("Generation() = %d; want MaxInt64", lease.Generation())
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}

	path, err := LockPath()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	for _, gen := range []string{strconv.FormatInt(math.MaxInt64, 10), strconv.FormatUint(math.MaxUint64, 10)} {
		want := seedGeneration(t, gen)
		if minted, err := AcquireInstanceLease(dir); err == nil {
			got := minted.Generation()
			_ = minted.Close()
			t.Fatalf("AcquireInstanceLease after generation %s minted generation %d; want a refusal", gen, got)
		}
		if release, err := AcquireInstance(dir); err == nil {
			release()
			t.Fatalf("AcquireInstance after generation %s succeeded; want a refusal", gen)
		}
		got, err := root.ReadFile(filepath.Base(path))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("generation %s: lock metadata changed to %q; want unchanged %q", gen, got, want)
		}
		f, err := root.OpenFile(filepath.Base(path), os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if err := lockFile(f); err != nil {
			_ = f.Close()
			t.Fatalf("generation %s: OS lock still held after the refusal: %v", gen, err)
		}
		_ = unlockFile(f)
		_ = f.Close()
	}
}
