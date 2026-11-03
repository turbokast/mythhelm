package service

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/turbokast/mythhelm/internal/supervisor"
)

// attachmentCandidateLimit bounds the actor owners one attachment request
// inspects. It refuses attachment reads in a larger roster; it is not an
// execution, view-slot or aggregate memory limit.
const attachmentCandidateLimit = 64

// attachmentRequestBudget bounds how long a request waits to retain a scope.
const attachmentRequestBudget = 2 * time.Second

func attachmentRefused(format string, args ...any) error {
	return fmt.Errorf("ownership_unresolved: "+format+": %w", append(args, supervisor.ErrOwnership)...)
}

// authorityGate is the runtime-local read/write authority boundary: mint and
// read take a read guard, revoke takes the write guard. Unlike sync.RWMutex a
// waiter gives up when its context ends. mu guards readers, writer and
// changed; changed is closed and replaced whenever a guard is released.
type authorityGate struct {
	mu      sync.Mutex
	readers int
	writer  bool
	changed chan struct{}
}

func newAuthorityGate() *authorityGate {
	return &authorityGate{changed: make(chan struct{})}
}

func (g *authorityGate) acquire(ctx context.Context, write bool) (func(), error) {
	for {
		g.mu.Lock()
		if err := ctx.Err(); err != nil {
			g.mu.Unlock()
			return nil, err
		}
		if !g.writer && (!write || g.readers == 0) {
			if write {
				g.writer = true
			} else {
				g.readers++
			}
			g.mu.Unlock()
			return sync.OnceFunc(func() {
				g.mu.Lock()
				defer g.mu.Unlock()
				if write {
					g.writer = false
				} else {
					g.readers--
				}
				close(g.changed)
				g.changed = make(chan struct{})
			}), nil
		}
		changed := g.changed
		g.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// attachmentScope retains one existing owner's observation, an instance
// lease hold and an authority guard until release. Only
// retainAttachmentScope mints one; it survives the request's SQL commit and
// encoding until the consumer explicitly releases it on queue acceptance or
// discard.
type attachmentScope struct {
	owner   *supervisor.ServiceOwner
	runID   string
	write   bool
	release func()
}

// retainAttachmentScope routes runID to exactly one existing actor owner and
// retains it. It never acquires, recovers or enters execution: zero or
// several matching actors, a closing runtime or a failed proof refuse.
// write selects the guard kind only; it certifies nothing.
func (r *runtime) retainAttachmentScope(ctx context.Context, runID string, write bool) (*attachmentScope, error) {
	ctx, cancel := context.WithTimeout(ctx, attachmentRequestBudget)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	if r.closing || r.journal == nil {
		r.mu.Unlock()
		return nil, attachmentRefused("service is not accepting attachments")
	}
	if len(r.entries) > attachmentCandidateLimit {
		n := len(r.entries)
		r.mu.Unlock()
		return nil, attachmentRefused("%d actors exceed the %d attachment candidates", n, attachmentCandidateLimit)
	}
	candidates := make([]*supervisor.ServiceOwner, 0, len(r.entries))
	for a := range r.entries {
		var owner *supervisor.ServiceOwner
		if a != nil {
			owner = a.owner
		}
		candidates = append(candidates, owner)
	}
	r.mu.Unlock()

	var match *supervisor.ServiceOwner
	hits := 0
	for index, owner := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if owner.MatchesRun(runID) {
			hits++
			match = owner
		}
		if r.attachmentCandidateChecked != nil {
			r.attachmentCandidateChecked(index + 1)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if hits != 1 {
		return nil, attachmentRefused("%d existing owners match run %s", hits, runID)
	}

	var releases []func()
	releaseAll := func() {
		for _, release := range slices.Backward(releases) {
			release()
		}
	}
	observation, err := match.RetainObservation(ctx, r.journal, runID)
	if err != nil {
		return nil, err
	}
	releases = append(releases, observation)
	hold, err := r.lease.Hold()
	if err != nil {
		releaseAll()
		return nil, attachmentRefused("no live instance lease: %v", err)
	}
	releases = append(releases, hold)
	if err := r.checkAttachmentIdentity(ctx); err != nil {
		releaseAll()
		return nil, err
	}
	guard, err := r.authority.acquire(ctx, write)
	if err != nil {
		releaseAll()
		return nil, err
	}
	releases = append(releases, guard)
	r.mu.Lock()
	closing := r.closing
	r.mu.Unlock()
	if closing {
		releaseAll()
		return nil, attachmentRefused("service is shutting down")
	}
	if err := ctx.Err(); err != nil {
		releaseAll()
		return nil, err
	}
	return &attachmentScope{owner: match, runID: runID, write: write, release: sync.OnceFunc(releaseAll)}, nil
}

// checkAttachmentIdentity re-proves the held lease's generation and the
// original physical root and database independently of the owner.
func (r *runtime) checkAttachmentIdentity(ctx context.Context) error {
	if r.lease.Generation() <= 0 {
		return attachmentRefused("instance lease has no current generation")
	}
	if err := r.lease.CheckRootIdentity(ctx, r.journal.StateDir()); err != nil {
		return attachmentRefused("instance root identity: %v", err)
	}
	if err := r.journal.CheckStateIdentity(ctx); err != nil {
		return attachmentRefused("journal state identity: %v", err)
	}
	return nil
}
