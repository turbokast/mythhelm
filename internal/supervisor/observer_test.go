package supervisor

import (
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestObserverPrimitiveNeverBlocks(t *testing.T) {
	t.Parallel()
	s := NewObserverSink()
	r, err := s.OpenReader()
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan struct{})
	go func() {
		for i := range 1024 {
			s.Committed("synthetic-run", int64(i))
			s.Hint(NoticeReservationHeld)
		}
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("producer waited for stalled consumer")
	}
	defer func() { _ = r.Close() }()
	// I21 (v2 §2): fixed storage independent of producer count.
	if cap(r.Wakeups()) != 1 || len(r.Wakeups()) != 1 {
		t.Fatalf("wakeups len/cap = %d/%d, want 1/1", len(r.Wakeups()), cap(r.Wakeups()))
	}
	batch, err := r.DrainHints(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Codes) != 16 || batch.Dropped != 1008 {
		t.Fatalf("batch = %+v, want 16 hints, 1008 dropped", batch)
	}
	if !slices.Equal(batch.Codes, slices.Repeat([]NoticeCode{NoticeReservationHeld}, 16)) {
		t.Fatalf("codes = %v", batch.Codes)
	}
	<-r.Wakeups()
	s.Committed("another-run", 9999)
	select {
	case <-r.Wakeups():
	default:
		t.Fatal("new commit did not wake reader after consumption")
	}
}

func TestObserverHintOverflowIsNotEvidence(t *testing.T) {
	t.Parallel()
	s := NewObserverSink()
	r, err := s.OpenReader()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	pattern := []NoticeCode{NoticeReservationHeld, NoticeWorkerReattached, NoticeRenderRefresh}
	for i := range 21 {
		s.Hint(pattern[i%3])
	}
	first, err := r.DrainHints(3)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(first.Codes, []NoticeCode{NoticeRenderRefresh, NoticeReservationHeld, NoticeWorkerReattached}) || first.Dropped != 5 {
		t.Fatalf("oldest replacement/partial drain = %+v", first)
	}
	rest, err := r.DrainHints(0)
	if err != nil {
		t.Fatal(err)
	}
	want := []NoticeCode{NoticeRenderRefresh, NoticeReservationHeld, NoticeWorkerReattached, NoticeRenderRefresh, NoticeReservationHeld, NoticeWorkerReattached, NoticeRenderRefresh, NoticeReservationHeld, NoticeWorkerReattached, NoticeRenderRefresh, NoticeReservationHeld, NoticeWorkerReattached, NoticeRenderRefresh}
	if !slices.Equal(rest.Codes, want) || rest.Dropped != 5 || rest.Rejected != 0 {
		t.Fatalf("remaining FIFO = %+v, want %v", rest, want)
	}
	// I09/I23 (v2 §2): invalid/authority-bearing text is refused; committed
	// identifiers and sequences are never retained as hints or event cursors.
	for _, code := range []NoticeCode{"", "unknown", "stop_requested", "deadline_reached", "verified", "quota_unavailable", "worker_identity_unresolved", "worker_exited_early", "spool_ingestion_failed", "candidate_freeze_failed", "verification_unavailable", "verification_binding_unavailable", "recovery_ownership_unresolved"} {
		s.Hint(code)
	}
	s.Committed("synthetic-run", 9223372036854775807)
	b, err := r.DrainHints(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Codes) != 0 || b.Rejected != 13 || b.Dropped != 5 || b.Closed {
		t.Fatalf("non-evidence batch = %+v", b)
	}
	// Seed counters under their production lock to reach the otherwise infeasible
	// uint64 boundary without adding a production testing API.
	s.state.mu.Lock()
	s.state.dropped = ^uint64(0) - 1
	s.state.rejected = ^uint64(0) - 1
	s.state.mu.Unlock()
	for range 20 {
		s.Hint(NoticeRenderRefresh)
		s.Hint("stop_requested")
	}
	s.RequestClose()
	s.RequestClose()
	s.Hint(NoticeReservationHeld)
	s.Hint("unknown")
	s.Committed("late", 1)
	b, err = r.DrainHints(0)
	if err != nil {
		t.Fatal(err)
	}
	if b.Dropped != ^uint64(0) || b.Rejected != ^uint64(0) || !b.Closed || !slices.Equal(b.Codes, slices.Repeat([]NoticeCode{NoticeRenderRefresh}, 16)) {
		t.Fatalf("saturation/close = %+v", b)
	}
	select {
	case _, ok := <-r.Wakeups():
		if !ok {
			t.Fatal("Wakeups closed")
		}
	default:
		t.Fatal("lost coalesced wakeup on close")
	}
	select {
	case _, ok := <-r.Wakeups():
		if !ok {
			t.Fatal("Wakeups closed after draining")
		}
		t.Fatal("late commit queued after close")
	default:
	}
}

func TestObserverPrimitiveImportsNoOwner(t *testing.T) {
	t.Parallel()
	source, err := parser.ParseFile(token.NewFileSet(), "observer.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range source.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		switch path {
		case "sync", "github.com/turbokast/mythhelm/internal/v2contract":
		default:
			t.Fatalf("observer primitive gained an execution/I/O dependency: %s", path)
		}
	}
	s := NewObserverSink()
	copied := *s
	r, err := s.OpenReader()
	if err != nil {
		t.Fatal(err)
	}
	readerCopy := *r
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		<-start
		for i := range 100 {
			copied.Hint(NoticeWorkerReattached)
			copied.Committed("run", int64(i))
			copied.Hint("stop_requested")
		}
	})
	wg.Go(func() { <-start; s.RequestClose(); copied.RequestClose() })
	close(start)
	wg.Wait()
	select {
	case <-readerCopy.Done():
	default:
		t.Fatal("copied reader has independent lifetime")
	}
	batch, err := readerCopy.DrainHints(0)
	if err != nil {
		t.Fatal(err)
	}
	if !batch.Closed || len(batch.Codes) > 16 {
		t.Fatalf("closed bounded batch = %+v", batch)
	}
	for _, code := range batch.Codes {
		if code != NoticeWorkerReattached {
			t.Fatalf("authority-bearing code retained: %q", code)
		}
	}
	before := batch
	copied.Hint(NoticeRenderRefresh)
	copied.Hint("unknown")
	copied.Committed("late", -1)
	after, err := r.DrainHints(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Codes) != 0 || after.Dropped != before.Dropped || after.Rejected != before.Rejected || !after.Closed {
		t.Fatalf("late producer changed closed sink: before %+v, after %+v", before, after)
	}
	if err := readerCopy.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}
