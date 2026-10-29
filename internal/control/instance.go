package control

import (
	"errors"
	"fmt"
	"math"
	"sync"
)

// ErrLeaseClosed reports a lease that is closed, closing or was never
// minted by a successful acquisition.
var ErrLeaseClosed = errors.New("control: instance lease is not held")

// InstanceLease is the proof that this process holds the per-user instance
// lock for one canonical root (supervisor-attachment-control design §2).
// Its only field is private, so the zero value and a value from another
// package prove nothing; copies share one lifetime.
type InstanceLease struct {
	s *leaseState
}

// leaseState is one acquisition's lifetime. mu guards holds, closing and
// closed; root and generation are immutable after minting.
type leaseState struct {
	root       string
	generation int64
	release    func()

	mu      sync.Mutex
	drained *sync.Cond
	holds   int
	closing bool
	closed  bool
}

// AcquireInstanceLease takes the instance lock for stateDir as
// AcquireInstance does and mints the only proof of holding it.
func AcquireInstanceLease(stateDir string) (*InstanceLease, error) {
	release, meta, err := acquireInstance(stateDir)
	if err != nil {
		return nil, err
	}
	if meta.Generation == 0 || meta.Generation > math.MaxInt64 {
		release()
		return nil, fmt.Errorf("control: instance generation %d is out of range", meta.Generation)
	}
	s := &leaseState{root: meta.Root, generation: int64(meta.Generation), release: release}
	s.drained = sync.NewCond(&s.mu)
	return &InstanceLease{s: s}, nil
}

// Generation is the held boot generation, or 0 for a closed or forged lease.
func (l *InstanceLease) Generation() int64 {
	if l == nil || l.s == nil {
		return 0
	}
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	if l.s.closed {
		return 0
	}
	return l.s.generation
}

// Root is the held canonical state root, or "" for a closed or forged lease.
func (l *InstanceLease) Root() string {
	if l == nil || l.s == nil {
		return ""
	}
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	if l.s.closed {
		return ""
	}
	return l.s.root
}

// Hold pins the lease's lifetime until the returned release runs: Close
// waits for every outstanding hold before it frees the OS lock. A lease that
// is closing, closed or forged refuses new holds. release is idempotent.
func (l *InstanceLease) Hold() (func(), error) {
	if l == nil || l.s == nil {
		return nil, ErrLeaseClosed
	}
	s := l.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing || s.closed {
		return nil, ErrLeaseClosed
	}
	s.holds++
	return sync.OnceFunc(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.holds--
		if s.holds == 0 {
			s.drained.Broadcast()
		}
	}), nil
}

// Close refuses new holds, waits for outstanding ones, then releases the
// instance lock. It is idempotent; concurrent calls all return once closed.
// It blocks while any hold is outstanding, so a holder (such as a service
// owner's hook callback) must never call it: cancel the holder's work and
// call Close from outside.
func (l *InstanceLease) Close() error {
	if l == nil || l.s == nil {
		return ErrLeaseClosed
	}
	s := l.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		for !s.closed {
			s.drained.Wait()
		}
		return nil
	}
	s.closing = true
	for s.holds > 0 {
		s.drained.Wait()
	}
	s.release()
	s.closed = true
	s.drained.Broadcast()
	return nil
}
