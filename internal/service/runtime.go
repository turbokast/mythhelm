// Package service composes the singleton runtime. Dispatch remains unavailable
// until the later attachment-control producers are independently delivered.
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/migrate"
	"github.com/turbokast/mythhelm/internal/statedir"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// ErrCleanupIncomplete means execution has not joined; ownership remains held.
var ErrCleanupIncomplete = errors.New("service cleanup is incomplete; ownership retained")

type runtime struct {
	mu                sync.Mutex // guards closing, entries and their cancellation functions
	lease             *control.InstanceLease
	journal           *journal.Journal
	server            *control.Server
	migrationRequired bool
	views             *viewSlots
	closing           bool
	entries           map[*actor]struct{}
	joined            chan struct{}
	closeOnce         sync.Once
	closeErr          error
}

// Run owns one root independently of any status client's lifetime. A fresh
// root is adopted; existing unadopted work is exposed only as read-only status.
// Public legacy run/recover/apply then remain unavailable on adopted roots
// until the later service clients land; this shell does not bypass that fence.
func Run(ctx context.Context, stateDir string) error {
	r, err := openRuntime(ctx, stateDir)
	if err != nil {
		return err
	}
	transport, err := control.DefaultTransport()
	if err != nil {
		return errors.Join(err, r.shutdown(context.WithoutCancel(ctx)))
	}
	endpoint, err := control.EndpointPath()
	if err != nil {
		return errors.Join(err, r.shutdown(context.WithoutCancel(ctx)))
	}
	// A live lease proves this endpoint has no other owner; only stale paths go.
	if err := os.Remove(endpoint); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(err, r.shutdown(context.WithoutCancel(ctx)))
	}
	listener, err := transport.Listen(endpoint)
	if err != nil {
		return errors.Join(err, r.shutdown(context.WithoutCancel(ctx)))
	}
	err = control.Serve(ctx, listener, r.server, nil)
	return errors.Join(err, r.shutdown(context.WithoutCancel(ctx)))
}

func openRuntime(ctx context.Context, stateDir string) (*runtime, error) {
	return openRuntimePrepared(ctx, stateDir, migrate.PrepareServiceRoot)
}

func openRuntimePrepared(ctx context.Context, stateDir string, prepare func(context.Context, string) error) (*runtime, error) {
	if err := statedir.Ensure(stateDir); err != nil {
		return nil, err
	}
	originalRoot, err := os.Stat(stateDir)
	if err != nil {
		return nil, err
	}
	originalDB, dbErr := os.Stat(filepath.Join(stateDir, journal.DBName))
	if dbErr != nil && !errors.Is(dbErr, os.ErrNotExist) {
		return nil, dbErr
	}
	err = prepare(ctx, stateDir)
	required := false
	if err != nil {
		var ce *v2contract.ControlError
		// Only the explicit migration-required refusal permits a status-only mode.
		if !errors.As(err, &ce) || ce.Code != v2contract.CodeOwnershipUnresolved || ce.Owner != "migrate" || ce.OperationID != "prepare-service-root" || ce.NextAction != "review mythhelm migrate --preview, then run mythhelm migrate --apply --yes with no service writer running" {
			return nil, err
		}
		required = true
	}
	currentRoot, rootErr := os.Stat(stateDir)
	if rootErr != nil || !os.SameFile(originalRoot, currentRoot) {
		return nil, errors.Join(control.ErrRootConflict, rootErr)
	}
	preparedDB, preparedDBErr := os.Stat(filepath.Join(stateDir, journal.DBName))
	if !required && preparedDBErr != nil {
		return nil, preparedDBErr
	}
	if originalDB != nil && (preparedDBErr != nil || !os.SameFile(originalDB, preparedDB)) {
		return nil, errors.Join(control.ErrRootConflict, preparedDBErr)
	}
	lease, err := control.AcquireInstanceLease(stateDir)
	if err != nil {
		return nil, err
	}
	r := &runtime{lease: lease, migrationRequired: required, entries: map[*actor]struct{}{}, joined: make(chan struct{}), views: newViewSlots()}
	if err := lease.CheckRootIdentity(ctx, stateDir); err != nil {
		_ = lease.Close()
		return nil, err
	}
	if !required {
		currentDB, err := os.Stat(filepath.Join(stateDir, journal.DBName))
		if err != nil || !os.SameFile(preparedDB, currentDB) {
			_ = lease.Close()
			return nil, errors.Join(control.ErrRootConflict, err)
		}
		ro, err := journal.OpenReadOnly(ctx, stateDir)
		if err != nil {
			_ = lease.Close()
			return nil, err
		}
		err = ro.Transact(ctx, func(tx *sql.Tx) error {
			var phase string
			if err := tx.QueryRowContext(ctx, `SELECT phase FROM migration_state WHERE id=1`).Scan(&phase); err != nil {
				return err
			}
			if phase != "adopted" {
				return &control.Error{Code: control.CodeCapabilityUnsupported, Message: "migration_required: root was not adopted under this lease"}
			}
			return nil
		})
		err = errors.Join(err, ro.CheckStateIdentity(ctx), ro.Close())
		if err != nil {
			_ = lease.Close()
			return nil, err
		}
		r.journal, err = journal.Open(ctx, stateDir)
		if err == nil {
			err = r.journal.CheckStateIdentity(ctx)
		}
		if err != nil {
			if r.journal != nil {
				_ = r.journal.Close()
			}
			_ = lease.Close()
			return nil, err
		}
	}
	r.server = control.NewStatusServer(r.status)
	return r, nil
}

func (r *runtime) status(ctx context.Context, _ control.Peer, _ control.Intent) (control.Result, error) {
	r.mu.Lock()
	closing := r.closing
	r.mu.Unlock()
	if closing {
		return control.Result{}, &control.Error{Code: control.CodeCapabilityUnsupported, Message: "service is shutting down"}
	}
	if err := r.lease.CheckRootIdentity(ctx, r.lease.Root()); err != nil {
		return control.Result{}, err
	}
	if r.journal != nil {
		if err := r.journal.CheckStateIdentity(ctx); err != nil {
			return control.Result{}, err
		}
	}
	endpoint, err := control.EndpointPath()
	if err != nil {
		return control.Result{}, err
	}
	body, err := json.Marshal(map[string]any{"pid": os.Getpid(), "generation": r.lease.Generation(), "root": r.lease.Root(), "endpoint": endpoint, "migration_required": r.migrationRequired})
	return control.Result{Body: body}, err
}

// shutdown refuses new work immediately. Its coordinator closes owners and
// leases only after entries actually join, never from inside a callback.
func (r *runtime) shutdown(ctx context.Context) error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closing = true
		r.views.close()
		actors := make([]*actor, 0, len(r.entries))
		for a := range r.entries {
			actors = append(actors, a)
			a.cancel()
		}
		r.mu.Unlock()
		go func() {
			for _, a := range actors {
				<-a.done
				r.closeErr = errors.Join(r.closeErr, a.err)
				if a.owner != nil {
					r.closeErr = errors.Join(r.closeErr, a.owner.Close())
				}
				r.closeErr = errors.Join(r.closeErr, a.adapter.close())
			}
			if r.journal != nil {
				r.closeErr = errors.Join(r.closeErr, r.journal.Close())
			}
			r.closeErr = errors.Join(r.closeErr, r.lease.Close())
			close(r.joined)
		}()
	})
	select {
	case <-r.joined:
		return r.closeErr
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", ErrCleanupIncomplete, ctx.Err())
	}
}

type actor struct {
	owner   *supervisor.ServiceOwner
	adapter *observerAdapter
	cancel  context.CancelFunc
	done    chan struct{}
	err     error
}

// startEntry is the shell's lifetime boundary. Task7 wires adopted recovery
// through this boundary; Task9 wires admitted starts after its binding gates.
func (r *runtime) startEntry(ctx context.Context, owner *supervisor.ServiceOwner, work func(context.Context, supervisor.Hooks) error) (*actor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if work == nil {
		return nil, &control.Error{Code: control.CodeInvalidContract, Message: "entry function is required"}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closing || r.migrationRequired {
		return nil, &control.Error{Code: control.CodeCapabilityUnsupported, Message: "service entry is unavailable"}
	}
	hold, err := r.lease.Hold()
	if err != nil {
		return nil, err
	}
	adapter, err := newObserverAdapter()
	if err != nil {
		hold()
		return nil, err
	}
	entryCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	a := &actor{owner: owner, adapter: adapter, cancel: cancel, done: make(chan struct{})}
	r.entries[a] = struct{}{}
	go func() { defer close(a.done); defer hold(); a.err = work(entryCtx, adapter.hooks()) }()
	return a, nil
}
