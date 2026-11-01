package supervisor_test

import (
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

func observerError(t *testing.T, err error, code v2contract.Code, reason string) {
	t.Helper()
	var ce *v2contract.ControlError
	if !errors.As(err, &ce) {
		t.Fatalf("error = %v, want ControlError", err)
	}
	if ce.Code != code || ce.Detail["mythhelm.observer/reason"] != reason {
		t.Fatalf("error = %+v, want %s/%s", ce, code, reason)
	}
	if err := ce.Validate(); err != nil {
		t.Fatalf("invalid typed error: %v", err)
	}
}

func TestObserverExternalReaderLifecycle(t *testing.T) {
	t.Parallel()
	s := supervisor.NewObserverSink()
	s.Hint(supervisor.NoticeReservationHeld)
	r, err := s.OpenReader()
	if err != nil {
		t.Fatal(err)
	}
	// This interface requires exactly read-only channels from the exported seam.
	var channels interface {
		Wakeups() <-chan struct{}
		Done() <-chan struct{}
	} = r
	wakeups, done := channels.Wakeups(), channels.Done()
	if wakeups == nil || done == nil {
		t.Fatal("reader channels missing")
	}
	if second, err := s.OpenReader(); second != nil || err == nil {
		t.Fatalf("duplicate = %v, %v", second, err)
	} else {
		observerError(t, err, v2contract.CodeRevisionConflict, "observer_reader_active")
	}
	for _, limit := range []int{-1, 17} {
		b, err := r.DrainHints(limit)
		observerError(t, err, v2contract.CodeInvalidContract, "invalid_contract")
		if len(b.Codes) != 0 {
			t.Fatalf("invalid drain returned codes: %+v", b)
		}
	}
	b, err := r.DrainHints(1)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(b.Codes, []supervisor.NoticeCode{supervisor.NoticeReservationHeld}) {
		t.Fatalf("first drain = %+v", b)
	}
	b.Codes[0] = supervisor.NoticeCode("caller mutation")
	s.Hint(supervisor.NoticeWorkerReattached)
	next, err := r.DrainHints(16)
	if err != nil || !slices.Equal(next.Codes, []supervisor.NoticeCode{supervisor.NoticeWorkerReattached}) {
		t.Fatalf("independent batch = %+v, %v", next, err)
	}
	for range 16 {
		s.Hint(supervisor.NoticeRenderRefresh)
	}
	wrapped, err := r.DrainHints(0)
	if err != nil || len(wrapped.Codes) != 16 {
		t.Fatalf("wrapped batch = %+v, %v", wrapped, err)
	}
	if next.Codes[0] != supervisor.NoticeWorkerReattached {
		t.Fatal("ring reuse mutated earlier batch")
	}
	if b.Codes[0] != "caller mutation" {
		t.Fatal("later drain mutated returned batch")
	}
	copyReader := *r
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	default:
		t.Fatal("Close did not signal Done")
	}
	if err := copyReader.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := s.OpenReader()
	if err != nil {
		t.Fatal(err)
	}
	s.Hint(supervisor.NoticeRenderRefresh)
	if _, err := copyReader.DrainHints(0); err == nil {
		t.Fatal("old lease drained reopened reader")
	} else {
		observerError(t, err, v2contract.CodeInvalidContract, "invalid_contract")
	}
	if err := copyReader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenReader(); err == nil {
		t.Fatal("old Close released new lease")
	} else {
		observerError(t, err, v2contract.CodeRevisionConflict, "observer_reader_active")
	}
	s.RequestClose()
	s.RequestClose()
	select {
	case <-reopened.Done():
	default:
		t.Fatal("sink close did not signal Done")
	}
	closed, err := reopened.DrainHints(0)
	if err != nil || !closed.Closed || !slices.Equal(closed.Codes, []supervisor.NoticeCode{supervisor.NoticeRenderRefresh}) {
		t.Fatalf("post-close batch = %+v, %v", closed, err)
	}
	empty, err := reopened.DrainHints(0)
	if err != nil || !empty.Closed || len(empty.Codes) != 0 {
		t.Fatalf("second post-close drain = %+v, %v", empty, err)
	}
	if reader, err := s.OpenReader(); reader != nil || err == nil {
		t.Fatalf("closed open = %v, %v", reader, err)
	} else {
		observerError(t, err, v2contract.CodeDependencyStale, "observer_closed")
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.DrainHints(0); err == nil {
		t.Fatal("closed lease drain succeeded")
	}
}

func TestObserverExternalNilHandles(t *testing.T) {
	t.Parallel()
	for _, s := range []*supervisor.ObserverSink{nil, {}} {
		s.Hint(supervisor.NoticeRenderRefresh)
		s.Committed("run", 1)
		s.RequestClose()
		r, err := s.OpenReader()
		if r != nil {
			t.Fatal("invalid sink returned reader")
		}
		observerError(t, err, v2contract.CodeInvalidContract, "invalid_contract")
	}
	for _, r := range []*supervisor.ObserverReader{nil, {}} {
		if r.Wakeups() != nil {
			t.Fatal("nil handle wakeups not nil")
		}
		select {
		case <-r.Done():
		default:
			t.Fatal("nil handle Done not closed")
		}
		_, err := r.DrainHints(0)
		observerError(t, err, v2contract.CodeInvalidContract, "invalid_contract")
		observerError(t, r.Close(), v2contract.CodeInvalidContract, "invalid_contract")
	}
}

func TestObserverExternalConcurrentCloseHint(t *testing.T) {
	t.Parallel()
	s := supervisor.NewObserverSink()
	r, err := s.OpenReader()
	if err != nil {
		t.Fatal(err)
	}
	copiedSink, copiedReader := *s, *r
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			<-start
			for j := range 256 {
				copiedSink.Hint(supervisor.NoticeReservationHeld)
				copiedSink.Committed("run", int64(j))
				_, _ = copiedReader.DrainHints(1)
			}
		})
	}
	wg.Go(func() { <-start; _ = r.Close(); s.RequestClose() })
	close(start)
	wg.Wait()
	select {
	case <-copiedReader.Done():
	default:
		t.Fatal("copy did not share close lifetime")
	}
	s.Hint(supervisor.NoticeRenderRefresh)
	s.Committed("late", 999)
	if _, err := copiedSink.OpenReader(); err == nil {
		t.Fatal("copied sink reopened after closure")
	} else {
		observerError(t, err, v2contract.CodeDependencyStale, "observer_closed")
	}
}
