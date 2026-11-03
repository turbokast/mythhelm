package supervisor_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
)

// ownedLease points this process's instance lock at the fixture's runtime
// dir and takes the service lease for its root. t.Setenv forbids t.Parallel.
func ownedLease(t *testing.T, f fixture) *control.InstanceLease {
	t.Helper()
	t.Setenv("XDG_RUNTIME_DIR", f.rt)
	lease, err := control.AcquireInstanceLease(f.state)
	if err != nil {
		t.Fatalf("AcquireInstanceLease: %v", err)
	}
	t.Cleanup(func() { _ = lease.Close() })
	return lease
}

func setMigrationPhase(t *testing.T, j *journal.Journal, phase string) {
	t.Helper()
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), `UPDATE migration_state SET phase = ? WHERE id = 1`, phase)
		return err
	}); err != nil {
		t.Fatalf("set phase %q: %v", phase, err)
	}
}

func acquireOwned(ctx context.Context, t *testing.T, j *journal.Journal, lease *control.InstanceLease, runID string) *supervisor.ServiceOwner {
	t.Helper()
	owner, err := supervisor.AcquireServiceOwner(ctx, j, lease, runID)
	if err != nil {
		t.Fatalf("AcquireServiceOwner(%s): %v", runID, err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	return owner
}

// runLockHeld probes the compatibility run lock from outside its holder.
func runLockHeld(t *testing.T, runDir string) bool {
	t.Helper()
	release, err := supervisor.AcquireOwner(runDir)
	if err == nil {
		release()
		return false
	}
	if !errors.Is(err, supervisor.ErrOwnerHeld) {
		t.Fatalf("probing run lock %s: %v", runDir, err)
	}
	return true
}

// noRunEffects reports whether runID has neither a ledger row nor an
// attempt directory, the evidence of a worker launch.
func noRunEffects(t *testing.T, f fixture, j *journal.Journal, runID string) bool {
	t.Helper()
	_, err := j.Run(t.Context(), runID)
	if err != nil && !errors.Is(err, journal.ErrNotFound) {
		t.Fatalf("reading run %s: %v", runID, err)
	}
	_, statErr := os.Stat(filepath.Join(f.state, "runs", runID, "attempts"))
	return errors.Is(err, journal.ErrNotFound) && errors.Is(statErr, os.ErrNotExist)
}

func deadlineDecision(t *testing.T, f fixture, execution time.Duration) admission.Decision {
	t.Helper()
	d, err := admission.Decide(t.Context(), admission.Request{
		StateDir: f.state, Repo: f.repo, TaskFile: f.task,
		Adapter: admission.AdapterFake, Billing: admission.BillingLocalScripted,
		ExecutionProfile: admission.ProfileTrustedHost, Env: os.Environ(),
		NoChecks: true, Scenario: "slow",
		EnvelopeFlags: &billing.Ceilings{Execution: execution, Repairs: -1, Replans: -1, TransportRetries: -1},
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func stopRequester(t *testing.T, f fixture, d admission.Decision) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(workers.AttemptDir(f.state, d.RunID, d.AttemptID), "stop.request")) // #nosec G304 -- synthetic attempt control file
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	return request.RequestID
}

// TestServiceOwnerCapability: only a live lease on the adopted canonical root
// mints a run capability, every owned entry re-verifies it, and refusals
// leave no ledger row or worker. The valid capability runs the pipeline under
// its one held run lock until Close (I05/I06/I18 (v2 §§3, 6)).
func TestServiceOwnerCapability(t *testing.T) {
	// Sequential: the instance lock follows XDG_RUNTIME_DIR.
	f := newFixture(t)
	lease := ownedLease(t, f)
	j := f.journal(t)
	setMigrationPhase(t, j, "adopted")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	d := deadlineDecision(t, f, 2*time.Second)

	other, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close() })
	setMigrationPhase(t, other, "adopted")

	acquire := func(j *journal.Journal, lease *control.InstanceLease, runID string) error {
		owner, err := supervisor.AcquireServiceOwner(ctx, j, lease, runID)
		if err == nil {
			_ = owner.Close()
		}
		return err
	}
	runWith := func(owner *supervisor.ServiceOwner, j *journal.Journal, h supervisor.Hooks) error {
		_, err := supervisor.RunOwned(ctx, j, owner, d, h)
		return err
	}
	refusals := []struct {
		name string
		call func() error
	}{
		{"wrong root", func() error { return acquire(other, lease, d.RunID) }},
		{"zero forged lease", func() error { return acquire(j, &control.InstanceLease{}, d.RunID) }},
		{"nil lease", func() error { return acquire(j, nil, d.RunID) }},
		{"path run id", func() error { return acquire(j, lease, filepath.Join("..", d.RunID)) }},
		{"legacy previewed phase", func() error {
			setMigrationPhase(t, j, "previewed")
			defer setMigrationPhase(t, j, "adopted")
			return acquire(j, lease, d.RunID)
		}},
		{"imported phase", func() error {
			setMigrationPhase(t, j, "imported")
			defer setMigrationPhase(t, j, "adopted")
			return acquire(j, lease, d.RunID)
		}},
		{"zero forged owner", func() error { return runWith(&supervisor.ServiceOwner{}, j, supervisor.Hooks{}) }},
		{"nil owner", func() error { return runWith(nil, j, supervisor.Hooks{}) }},
		{"wrong run", func() error {
			owner := acquireOwned(ctx, t, j, lease, ids.New("run"))
			defer func() { _ = owner.Close() }()
			return runWith(owner, j, supervisor.Hooks{})
		}},
		{"owner used on another root", func() error {
			owner := acquireOwned(ctx, t, j, lease, d.RunID)
			defer func() { _ = owner.Close() }()
			return runWith(owner, other, supervisor.Hooks{})
		}},
		{"closed owner", func() error {
			owner := acquireOwned(ctx, t, j, lease, d.RunID)
			if err := owner.Close(); err != nil {
				t.Fatal(err)
			}
			return runWith(owner, j, supervisor.Hooks{})
		}},
		{"forwarded owner detach", func() error {
			owner := acquireOwned(ctx, t, j, lease, d.RunID)
			defer func() { _ = owner.Close() }()
			return runWith(owner, j, supervisor.Hooks{Detach: make(chan struct{})})
		}},
		{"legacy Run on adopted root", func() error {
			_, err := supervisor.Run(ctx, d, supervisor.Hooks{})
			return err
		}},
	}
	for _, r := range refusals {
		if err := r.call(); !errors.Is(err, supervisor.ErrOwnership) {
			t.Errorf("%s: err = %v; want an ownership refusal", r.name, err)
		}
		if !noRunEffects(t, f, j, d.RunID) {
			t.Fatalf("%s: refusal left a ledger row or worker for %s", r.name, d.RunID)
		} // I18 (v2 §3)
	}

	owner := acquireOwned(ctx, t, j, lease, d.RunID)
	if err := acquire(j, lease, d.RunID); !errors.Is(err, supervisor.ErrOwnerHeld) {
		t.Fatalf("second owner for a held run = %v; want ErrOwnerHeld", err)
	}
	probed, heldDuring := false, false
	out, err := supervisor.RunOwned(ctx, j, owner, d, supervisor.Hooks{Event: func(ev journal.Event) {
		if !probed && ev.Type == "attempt.launched" {
			probed, heldDuring = true, runLockHeld(t, d.RunDir)
		}
	}})
	if err != nil {
		t.Fatalf("RunOwned: %v", err)
	}
	if out.State != supervisor.RunBlocked || out.Reason != workers.DeadlineReason || out.AttemptState != supervisor.AttemptStopped {
		t.Fatalf("RunOwned outcome %+v; want blocked by the immutable deadline with the attempt stopped", out)
	} // I21 (v2 §6)
	if !probed || !heldDuring {
		t.Fatalf("run lock during the owned run: probed=%t held=%t", probed, heldDuring)
	}
	if n := eventTypeCounts(t, f, d.RunID)["attempt.launched"]; n != 1 {
		t.Fatalf("native launches %d, want 1", n)
	}
	if noRunEffects(t, f, j, d.RunID) {
		t.Fatal("the effect probe cannot see the valid run's ledger row and worker")
	} // counterexample: the refusal assertions above can fail
	if !runLockHeld(t, d.RunDir) {
		t.Fatal("RunOwned released the capability's run lock")
	}

	// Apply reaches the existing guarded body only through a matching owner.
	if _, err := supervisor.ApplyOwned(ctx, j, owner, ids.New("run"), "mythhelm/owned", false, false); !errors.Is(err, supervisor.ErrOwnership) {
		t.Fatalf("ApplyOwned for another run = %v; want an ownership refusal", err)
	}
	if _, err := supervisor.ApplyOwned(ctx, j, owner, d.RunID, "mythhelm/owned", false, false); !errors.Is(err, supervisor.ErrApplyBlocked) {
		t.Fatalf("ApplyOwned of a blocked run = %v; want the existing ErrApplyBlocked guard", err)
	}

	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if runLockHeld(t, d.RunDir) {
		t.Fatal("Close kept the run lock")
	}
	if _, err := supervisor.RecoverOwned(ctx, j, owner, d.RunID, supervisor.Hooks{}); !errors.Is(err, supervisor.ErrOwnership) {
		t.Fatalf("RecoverOwned after Close = %v; want an ownership refusal", err)
	}
	if _, err := supervisor.ApplyOwned(ctx, j, owner, d.RunID, "mythhelm/owned", false, false); !errors.Is(err, supervisor.ErrOwnership) {
		t.Fatalf("ApplyOwned after Close = %v; want an ownership refusal", err)
	}

	// A closed lease's generation is stale once the next boot holds the lock.
	gen := lease.Generation()
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := control.AcquireInstanceLease(f.state)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = next.Close() })
	if next.Generation() != gen+1 {
		t.Fatalf("next generation %d; want %d", next.Generation(), gen+1)
	}
	stale := ids.New("run")
	if err := acquire(j, lease, stale); !errors.Is(err, supervisor.ErrOwnership) {
		t.Fatalf("owner from a closed generation-%d lease = %v; want an ownership refusal", gen, err)
	}
	if !noRunEffects(t, f, j, stale) {
		t.Fatal("stale lease refusal left effects")
	}
	if err := acquire(j, next, stale); err != nil {
		t.Fatalf("owner from the current lease: %v", err)
	}
}

// TestServiceOwnerCopiesShareOneLifetime: a copied ServiceOwner is the same
// capability, not a second one. While the original ingests, the copy cannot
// open a parallel entry; closing the copy releases the run lock and leaves
// every handle refused, with no further ledger effects (I05/I18 (v2 §3)).
func TestServiceOwnerCopiesShareOneLifetime(t *testing.T) {
	// Sequential: the instance lock follows XDG_RUNTIME_DIR.
	f := newFixture(t)
	lease := ownedLease(t, f)
	j := f.journal(t)
	setMigrationPhase(t, j, "adopted")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	d := deadlineDecision(t, f, 2*time.Second)

	owner := acquireOwned(ctx, t, j, lease, d.RunID)
	copied := *owner // copied before any entry, while nothing inside is locked
	var copiedErr error
	probed := false
	out, err := supervisor.RunOwned(ctx, j, owner, d, supervisor.Hooks{Event: func(ev journal.Event) {
		if !probed && ev.Type == "attempt.launched" {
			probed = true
			_, copiedErr = supervisor.ApplyOwned(ctx, j, &copied, d.RunID, "mythhelm/copied", false, false)
		}
	}})
	if err != nil {
		t.Fatalf("RunOwned: %v", err)
	}
	if out.State != supervisor.RunBlocked || out.Reason != workers.DeadlineReason {
		t.Fatalf("RunOwned outcome %+v; want blocked by the immutable deadline", out)
	}
	if !probed {
		t.Fatal("RunOwned ingested no launch event")
	}
	if !errors.Is(copiedErr, supervisor.ErrOwnership) {
		t.Fatalf("entry through a copy while the original ingests = %v; want an ownership refusal", copiedErr)
	}
	// Counterexample: the idle copy is a live capability, so the refusal
	// above came from the active entry, not from a broken copy.
	if _, err := supervisor.ApplyOwned(ctx, j, &copied, d.RunID, "mythhelm/copied", false, false); !errors.Is(err, supervisor.ErrApplyBlocked) {
		t.Fatalf("ApplyOwned through the idle copy = %v; want the existing ErrApplyBlocked guard", err)
	}

	if err := copied.Close(); err != nil {
		t.Fatal(err)
	}
	if runLockHeld(t, d.RunDir) {
		t.Fatal("closing the copy kept the run lock")
	}
	before := eventTypeCounts(t, f, d.RunID)
	if _, err := supervisor.RecoverOwned(ctx, j, owner, d.RunID, supervisor.Hooks{}); !errors.Is(err, supervisor.ErrOwnership) {
		t.Fatalf("RecoverOwned through the original after the copy closed = %v; want an ownership refusal", err)
	}
	if _, err := supervisor.ApplyOwned(ctx, j, owner, d.RunID, "mythhelm/copied", false, false); !errors.Is(err, supervisor.ErrOwnership) {
		t.Fatalf("ApplyOwned through the original after the copy closed = %v; want an ownership refusal", err)
	}
	if _, err := supervisor.RunOwned(ctx, j, owner, d, supervisor.Hooks{}); !errors.Is(err, supervisor.ErrOwnership) {
		t.Fatalf("RunOwned through the original after the copy closed = %v; want an ownership refusal", err)
	}
	if after := eventTypeCounts(t, f, d.RunID); !reflect.DeepEqual(after, before) {
		t.Fatalf("events after ownership was released %v; want unchanged %v", after, before)
	} // I18 (v2 §3)
	if err := owner.Close(); err != nil {
		t.Fatalf("closing the original after its copy: %v", err)
	}
}

// closersAccountedFor reports whether returned+parked covers want closers,
// parked counting goroutines blocked inside ServiceOwner.Close.
func closersAccountedFor(returned, want int) bool {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	parked := 0
	for g := range strings.SplitSeq(string(buf), "\n\n") {
		if strings.Contains(g, "supervisor.(*ServiceOwner).Close(") && strings.Contains(g, "sync.(*Cond).Wait(") {
			parked++
		}
	}
	return returned+parked >= want
}

// TestServiceOwnerConcurrentCloseWaitsForActiveEntry: every Close, through
// the original or a copy, waits until the active entry exits and the run lock
// and lease hold are released; none returns while ownership is still held.
func TestServiceOwnerConcurrentCloseWaitsForActiveEntry(t *testing.T) {
	// Sequential: the instance lock follows XDG_RUNTIME_DIR.
	f := newFixture(t)
	lease := ownedLease(t, f)
	j := f.journal(t)
	setMigrationPhase(t, j, "adopted")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	d := deadlineDecision(t, f, 2*time.Second)

	owner := acquireOwned(ctx, t, j, lease, d.RunID)
	copied := *owner // copied before any entry, while nothing inside is locked
	entered, release := make(chan struct{}), make(chan struct{})
	releaseEntry := sync.OnceFunc(func() { close(release) })
	type result struct {
		out supervisor.Outcome
		err error
	}
	ran := make(chan result, 1)
	var active sync.WaitGroup
	t.Cleanup(func() { releaseEntry(); active.Wait() })
	active.Go(func() {
		blocked := false
		out, err := supervisor.RunOwned(ctx, j, owner, d, supervisor.Hooks{Event: func(ev journal.Event) {
			if !blocked && ev.Type == "attempt.launched" {
				blocked = true
				close(entered)
				<-release
			}
		}})
		ran <- result{out, err}
	})
	select {
	case <-entered:
	case r := <-ran:
		t.Fatalf("RunOwned ended before its launch barrier: %+v %v", r.out, r.err)
	}

	handles := []*supervisor.ServiceOwner{owner, &copied}
	closeErrs := make([]error, len(handles))
	probeErrs := make([]error, len(handles))
	done := make(chan struct{}, len(handles))
	var probeMu sync.Mutex
	for i, h := range handles {
		active.Go(func() {
			closeErrs[i] = h.Close()
			// Probe the run lock the moment Close returns. The probes are
			// serialised so one closer's probe cannot hold the lock against
			// the other's.
			probeMu.Lock()
			var releaseProbe func()
			if releaseProbe, probeErrs[i] = supervisor.AcquireOwner(d.RunDir); probeErrs[i] == nil {
				releaseProbe()
			}
			probeMu.Unlock()
			done <- struct{}{}
		})
	}
	returned := 0
	for !closersAccountedFor(returned, len(handles)) {
		select {
		case <-done:
			returned++
		case <-ctx.Done():
			t.Fatalf("closers neither returned nor blocked in Close: %v", ctx.Err())
		default:
			runtime.Gosched()
		}
	}
	if returned != 0 {
		t.Fatalf("%d Close calls returned while the entry was active", returned)
	}
	if !runLockHeld(t, d.RunDir) {
		t.Fatal("run lock released while the entry was active")
	}
	if _, err := supervisor.ApplyOwned(ctx, j, &copied, d.RunID, "mythhelm/copied", false, false); !errors.Is(err, supervisor.ErrOwnership) {
		t.Fatalf("new entry while closing = %v; want an ownership refusal", err)
	}

	releaseEntry()
	r := <-ran
	if r.err != nil || r.out.State != supervisor.RunBlocked || r.out.Reason != workers.DeadlineReason {
		t.Fatalf("active entry under Close %+v: %v; want it to finish blocked by its deadline", r.out, r.err)
	}
	active.Wait()
	for i := range handles {
		if closeErrs[i] != nil {
			t.Errorf("Close %d: %v", i, closeErrs[i])
		}
		if probeErrs[i] != nil {
			t.Errorf("run lock when Close %d returned: %v; want released", i, probeErrs[i])
		}
	}
	if _, err := supervisor.RecoverOwned(ctx, j, &copied, d.RunID, supervisor.Hooks{}); !errors.Is(err, supervisor.ErrOwnership) {
		t.Fatalf("RecoverOwned after Close = %v; want an ownership refusal", err)
	}
	// The lease hold is released: the lease closes without waiting.
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestOwnedRecoveryUsesOneIngestor: RecoverOwned reattaches the same worker,
// ingests and concludes under the capability's one held run lock. A parallel
// legacy recovery, a second entry on the same capability and a reacquire of
// the held lock all fail while it ingests (I05/I12 (v2 §§3, 6)).
func TestOwnedRecoveryUsesOneIngestor(t *testing.T) {
	// Sequential: the instance lock follows XDG_RUNTIME_DIR.
	f := newFixture(t)
	lease := ownedLease(t, f)
	d, _, _ := seedLaunchTimed(t, f, "slow", pinnedPayload, 2*time.Second)
	j := f.journal(t)
	setMigrationPhase(t, j, "adopted")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	if _, err := supervisor.RecoverWithHooks(ctx, j, d.RunID, supervisor.Hooks{}); !errors.Is(err, supervisor.ErrOwnership) {
		t.Fatalf("legacy recovery of an adopted root = %v; want an ownership refusal", err)
	}
	owner := acquireOwned(ctx, t, j, lease, d.RunID)
	var parallelErr, secondErr, reacquireErr error
	probed := false
	out, err := supervisor.RecoverOwned(ctx, j, owner, d.RunID, supervisor.Hooks{Event: func(journal.Event) {
		if probed {
			return
		}
		probed = true
		_, parallelErr = supervisor.RecoverWithHooks(ctx, j, d.RunID, supervisor.Hooks{})
		_, secondErr = supervisor.RecoverOwned(ctx, j, owner, d.RunID, supervisor.Hooks{})
		var release func()
		if release, reacquireErr = supervisor.AcquireOwner(d.RunDir); reacquireErr == nil {
			release()
		}
	}})
	if err != nil {
		t.Fatalf("RecoverOwned: %v", err)
	}
	if out.State != supervisor.RunBlocked || out.Reason != workers.DeadlineReason || out.AttemptState != supervisor.AttemptStopped || out.AttemptID != d.AttemptID {
		t.Fatalf("RecoverOwned outcome %+v; want the same attempt concluded blocked by its deadline", out)
	}
	if !probed {
		t.Fatal("RecoverOwned ingested no event")
	}
	if !errors.Is(parallelErr, supervisor.ErrOwnership) {
		t.Fatalf("parallel legacy recovery = %v; want an ownership refusal", parallelErr)
	}
	if !errors.Is(secondErr, supervisor.ErrOwnership) {
		t.Fatalf("second owned entry while ingesting = %v; want an ownership refusal", secondErr)
	}
	if !errors.Is(reacquireErr, supervisor.ErrOwnerHeld) {
		t.Fatalf("reacquiring the held run lock = %v; want ErrOwnerHeld", reacquireErr)
	}
	counts := eventTypeCounts(t, f, d.RunID)
	if counts["attempt.launched"] != 1 || counts["candidate.frozen"] != 1 {
		t.Fatalf("event counts %v; want one launch and one candidate", counts)
	} // I12 (v2 §6)

	// Counterexample: a body that reacquires its run lock under the held
	// capability is refused, so RecoverOwned cannot have done so.
	setMigrationPhase(t, j, "previewed")
	if _, err := supervisor.RecoverWithHooks(ctx, j, d.RunID, supervisor.Hooks{}); !errors.Is(err, supervisor.ErrOwnerHeld) {
		t.Fatalf("lock-reacquiring recovery under the held capability = %v; want ErrOwnerHeld", err)
	}
}

// TestLegacyDetachContractPreserved: before migration, legacy Run and
// recovery detach without a stop request and release their lock. After
// adoption, an owned entry refuses a forwarded detach hook, keeps the worker
// and its lock, and the worker's immutable launch deadline is the first stop
// (MH-23; I06/I21 (v2 §§3, 6)).
func TestLegacyDetachContractPreserved(t *testing.T) {
	// Sequential: the instance lock follows XDG_RUNTIME_DIR.
	f := newFixture(t)
	t.Setenv("XDG_RUNTIME_DIR", f.rt)
	d := deadlineDecision(t, f, 4*time.Second)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	release, err := control.AcquireInstance(f.state)
	if err != nil {
		t.Fatal(err)
	}
	detach := make(chan struct{}, 1)
	sent := false
	out, err := supervisor.Run(ctx, d, supervisor.Hooks{Detach: detach, Event: func(ev journal.Event) {
		if !sent && ev.Type == "attempt.state_changed" && payloadOf(t, ev)["state"] == "running" {
			sent = true
			detach <- struct{}{}
		}
	}})
	if err != nil || !sent || !out.DetachedRunning || out.State != supervisor.RunExecuting {
		t.Fatalf("legacy detach %+v sent=%t: %v", out, sent, err)
	}
	assertRunningDetached(t, f, d)
	detach <- struct{}{}
	recovered, err := supervisor.RecoverWithHooks(ctx, f.journal(t), d.RunID, supervisor.Hooks{Detach: detach})
	if err != nil || !recovered.DetachedRunning || recovered.AttemptID != d.AttemptID {
		t.Fatalf("legacy recovery detach %+v: %v", recovered, err)
	}
	assertRunningDetached(t, f, d)
	if runLockHeld(t, d.RunDir) {
		t.Fatal("legacy detach kept the run lock")
	}
	release()

	lease := ownedLease(t, f)
	j := f.journal(t)
	setMigrationPhase(t, j, "adopted")
	owner := acquireOwned(ctx, t, j, lease, d.RunID)
	detach <- struct{}{}
	if _, err := supervisor.RecoverOwned(ctx, j, owner, d.RunID, supervisor.Hooks{Detach: detach}); !errors.Is(err, supervisor.ErrOwnership) {
		t.Fatalf("owned recovery with a forwarded detach = %v; want an ownership refusal", err)
	}
	if len(detach) != 1 {
		t.Fatal("the refused owned entry consumed the client's detach")
	}
	<-detach
	assertRunningDetached(t, f, d)
	if !runLockHeld(t, d.RunDir) {
		t.Fatal("a refused client detach released service ownership")
	}

	out2, err := supervisor.RecoverOwned(ctx, j, owner, d.RunID, supervisor.Hooks{})
	if err != nil {
		t.Fatalf("RecoverOwned: %v", err)
	}
	if out2.State != supervisor.RunBlocked || out2.Reason != workers.DeadlineReason || out2.AttemptState != supervisor.AttemptStopped || out2.AttemptID != d.AttemptID {
		t.Fatalf("owned recovery %+v; want the original attempt stopped by its launch deadline", out2)
	}
	if got := stopRequester(t, f, d); got != workers.DeadlineRequester {
		t.Fatalf("first stop requester %q; want the immutable deadline %q", got, workers.DeadlineRequester)
	} // ADR0020 first-request precedence
	counts := eventTypeCounts(t, f, d.RunID)
	if counts["attempt.launched"] != 1 || counts["attempt.launch_intent_recorded"] != 1 {
		t.Fatalf("launch counts %v; want the one original launch", counts)
	} // I12/I18 (v2 §2)
}

// TestServiceOwnerRejectsInvalidRunIDs: only a workers.ValidID run id mints a
// capability. Dots, non-ASCII, separators and over-long ids are refused
// before any run directory exists; ASCII underscore ids up to 64 characters
// still acquire (I18 (v2 §3)).
func TestServiceOwnerRejectsInvalidRunIDs(t *testing.T) {
	// Sequential: the instance lock follows XDG_RUNTIME_DIR.
	f := newFixture(t)
	lease := ownedLease(t, f)
	j := f.journal(t)
	setMigrationPhase(t, j, "adopted")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	runs := filepath.Join(f.state, "runs")
	listRuns := func() []string {
		entries, err := os.ReadDir(runs)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		return names
	}

	before := listRuns()
	for _, id := range []string{"", ".", "..", "run.1", "run-1", "rün", "run 1", "run/1", `run\1`, strings.Repeat("a", 65)} {
		owner, err := supervisor.AcquireServiceOwner(ctx, j, lease, id)
		if err == nil {
			_ = owner.Close()
		}
		if !errors.Is(err, supervisor.ErrOwnership) {
			t.Errorf("AcquireServiceOwner(%q) = %v; want an ownership refusal", id, err)
		}
	}
	if after := listRuns(); !slices.Equal(after, before) {
		t.Fatalf("refused run ids changed the run directories from %v to %v", before, after)
	}
	for _, id := range []string{"run_ok_1", strings.Repeat("A", 64)} {
		owner, err := supervisor.AcquireServiceOwner(ctx, j, lease, id)
		if err != nil {
			t.Fatalf("AcquireServiceOwner(%q): %v; want a capability", id, err)
		}
		if !runLockHeld(t, filepath.Join(runs, id)) {
			t.Fatalf("capability for %q holds no run lock", id)
		}
		if err := owner.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// TestServiceOwnerRunBindsAdmittedWorkspaceAndConfig: RunOwned refuses a
// decision whose workspace is not the owned run's or whose checks, envelopes
// or config digest differ from the config committed at the admitted base
// revision, before any ledger row, worker or workspace exists. The unmutated
// admitted decision, and a no-checks decision without configuration, still
// run (I02/I18 (v2 §§2, 3)).
func TestServiceOwnerRunBindsAdmittedWorkspaceAndConfig(t *testing.T) {
	// Sequential: the instance lock follows XDG_RUNTIME_DIR.
	f := newFixture(t)
	digest := f.envelopeConfig(t)
	lease := ownedLease(t, f)
	j := f.journal(t)
	setMigrationPhase(t, j, "adopted")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	d := f.decideEnvelope(t, digest, nil)
	owner := acquireOwned(ctx, t, j, lease, d.RunID)
	outside := t.TempDir()

	mutants := []struct {
		name   string
		want   error
		mutate func(*admission.Decision)
	}{
		{"workdir outside the owned run", supervisor.ErrOwnership, func(m *admission.Decision) { m.Workdir = outside }},
		{"workdir is the run directory", supervisor.ErrOwnership, func(m *admission.Decision) { m.Workdir = m.RunDir }},
		{"check argv replaced", admission.ErrProjectConfig, func(m *admission.Decision) {
			m.ProjectConfig.Checks = slices.Clone(m.ProjectConfig.Checks)
			m.ProjectConfig.Checks[0].Argv = []string{"true"}
		}},
		{"check dropped", admission.ErrProjectConfig, func(m *admission.Decision) { m.ProjectConfig.Checks = nil }},
		{"repairs envelope raised", admission.ErrProjectConfig, func(m *admission.Decision) {
			repairs := 9
			m.ProjectConfig.Envelopes.Repairs = &repairs
		}},
		{"execution envelope removed", admission.ErrProjectConfig, func(m *admission.Decision) { m.ProjectConfig.Envelopes.Execution = "" }},
		{"config digest replaced", admission.ErrProjectConfig, func(m *admission.Decision) { m.ConfigDigest = strings.Repeat("0", 64) }},
		{"no checks with admitted config", admission.ErrProjectConfig, func(m *admission.Decision) { m.NoChecks = true }},
	}
	for _, mc := range mutants {
		m := d
		mc.mutate(&m)
		if _, err := supervisor.RunOwned(ctx, j, owner, m, supervisor.Hooks{}); !errors.Is(err, mc.want) {
			t.Errorf("%s: RunOwned = %v; want %v", mc.name, err, mc.want)
		}
		if !noRunEffects(t, f, j, d.RunID) {
			t.Fatalf("%s: refusal left a ledger row or worker for %s", mc.name, d.RunID)
		} // I18 (v2 §3)
		if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
			t.Fatalf("%s: outside directory has %d entries (%v); want untouched", mc.name, len(entries), err)
		}
		if _, err := os.Stat(d.Workdir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s: refusal created the owned workspace: %v", mc.name, err)
		}
	}

	out, err := supervisor.RunOwned(ctx, j, owner, d, supervisor.Hooks{})
	if err != nil || out.State != supervisor.RunReadyForReview {
		t.Fatalf("RunOwned of the admitted decision = %+v, %v; want ready_for_review", out, err)
	}
	if n := eventTypeCounts(t, f, d.RunID)["attempt.launched"]; n != 1 {
		t.Fatalf("native launches %d, want 1", n)
	}

	nc := deadlineDecision(t, f, 2*time.Second)
	ncOwner := acquireOwned(ctx, t, j, lease, nc.RunID)
	forged := nc
	forged.ConfigDigest = digest
	if _, err := supervisor.RunOwned(ctx, j, ncOwner, forged, supervisor.Hooks{}); !errors.Is(err, admission.ErrProjectConfig) {
		t.Fatalf("no-checks decision carrying a config digest = %v; want a config refusal", err)
	}
	if !noRunEffects(t, f, j, nc.RunID) {
		t.Fatal("no-checks config refusal left a ledger row or worker")
	}
	out, err = supervisor.RunOwned(ctx, j, ncOwner, nc, supervisor.Hooks{})
	if err != nil || out.State != supervisor.RunBlocked || out.Reason != workers.DeadlineReason {
		t.Fatalf("RunOwned of the no-checks decision = %+v, %v; want blocked by its deadline", out, err)
	}
}

// mktree commits a single-entry root tree built from line and returns the
// commit's object ID, without moving any branch.
func mktree(t *testing.T, f fixture, line string) string {
	t.Helper()
	cmd := exec.Command("git", "mktree")
	cmd.Dir = f.repo
	cmd.Env = append(os.Environ(), "HOME="+f.home, "XDG_CONFIG_HOME="+f.home, "GIT_CONFIG_NOSYSTEM=1")
	cmd.Stdin = strings.NewReader(line + "\n")
	tree, err := cmd.Output()
	if err != nil {
		t.Fatalf("git mktree: %v", err)
	}
	return f.git(t, "commit-tree", strings.TrimSpace(string(tree)), "-m", "forged config entry")
}

// TestServiceOwnerRunResolvesAdmittedRevisionSafely: the config check resolves
// the decision's base revision with --end-of-options before any other git
// call, so an option-like revision never runs as a git option (here one that
// would write an outside file), an unresolvable revision refuses, and a
// committed mythhelm.toml that is not a regular blob refuses before parsing,
// all before any ledger row, worker or workspace exists. The admitted
// decision still runs (I02/I18 (v2 §§2, 3)).
func TestServiceOwnerRunResolvesAdmittedRevisionSafely(t *testing.T) {
	// Sequential: the instance lock follows XDG_RUNTIME_DIR.
	f := newFixture(t)
	digest := f.envelopeConfig(t)
	lease := ownedLease(t, f)
	j := f.journal(t)
	setMigrationPhase(t, j, "adopted")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	d := f.decideEnvelope(t, digest, nil)
	owner := acquireOwned(ctx, t, j, lease, d.RunID)
	outside := t.TempDir()
	leak := filepath.Join(outside, "leak")

	blob := f.git(t, "rev-parse", "HEAD:mythhelm.toml")
	revisions := []struct{ name, rev string }{
		{"option-like revision writing an outside file", "--output=" + leak},
		{"option-like revision with a commit suffix", "--output=" + leak + "^{commit}"},
		{"unresolvable revision", "refs/heads/does-not-exist"},
		{"unknown object ID", strings.Repeat("0", len(d.Snapshot.BaseRev))},
		{"config committed as a symlink to the admitted bytes", mktree(t, f, "120000 blob "+blob+"\tmythhelm.toml")},
		{"config committed as a tree", mktree(t, f, "040000 tree "+f.git(t, "rev-parse", "HEAD^{tree}")+"\tmythhelm.toml")},
		{"config absent at the revision", mktree(t, f, "100644 blob "+blob+"\tother.toml")},
	}
	for _, rc := range revisions {
		m := d
		m.Snapshot.BaseRev = rc.rev
		if _, err := supervisor.RunOwned(ctx, j, owner, m, supervisor.Hooks{}); !errors.Is(err, admission.ErrProjectConfig) {
			t.Errorf("%s: RunOwned = %v; want %v", rc.name, err, admission.ErrProjectConfig)
		}
		if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
			t.Fatalf("%s: outside directory has %d entries (%v); want untouched", rc.name, len(entries), err)
		}
		if !noRunEffects(t, f, j, d.RunID) {
			t.Fatalf("%s: refusal left a ledger row or worker for %s", rc.name, d.RunID)
		} // I18 (v2 §3)
		if _, err := os.Stat(d.Workdir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s: refusal created the owned workspace: %v", rc.name, err)
		}
	}

	out, err := supervisor.RunOwned(ctx, j, owner, d, supervisor.Hooks{})
	if err != nil || out.State != supervisor.RunReadyForReview {
		t.Fatalf("RunOwned of the admitted decision = %+v, %v; want ready_for_review", out, err)
	}
}

// TestRetainedOwnerObservationLifetime: an observation retained from an
// existing owner during a busy owned entry keeps the run lock and lease hold
// after the entry joins, until every retained copy releases; Close waits for
// both. Retention never acquires or enters (I06/I18 (v2 §3)).
func TestRetainedOwnerObservationLifetime(t *testing.T) {
	// Sequential: the instance lock follows XDG_RUNTIME_DIR.
	f := newFixture(t)
	lease := ownedLease(t, f)
	d, _, waitWorker := seedLaunchTimed(t, f, "slow", pinnedPayload, 2*time.Second)
	j := f.journal(t)
	setMigrationPhase(t, j, "adopted")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	other, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close() })

	owner := acquireOwned(ctx, t, j, lease, d.RunID)
	copied := *owner
	for _, c := range []struct {
		name  string
		owner *supervisor.ServiceOwner
		runID string
		want  bool
	}{
		{"nil owner", nil, d.RunID, false},
		{"zero owner", &supervisor.ServiceOwner{}, d.RunID, false},
		{"empty run", owner, "", false},
		{"other run", owner, ids.New("run"), false},
		{"own run", owner, d.RunID, true},
		{"copy", &copied, d.RunID, true},
	} {
		if got := c.owner.MatchesRun(c.runID); got != c.want {
			t.Errorf("MatchesRun %s = %t; want %t", c.name, got, c.want)
		}
	}

	entered, release := make(chan struct{}), make(chan struct{})
	releaseEntry := sync.OnceFunc(func() { close(release) })
	ran := make(chan error, 1)
	var active sync.WaitGroup
	t.Cleanup(func() { releaseEntry(); active.Wait() })
	active.Go(func() {
		var once sync.Once
		_, err := supervisor.RecoverOwned(ctx, j, owner, d.RunID, supervisor.Hooks{Event: func(journal.Event) {
			once.Do(func() { close(entered); <-release })
		}})
		ran <- err
	})
	select {
	case <-entered:
	case err := <-ran:
		t.Fatalf("RecoverOwned ended before its callback barrier: %v", err)
	}

	for _, c := range []struct {
		name  string
		owner *supervisor.ServiceOwner
		j     *journal.Journal
		runID string
	}{
		{"nil owner", nil, j, d.RunID},
		{"zero owner", &supervisor.ServiceOwner{}, j, d.RunID},
		{"foreign run", owner, j, ids.New("run")},
		{"replaced root", owner, other, d.RunID},
		{"nil journal", owner, nil, d.RunID},
	} {
		if rel, err := c.owner.RetainObservation(ctx, c.j, c.runID); !errors.Is(err, supervisor.ErrOwnership) || rel != nil {
			if rel != nil {
				rel()
			}
			t.Errorf("RetainObservation %s = %v; want an ownership refusal", c.name, err)
		}
	}
	// Quiesce this real worker while the real recovery callback remains held.
	// Its complete identity and projected rows are then stable independent
	// witnesses of observation-only behaviour, without invented call counts.
	waitWorker()
	attemptBefore, err := j.Attempt(ctx, d.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	runBefore, err := j.Run(ctx, d.RunID)
	if err != nil {
		t.Fatal(err)
	}
	identityBefore, err := workers.ReadIdentity(workers.AttemptDir(f.state, d.RunID, d.AttemptID))
	if err != nil {
		t.Fatal(err)
	}
	countsBefore := eventTypeCounts(t, f, d.RunID)
	viaOwner, err := owner.RetainObservation(ctx, j, d.RunID)
	if err != nil {
		t.Fatalf("RetainObservation during a busy entry: %v", err)
	}
	t.Cleanup(viaOwner)
	viaCopy, err := copied.RetainObservation(ctx, j, d.RunID)
	if err != nil {
		t.Fatalf("RetainObservation through a copy: %v", err)
	}
	t.Cleanup(viaCopy)
	attemptAfter, err := j.Attempt(ctx, d.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	runAfter, err := j.Run(ctx, d.RunID)
	if err != nil {
		t.Fatal(err)
	}
	identityAfter, err := workers.ReadIdentity(workers.AttemptDir(f.state, d.RunID, d.AttemptID))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(attemptBefore, attemptAfter) || !reflect.DeepEqual(runBefore, runAfter) || !reflect.DeepEqual(identityBefore, identityAfter) {
		t.Fatal("retention changed the exact existing run, attempt or real worker launch identity")
	}
	if countsAfter := eventTypeCounts(t, f, d.RunID); !reflect.DeepEqual(countsBefore, countsAfter) {
		t.Fatalf("retention changed canonical recovery/launch event counts: before=%v after=%v", countsBefore, countsAfter)
	}

	closed := make(chan error, 1)
	active.Go(func() { closed <- copied.Close() })
	releaseEntry()
	if err := <-ran; err != nil {
		t.Fatalf("RecoverOwned: %v", err)
	}
	for !closersAccountedFor(len(closed), 1) {
		runtime.Gosched()
	}
	if len(closed) != 0 {
		t.Fatal("Close returned while observations were retained")
	}
	if !runLockHeld(t, d.RunDir) {
		t.Fatal("run lock released after the entry joined while observations were retained")
	}
	if rel, err := owner.RetainObservation(ctx, j, d.RunID); !errors.Is(err, supervisor.ErrOwnership) || rel != nil {
		if rel != nil {
			rel()
		}
		t.Fatalf("RetainObservation while closing = %v; want an ownership refusal", err)
	}
	viaOwner()
	viaOwner() // a duplicate release must not release the copy's reference
	if !runLockHeld(t, d.RunDir) {
		t.Fatal("a duplicate release dropped another observation's reference")
	}
	if len(closed) != 0 {
		t.Fatal("Close returned while the copy's observation was retained")
	}
	viaCopy()
	if err := <-closed; err != nil {
		t.Fatalf("Close: %v", err)
	}
	if runLockHeld(t, d.RunDir) {
		t.Fatal("Close kept the run lock after every observation released")
	}
	if rel, err := owner.RetainObservation(ctx, j, d.RunID); !errors.Is(err, supervisor.ErrOwnership) || rel != nil {
		t.Fatalf("RetainObservation after Close = %v; want an ownership refusal", err)
	}
	if !owner.MatchesRun(d.RunID) {
		t.Fatal("MatchesRun of a closed owner must stay routing-only true")
	}
	after := eventTypeCounts(t, f, d.RunID)
	if after["attempt.launched"] != countsBefore["attempt.launched"] {
		t.Fatalf("observation launched work: %v -> %v", countsBefore, after)
	}

}

// Retain the actual service/run owners while replacing a closed journal.
// Windows refuses replacing an open SQLite file. Reopening also distinguishes
// the old owner's database identity check from a stale Journal handle.
func TestRetainedOwnerDatabaseReplacement(t *testing.T) {
	f := newFixture(t)
	lease := ownedLease(t, f)
	ctx := t.Context()
	j, err := journal.Open(ctx, f.state)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	setMigrationPhase(t, j, "adopted")
	freshRun := ids.New("run")
	fresh := acquireOwned(ctx, t, j, lease, freshRun)
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(f.state, journal.DBName)
	raw, err := os.ReadFile(db) // #nosec G304 -- fixture database
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(db, db+".orig"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(db, raw, 0o600); err != nil { // #nosec G703 -- db is the fixture's own temp state database

		t.Fatal(err)
	}
	replacement, err := journal.Open(ctx, f.state)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = replacement.Close() })
	if err := replacement.CheckStateIdentity(ctx); err != nil {
		t.Fatalf("replacement journal must have its own valid identity: %v", err)
	}
	if rel, err := fresh.RetainObservation(ctx, replacement, freshRun); !errors.Is(err, supervisor.ErrOwnership) || rel != nil {
		if rel != nil {
			rel()
		}
		t.Fatalf("RetainObservation on a replaced database = %v; want an ownership refusal", err)
	}
}
