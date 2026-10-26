package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/qualify"
	"github.com/turbokast/mythhelm/internal/workers"
	"github.com/turbokast/mythhelm/internal/workspace"
)

// ServiceOwner is the service's proof that it owns one run: a held instance
// lease on the adopted canonical root plus that run's compatibility owner
// lock (supervisor-attachment-control design §2). Its fields are private, so
// a zero or foreign value proves nothing; it is never a boolean bypass of the
// legacy migration guard. Copies share one lifetime.
//
// Hooks.Event and Hooks.Notice run synchronously on the owned entry while it
// is busy and holds the run lock and the lease hold. They must not block
// unboundedly and are not reentrant: a callback must never call Close on its
// own owner (or on the lease), which would wait for the entry it runs inside.
// The service's actor delivers bounded copies to clients; to end ownership
// the caller cancels the active entry's context and calls Close outside the
// callback.
type ServiceOwner struct {
	s *ownerState
}

// ownerState is one capability's identity and lifetime. The identity fields
// are immutable after minting; mu guards busy, closing, closed and the
// releases. busy admits one owned entry at a time: the capability has
// exactly one ingestor.
type ownerState struct {
	lease      *control.InstanceLease
	generation int64
	root       string
	runID      string
	runDir     string

	mu          sync.Mutex
	idle        *sync.Cond
	busy        bool
	closing     bool
	closed      bool
	releaseRun  func()
	releaseHold func()
}

func ownershipRefused(format string, args ...any) error {
	return fmt.Errorf("ownership_unresolved: "+format+": %w", append(args, ErrOwnership)...)
}

// AcquireServiceOwner verifies the live lease, its canonical root, the run
// identifier and the adopted migration phase, then takes the run's
// compatibility owner lock. The lease stays held until Close.
func AcquireServiceOwner(ctx context.Context, j *journal.Journal, lease *control.InstanceLease, runID string) (*ServiceOwner, error) {
	if !workers.ValidID(runID) {
		return nil, ownershipRefused("invalid run id %q", runID)
	}
	releaseHold, err := lease.Hold()
	if err != nil {
		return nil, ownershipRefused("no live instance lease: %v", err)
	}
	owner, err := acquireServiceOwner(ctx, j, lease, runID)
	if err != nil {
		releaseHold()
		return nil, err
	}
	owner.s.releaseHold = releaseHold
	return owner, nil
}

func acquireServiceOwner(ctx context.Context, j *journal.Journal, lease *control.InstanceLease, runID string) (*ServiceOwner, error) {
	generation, root := lease.Generation(), lease.Root()
	if generation <= 0 {
		return nil, ownershipRefused("instance lease has no current generation")
	}
	if err := sameRoot(root, j.StateDir()); err != nil {
		return nil, err
	}
	phase, err := migrationPhase(ctx, j)
	if err != nil {
		return nil, err
	}
	// Only an adopted root belongs to the service; earlier phases stay with
	// the legacy writers and the explicit migration (I05, I18).
	if phase != "adopted" {
		return nil, ownershipRefused("migration phase is %q, not adopted", phase)
	}
	runDir := filepath.Join(j.StateDir(), "runs", runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return nil, err
	}
	releaseRun, err := AcquireOwner(runDir)
	if err != nil {
		return nil, err
	}
	s := &ownerState{lease: lease, generation: generation, root: root, runID: runID, runDir: runDir, releaseRun: releaseRun}
	s.idle = sync.NewCond(&s.mu)
	return &ServiceOwner{s: s}, nil
}

// sameRoot refuses a journal whose state directory is not the lease's root.
func sameRoot(root, stateDir string) error {
	if root == "" {
		return ownershipRefused("instance lease has no root")
	}
	a, err := os.Stat(root)
	if err != nil {
		return errors.Join(ownershipRefused("instance root unavailable"), err)
	}
	b, err := os.Stat(stateDir)
	if err != nil {
		return errors.Join(ownershipRefused("state directory unavailable"), err)
	}
	if !os.SameFile(a, b) {
		return ownershipRefused("state directory %s is not the instance root %s", stateDir, root)
	}
	return nil
}

// Close refuses new entries, waits for the active one, then releases the run
// lock and the lease hold. It is idempotent across every copy; concurrent
// calls all return only once the releases are done. Calling it from the
// active entry's own Hooks.Event or Hooks.Notice never returns.
func (o *ServiceOwner) Close() error {
	if o == nil || o.s == nil {
		return ownershipRefused("not a service owner")
	}
	s := o.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		for !s.closed {
			s.idle.Wait()
		}
		return nil
	}
	s.closing = true
	for s.busy {
		s.idle.Wait()
	}
	s.releaseRun()
	s.releaseHold()
	s.closed = true
	s.idle.Broadcast()
	return nil
}

// enter verifies the capability for runID on j and marks it busy until the
// returned exit runs. Every owned entry calls it before any effect.
func (o *ServiceOwner) enter(j *journal.Journal, runID string, h Hooks) (func(), error) {
	if o == nil || o.s == nil {
		return nil, ownershipRefused("not a service owner")
	}
	// A service client's detach closes only its subscription; forwarding it
	// (or a terminal interrupt) would stop ingestion while ownership stays held.
	if h.Detach != nil || h.Interrupt != nil {
		return nil, ownershipRefused("service owner accepts no terminal detach or interrupt")
	}
	s := o.s
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.closing:
		return nil, ownershipRefused("service owner is closed")
	case s.busy:
		return nil, ownershipRefused("run %s already has an owned entry", s.runID)
	case runID != s.runID:
		return nil, ownershipRefused("capability is for run %s, not %s", s.runID, runID)
	case s.lease.Generation() != s.generation:
		return nil, ownershipRefused("instance generation changed")
	}
	if err := sameRoot(s.root, j.StateDir()); err != nil {
		return nil, err
	}
	s.busy = true
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.busy = false
		s.idle.Broadcast()
	}, nil
}

// RunOwned runs the admitted decision's pipeline under owner's held run lock,
// without the legacy migration guard or a second lock acquisition. Before
// any ledger or worker effect it binds the decision's workspace to the owned
// run and its project configuration to the admitted committed revision. This
// is not a sealed decision: the remaining fields are trusted as admitted.
func RunOwned(ctx context.Context, j *journal.Journal, owner *ServiceOwner, d admission.Decision, h Hooks) (Outcome, error) {
	exit, err := owner.enter(j, d.RunID, h)
	if err != nil {
		return Outcome{}, err
	}
	defer exit()
	if err := sameRoot(owner.s.root, d.StateDir); err != nil {
		return Outcome{}, err
	}
	if filepath.Clean(d.RunDir) != owner.s.runDir {
		return Outcome{}, ownershipRefused("run directory %s is not the owned %s", d.RunDir, owner.s.runDir)
	}
	if workdir := filepath.Join(owner.s.runDir, "workspace"); filepath.Clean(d.Workdir) != workdir {
		return Outcome{}, ownershipRefused("workspace %s is not the owned %s", d.Workdir, workdir)
	}
	if err := admittedProjectConfig(ctx, d); err != nil {
		return Outcome{}, err
	}
	if h.Event == nil {
		h.Event = func(journal.Event) {}
	}
	if h.Notice == nil {
		h.Notice = func(string) {}
	}
	exe, err := os.Executable()
	if err != nil {
		return Outcome{}, fmt.Errorf("locating mythhelm for the worker: %w", err)
	}
	if err := qualify.EnsureSeeded(ctx, j); err != nil {
		return Outcome{}, persistenceUnavailable(d.StateDir, err)
	}
	return runLocked(ctx, j, exe, d, h)
}

// commitOID is a full SHA-1 or SHA-256 object ID as rev-parse prints it.
var commitOID = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// admittedProjectConfig refuses a decision whose project configuration or
// digest differs from what admission reads: nothing under NoChecks, otherwise
// the exact mythhelm.toml bytes committed at the admitted base revision. The
// revision is resolved as Snapshot resolves it, and only the returned commit
// object ID reaches later git arguments, so a revision is never an option.
func admittedProjectConfig(ctx context.Context, d admission.Decision) error {
	if d.NoChecks {
		if d.ConfigDigest != "" || !reflect.DeepEqual(d.ProjectConfig, admission.ProjectConfig{}) {
			return fmt.Errorf("%w: a run without checks carries project configuration", admission.ErrProjectConfig)
		}
		return nil
	}
	out, err := workspace.Git(ctx, d.Snapshot.SourceRepo, true, "rev-parse", "--verify", "--end-of-options", d.Snapshot.BaseRev+"^{commit}")
	if err != nil {
		return fmt.Errorf("%w: resolve admitted revision %q: %w", admission.ErrProjectConfig, d.Snapshot.BaseRev, err)
	}
	oid := strings.TrimSpace(string(out))
	if !commitOID.MatchString(oid) {
		return fmt.Errorf("%w: admitted revision %q resolved to %q, not a commit object ID", admission.ErrProjectConfig, d.Snapshot.BaseRev, oid)
	}
	entry, err := workspace.Git(ctx, d.Snapshot.SourceRepo, true, "ls-tree", oid, "--", admission.ProjectConfigFile)
	if err != nil {
		return fmt.Errorf("%w: locate config in admitted revision: %w", admission.ErrProjectConfig, err)
	}
	if !strings.HasPrefix(string(entry), "100644 blob ") && !strings.HasPrefix(string(entry), "100755 blob ") {
		return fmt.Errorf("%w: mythhelm.toml must be a regular committed file", admission.ErrProjectConfig)
	}
	raw, err := workspace.Git(ctx, d.Snapshot.SourceRepo, true, "show", oid+":"+admission.ProjectConfigFile)
	if err != nil {
		return fmt.Errorf("%w: read admitted config: %w", admission.ErrProjectConfig, err)
	}
	cfg, digest, err := admission.ParseProjectConfig(raw)
	if err != nil {
		return err
	}
	if digest != d.ConfigDigest || !reflect.DeepEqual(cfg, d.ProjectConfig) {
		return fmt.Errorf("%w: decision does not match the config committed at %s", admission.ErrProjectConfig, d.Snapshot.BaseRev)
	}
	return nil
}

// RecoverOwned reconciles runID's durable observations and carries the full
// continuation (watch, ingest, freeze, conclude, receipt repair) under
// owner's held run lock. It never runs a parallel reconciliation.
func RecoverOwned(ctx context.Context, j *journal.Journal, owner *ServiceOwner, runID string, h Hooks) (RecoveryOutcome, error) {
	exit, err := owner.enter(j, runID, h)
	if err != nil {
		return RecoveryOutcome{}, err
	}
	defer exit()
	return recoverLocked(ctx, j, runID, h, RecoveryOutcome{RunID: runID})
}

// ApplyOwned runs the existing explicit, candidate- and ref-guarded apply
// under owner's held run lock. It grants no authority beyond ApplyRun's.
func ApplyOwned(ctx context.Context, j *journal.Journal, owner *ServiceOwner, runID, branch string, acceptFlags, acceptUnverified bool) (Receipt, error) {
	exit, err := owner.enter(j, runID, Hooks{})
	if err != nil {
		return nil, err
	}
	defer exit()
	return ApplyRun(ctx, j, runID, branch, acceptFlags, acceptUnverified)
}
