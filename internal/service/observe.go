package service

import (
	"context"
	"sync"

	"github.com/turbokast/mythhelm/internal/control"
)

// viewSlots owns display lifetimes only. Task10 wires these slots into its
// public Observe handler and supplies the separate eight-reader/page bounds.
type viewSlots struct {
	mu     sync.Mutex
	closed bool
	byPeer map[string]int
	active map[*view]string
}

type view struct {
	cancel context.CancelFunc
	done   <-chan struct{}
	once   sync.Once
	slots  *viewSlots
}

func newViewSlots() *viewSlots {
	return &viewSlots{byPeer: map[string]int{}, active: map[*view]string{}}
}
func (s *viewSlots) open(ctx context.Context, peer string) (*view, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || peer == "" || len(s.active) >= 64 || s.byPeer[peer] >= 16 {
		return nil, &control.Error{Code: control.CodeCapabilityUnsupported, Message: "observer_capacity"}
	}
	viewCtx, cancel := context.WithCancel(ctx)
	v := &view{cancel: cancel, done: viewCtx.Done(), slots: s}
	s.active[v] = peer
	s.byPeer[peer]++
	return v, nil
}
func (v *view) close() {
	if v == nil {
		return
	}
	v.once.Do(func() {
		v.cancel()
		s := v.slots
		s.mu.Lock()
		defer s.mu.Unlock()
		if peer, ok := s.active[v]; ok {
			delete(s.active, v)
			s.byPeer[peer]--
			if s.byPeer[peer] == 0 {
				delete(s.byPeer, peer)
			}
		}
	})
}
func (s *viewSlots) close() {
	s.mu.Lock()
	s.closed = true
	views := make([]*view, 0, len(s.active))
	for v := range s.active {
		views = append(views, v)
	}
	s.mu.Unlock()
	for _, v := range views {
		v.close()
	}
}
