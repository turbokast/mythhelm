package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain lets the test binary act as a lock-holder child process for
// TestStaleLockAdoptedWithGenerationBump (repo precedent: integration's
// __verify_helper, supervisor's TestMain helpers). The holder branch runs
// before flag parsing and m.Run, so the child never depends on the testing
// framework's -test.run selection: re-running the framework in the child
// exited during startup on darwin (exit status 2 before acquiring).
func TestMain(m *testing.M) {
	if os.Getenv("MYTHHELM_TEST_HELPER") == "lockholder" {
		os.Exit(runLockHolder())
	}
	if os.Getenv("MYTHHELM_TEST_HELPER") == "sleeper" {
		// Block until the parent kills this sleeper. Sleep (a pending
		// timer), never a bare select{}: this child has no other
		// goroutine, so select{} trips the runtime's
		// all-goroutines-asleep deadlock detector on some platforms.
		time.Sleep(1000 * time.Hour)
		os.Exit(0) // Unreachable: the parent kills the sleeper first.
	}
	code := m.Run()
	cleanupE2EBinary()
	os.Exit(code)
}

// runLockHolder acquires the instance lock and holds it until killed,
// returning the child process exit code.
func runLockHolder() int {
	release, err := AcquireInstance(os.Getenv("MYTHHELM_TEST_HELPER_DIR"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "holder: AcquireInstance: %v\n", err)
		return 3
	}
	defer release()
	// Hold the lock until the parent kills this holder. No readiness file
	// is needed: the parent polls the lock metadata for this process's
	// PID. Sleep (a pending timer), never a bare select{}: this child has
	// no other goroutine, so select{} trips the runtime's
	// all-goroutines-asleep deadlock detector on some platforms.
	time.Sleep(1000 * time.Hour)
	return 0 // Unreachable: the parent kills the holder first.
}

func TestSecondInstanceHeld(t *testing.T) {
	dir := t.TempDir()

	first, err := AcquireInstance(dir)
	if err != nil {
		t.Fatalf("first AcquireInstance: %v", err)
	}
	t.Cleanup(first)

	second, err := AcquireInstance(dir)
	if !errors.Is(err, ErrInstanceHeld) {
		if err == nil {
			second()
		}
		t.Fatalf("second AcquireInstance while held = %v, want ErrInstanceHeld", err)
	}

	first()
	third, err := AcquireInstance(dir)
	if err != nil {
		t.Fatalf("AcquireInstance after release: %v", err)
	}
	t.Cleanup(third)
	third()
}

func TestLockPathIndependentOfRoot(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	if rootA == rootB {
		t.Fatal("temp roots must differ")
	}

	got, err := LockPath()
	if err != nil {
		t.Fatalf("LockPath: %v", err)
	}
	again, err := LockPath()
	if err != nil {
		t.Fatalf("LockPath: %v", err)
	}
	if got == "" || got != again {
		t.Fatalf("LockPath not stable: %q vs %q", got, again)
	}
	for _, root := range []string{rootA, rootB} {
		if got == root || strings.HasPrefix(got, root+string(filepath.Separator)) {
			t.Fatalf("LockPath %q lives under root %q", got, root)
		}
	}
}

func TestRootConflictRefused(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()

	hold, err := AcquireInstance(rootA)
	if err != nil {
		t.Fatalf("AcquireInstance(%q): %v", rootA, err)
	}
	defer hold()

	release, err := AcquireInstance(rootB)
	if err == nil {
		release()
		t.Fatalf("AcquireInstance(%q) succeeded while root %q holds the lock: second authority", rootB, rootA)
	}
	if !errors.Is(err, ErrRootConflict) {
		t.Fatalf("AcquireInstance(%q) = %v, want ErrRootConflict", rootB, err)
	}
	if errors.Is(err, ErrInstanceHeld) {
		t.Fatalf("AcquireInstance(%q) = %v, must not be ErrInstanceHeld", rootB, err)
	}
}

func TestStaleLockAdoptedWithGenerationBump(t *testing.T) {
	dir := t.TempDir()
	path, err := LockPath()
	if err != nil {
		t.Fatalf("LockPath: %v", err)
	}

	cmd := exec.Command(os.Args[0]) //nolint:gosec // G702: re-executes this test binary; TestMain selects the holder branch
	cmd.Env = append(os.Environ(),
		"MYTHHELM_TEST_HELPER=lockholder",
		"MYTHHELM_TEST_HELPER_DIR="+dir,
	)
	var childOut, childErr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &childOut, &childErr
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting holder subprocess: %v", err)
	}
	holderPID := cmd.Process.Pid
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	// Wait until the holder has recorded its own metadata, so the generation
	// baseline comes from this holder and not a leftover lock file.
	var oldGen uint64
	ready := false
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-waitDone:
			t.Fatalf("holder subprocess exited before acquiring: %v\nstdout: %s\nstderr: %s",
				err, childOut.String(), childErr.String())
		default:
		}
		meta, ok := readMetadata(path)
		if ok && meta.PID == holderPID {
			oldGen = meta.Generation
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		_ = cmd.Process.Kill()
		<-waitDone
		t.Fatal("holder subprocess did not record its lock metadata in time")
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("killing holder subprocess: %v", err)
	}
	<-waitDone // the OS releases the holder's flock when it dies

	release, err := AcquireInstance(dir)
	if err != nil {
		t.Fatalf("AcquireInstance after holder death: %v", err)
	}
	defer release()

	meta, ok := readMetadata(path)
	if !ok {
		t.Fatal("no lock metadata after adoption")
	}
	if meta.Generation <= oldGen {
		t.Fatalf("generation after adoption = %d, want higher than %d", meta.Generation, oldGen)
	}
	if meta.PID != os.Getpid() {
		t.Fatalf("adopted PID = %d, want %d", meta.PID, os.Getpid())
	}
	if meta.Root != canonicalRoot(dir) {
		t.Fatalf("adopted root = %q, want %q", meta.Root, canonicalRoot(dir))
	}
}

// TestReadMetadataWaitsOutIncompleteRecord pins the reader side of the
// holder's recording window: AcquireInstance creates the lock file empty
// and writeMetadata truncates before it writes, so a concurrent reader (a
// spawn site's CurrentGeneration, another process's status read) can see
// an empty or partial record that is complete a moment later. The reader
// must wait that out rather than report unexpected end of JSON input.
func TestReadMetadataWaitsOutIncompleteRecord(t *testing.T) { // I02
	want := lockMetadata{Root: "/state/root", PID: 4242, StartedAt: time.Unix(1700000000, 0).UTC(), Generation: 7}
	full, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	tests := []struct {
		name string
		seen []byte
	}{
		{name: "empty file", seen: nil},
		{name: "partial record", seen: full[:len(full)/2]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "supervisor.lock")
			if err := os.WriteFile(path, tt.seen, 0o600); err != nil {
				t.Fatalf("seeding incomplete record: %v", err)
			}
			finish := make(chan struct{})
			go func() {
				defer close(finish)
				time.Sleep(50 * time.Millisecond)
				if err := os.WriteFile(path, append(full, '\n'), 0o600); err != nil {
					t.Errorf("completing record: %v", err)
				}
			}()
			got, err := readMetadataErr(path)
			<-finish
			if err != nil {
				t.Fatalf("readMetadataErr during the recording window: %v, want the completed record", err)
			}
			if got != want {
				t.Fatalf("readMetadataErr = %+v, want %+v", got, want)
			}
		})
	}
}

// TestReadMetadataDuringRewriteNeverFails drives the real writer against a
// concurrent reader: writeMetadata truncates the flocked file before it
// writes, and a long record followed by a short one makes every rewrite
// pass through an empty file.
func TestReadMetadataDuringRewriteNeverFails(t *testing.T) { // I02
	path := filepath.Join(t.TempDir(), "supervisor.lock")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // G304: path is under t.TempDir()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()
	roots := []string{"/a/very/long/state/root/" + strings.Repeat("x", 200), "/r"}
	if err := writeMetadata(f, lockMetadata{Root: roots[1], PID: 1, Generation: 1}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	const rewrites = 200
	writerErr := make(chan error, 1)
	go func() {
		for i := range rewrites {
			if err := writeMetadata(f, lockMetadata{Root: roots[i%2], PID: 1, Generation: uint64(i)}); err != nil {
				writerErr <- err
				return
			}
		}
		writerErr <- nil
	}()

	reads, failures := 0, 0
	var firstFailure error
	for done := false; !done; {
		select {
		case err := <-writerErr:
			if err != nil {
				t.Fatalf("writeMetadata: %v", err)
			}
			done = true
		default:
		}
		if _, err := readMetadataErr(path); err != nil {
			failures++
			if firstFailure == nil {
				firstFailure = err
			}
		}
		reads++
	}
	if failures != 0 {
		t.Fatalf("%d of %d reads failed during rewrites, first: %v", failures, reads, firstFailure)
	}
}
